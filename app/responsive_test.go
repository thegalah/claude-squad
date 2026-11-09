package app

import (
	"claude-squad/cmd/cmd_test"
	"claude-squad/session"
	"claude-squad/session/git"
	"claude-squad/session/tmux"
	"claude-squad/ui"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

// fileptyFactory hands back a throwaway file instead of a real PTY.
type fileptyFactory struct{ t *testing.T }

func (p fileptyFactory) Start(*exec.Cmd) (*os.File, error) {
	return os.OpenFile(filepath.Join(p.t.TempDir(), "pty"), os.O_CREATE|os.O_RDWR, 0644)
}
func (fileptyFactory) Close() {}

// fakePane is a tmux session's pane content, and how often it has been captured.
type fakePane struct {
	content  atomic.Value
	captures atomic.Int32
}

// startedInstance returns a started instance whose tmux session is faked: capture-pane
// returns pane's content. If alive is false, the session is gone and the instance comes
// back Paused.
func startedInstance(t *testing.T, title string, alive bool) (*session.Instance, *fakePane) {
	t.Helper()
	pane := &fakePane{}
	pane.content.Store("")
	exec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if !alive && strings.Contains(cmd.String(), "has-session") {
				return errors.New("can't find session")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			pane.captures.Add(1)
			return []byte(pane.content.Load().(string)), nil
		},
	}
	inst, err := session.NewInstance(session.InstanceOptions{Title: title, Path: t.TempDir(), Program: "claude"})
	require.NoError(t, err)
	inst.SetTmuxSession(tmux.NewTmuxSessionWithDeps(title, "claude", fileptyFactory{t}, exec))
	repo, sha := testRepo(t)
	inst.SetGitWorktree(git.NewGitWorktreeFromStorage(repo, repo, title, "main", sha, true))
	require.NoError(t, inst.Start(false))
	return inst, pane
}

// testRepo creates a git repo with one commit, standing in for an instance's worktree.
func testRepo(t *testing.T) (dir, sha string) {
	t.Helper()
	dir = t.TempDir()
	run := func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		return strings.TrimSpace(string(out))
	}
	run("init", "-q", "-b", "main")
	run("commit", "-q", "--allow-empty", "-m", "init")
	return dir, run("rev-parse", "HEAD")
}

// memStorage is an in-memory config.InstanceStorage.
type memStorage struct{ data json.RawMessage }

func (s *memStorage) SaveInstances(d json.RawMessage) error { s.data = d; return nil }
func (s *memStorage) GetInstances() json.RawMessage {
	if s.data == nil {
		return json.RawMessage("[]")
	}
	return s.data
}
func (s *memStorage) UpdateInstances(fn func(json.RawMessage) (json.RawMessage, error)) error {
	d, err := fn(s.GetInstances())
	if err == nil {
		s.data = d
	}
	return err
}
func (s *memStorage) DeleteAllInstances() error { s.data = nil; return nil }

// allHelpSeen is a config.AppState that has seen every help screen.
type allHelpSeen struct{}

func (allHelpSeen) GetHelpScreensSeen() uint32      { return ^uint32(0) }
func (allHelpSeen) SetHelpScreensSeen(uint32) error { return nil }

// newResponsiveTestHome returns a home holding insts, with the first selected.
func newResponsiveTestHome(t *testing.T, insts ...*session.Instance) *home {
	t.Helper()
	h := newPruneTestHome(t)
	h.appState = allHelpSeen{}
	storage, err := session.NewRepoStorage(&memStorage{}, t.TempDir())
	require.NoError(t, err)
	h.storage = storage
	for _, inst := range insts {
		h.list.AddInstance(inst)()
	}
	h.instanceChanged()
	return h
}

// runCmd runs cmd and returns the messages it produces, expanding batches. Commands that
// don't finish within a second (tickers) are skipped.
func runCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(time.Second):
		return nil
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, runCmd(c)...)
		}
		return out
	}
	if msg == nil {
		return nil
	}
	return []tea.Msg{msg}
}

func findMsg[T any](t *testing.T, msgs []tea.Msg) T {
	t.Helper()
	for _, msg := range msgs {
		if m, ok := msg.(T); ok {
			return m
		}
	}
	var zero T
	t.Fatalf("no %T among %v", zero, msgs)
	return zero
}

func TestPreviewTickCapturesInBackground(t *testing.T) {
	alpha, pane := startedInstance(t, "alpha", true)
	h := newResponsiveTestHome(t, alpha)
	pane.content.Store("hello from alpha")
	before := pane.captures.Load()

	cmd := h.refreshPreview()
	require.Equal(t, before, pane.captures.Load(), "the tick must not capture on the UI goroutine")
	require.NotContains(t, h.View(), "hello from alpha")

	captured := findMsg[previewCapturedMsg](t, runCmd(cmd))
	h.Update(captured)
	require.Contains(t, h.View(), "hello from alpha")
}

func TestStalePreviewCaptureIsDropped(t *testing.T) {
	alpha, alphaPane := startedInstance(t, "alpha", true)
	bravo, _ := startedInstance(t, "bravo", true)
	h := newResponsiveTestHome(t, alpha, bravo)
	alphaPane.content.Store("hello from alpha")

	captured := findMsg[previewCapturedMsg](t, runCmd(h.refreshPreview()))
	h.list.SetSelectedInstance(1) // bravo
	h.instanceChanged()
	h.Update(captured)
	require.NotContains(t, h.View(), "hello from alpha", "alpha's capture landed on bravo's preview")
}

func TestSwitchingToDiffTabComputesDiffRightAway(t *testing.T) {
	alpha, _ := startedInstance(t, "alpha", true)
	h := newResponsiveTestHome(t, alpha)

	h.tabbedWindow.SetActiveTab(ui.DiffTab)
	cmd := h.instanceChanged()
	require.NotNil(t, cmd, "expected a background diff for the newly shown Diff tab")
	findMsg[diffComputedMsg](t, runCmd(cmd))

	require.Nil(t, h.instanceChanged(), "no new diff while nothing changed")
}

func TestNumstatPassKeepsSelectedFullDiff(t *testing.T) {
	alpha, _ := startedInstance(t, "alpha", true)
	bravo, _ := startedInstance(t, "bravo", true)
	h := newResponsiveTestHome(t, alpha, bravo)
	full := &git.DiffStats{Content: "+line\n", Added: 1}
	alpha.SetDiffStats(full)
	bravo.SetDiffStats(&git.DiffStats{Content: "+old\n", Added: 1})

	// A pass that began while bravo was selected: alpha only got counts.
	h.applyMetadata([]instanceMetaResult{
		{instance: alpha, diffStats: &git.DiffStats{Added: 2}},
		{instance: bravo, diffStats: &git.DiffStats{Added: 3}},
	}, time.Now())

	require.Same(t, full, alpha.GetDiffStats(), "the selected instance's full diff was blanked")
	require.Equal(t, 3, bravo.GetDiffStats().Added)
}

func TestStatusStaysRunningUntilOutputSettles(t *testing.T) {
	alpha, _ := startedInstance(t, "alpha", true)
	h := newResponsiveTestHome(t, alpha)
	t0 := time.Now()

	h.applyMetadata([]instanceMetaResult{{instance: alpha, updated: true}}, t0)
	require.Equal(t, session.Running, alpha.Status)
	h.applyMetadata([]instanceMetaResult{{instance: alpha}}, t0.Add(metadataInterval))
	require.Equal(t, session.Running, alpha.Status, "one quiet pass shouldn't flip it to Ready")
	h.applyMetadata([]instanceMetaResult{{instance: alpha}}, t0.Add(readyAfter))
	require.Equal(t, session.Ready, alpha.Status)
}

func TestResumeRunsInBackground(t *testing.T) {
	alpha, _ := startedInstance(t, "alpha", false)
	require.True(t, alpha.Paused())
	h := newResponsiveTestHome(t, alpha)

	cmd := press(h, "r")
	require.NotNil(t, cmd)
	require.Equal(t, session.BusyResuming, alpha.Busy())
	require.Contains(t, h.View(), "resuming…")
	require.Nil(t, press(h, "r"), "a second resume while one runs")

	// Don't run cmd (it would run git); deliver its result instead.
	h.Update(busyDoneMsg{instance: alpha, op: session.BusyResuming, err: errors.New("no worktree")})
	require.Equal(t, session.NotBusy, alpha.Busy())
	require.Contains(t, h.View(), "no worktree")
}

func TestCheckoutPausesInBackground(t *testing.T) {
	alpha, _ := startedInstance(t, "alpha", true)
	h := newResponsiveTestHome(t, alpha)

	require.NotNil(t, press(h, "c"))
	require.Equal(t, session.BusyPausing, alpha.Busy())
	require.Empty(t, h.snapshotActiveInstances(), "status polling must leave a pausing instance alone")
	require.False(t, h.canRestartNow())
	require.Contains(t, h.View(), "Pausing session…")
}

func TestKillRunsInBackground(t *testing.T) {
	alpha, _ := startedInstance(t, "alpha", true)
	bravo, _ := startedInstance(t, "bravo", true)
	h := newResponsiveTestHome(t, alpha, bravo)

	press(h, "D")
	require.Equal(t, stateConfirm, h.state)
	require.NotNil(t, press(h, "y"))
	require.Equal(t, session.BusyKilling, alpha.Busy())
	require.Len(t, h.list.GetInstances(), 2, "stays listed until the kill finishes")

	h.Update(killDoneMsg{instance: alpha})
	require.Len(t, h.list.GetInstances(), 1)
	require.Equal(t, "bravo", selectedTitle(h))
}

func TestRefusedKillKeepsInstance(t *testing.T) {
	alpha, _ := startedInstance(t, "alpha", true)
	h := newResponsiveTestHome(t, alpha)
	alpha.SetBusy(session.BusyKilling)

	h.Update(killDoneMsg{instance: alpha, checkErr: errors.New("instance alpha is currently checked out")})
	require.Equal(t, session.NotBusy, alpha.Busy())
	require.Len(t, h.list.GetInstances(), 1)
	require.Contains(t, h.View(), "currently checked out")
}

func TestQuitWaitsForBackgroundOperation(t *testing.T) {
	alpha, _ := startedInstance(t, "alpha", true)
	h := newResponsiveTestHome(t, alpha)
	alpha.SetBusy(session.BusyPushing)

	press(h, "q")
	require.Contains(t, h.View(), "still pushing 'alpha'")
}

func TestMetadataIgnoresKilledInstance(t *testing.T) {
	alpha, _ := startedInstance(t, "alpha", true)
	h := newResponsiveTestHome(t, alpha)
	alpha.SetBusy(session.BusyKilling)
	h.Update(killDoneMsg{instance: alpha})
	before := alpha.Status

	// A pass that began before the kill.
	h.applyMetadata([]instanceMetaResult{{instance: alpha, updated: true, diffStats: &git.DiffStats{Added: 1}}}, time.Now())
	require.Equal(t, before, alpha.Status)
	require.NotContains(t, h.lastOutputChange, alpha)
}
