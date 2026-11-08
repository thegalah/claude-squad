package app

import (
	"claude-squad/config"
	"claude-squad/session"
	"claude-squad/ui"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

// writeFakeExe writes a shell script standing in for the cs binary.
func writeFakeExe(t *testing.T, path string, exitCode string, padding string) {
	t.Helper()
	tmp := path + ".tmp"
	script := "#!/bin/sh\n# " + padding + "\nexit " + exitCode + "\n"
	require.NoError(t, os.WriteFile(tmp, []byte(script), 0o755))
	// Replace like `go build -o` does: the running file is unlinked, not rewritten.
	require.NoError(t, os.Rename(tmp, path))
}

func newRestartTestHome(t *testing.T) (*home, string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	prevInterval := exeCheckInterval
	exeCheckInterval = time.Millisecond
	t.Cleanup(func() { exeCheckInterval = prevInterval })
	storage, err := session.NewRepoStorage(config.LoadState(), "/repos/a")
	require.NoError(t, err)

	exe := filepath.Join(t.TempDir(), "cs")
	writeFakeExe(t, exe, "0", "v1")
	stamp, err := statExe(exe)
	require.NoError(t, err)

	s := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	h := &home{
		ctx:      context.Background(),
		spinner:  s,
		list:     ui.NewList(&s, false),
		storage:  storage,
		state:    stateDefault,
		exeWatch: &exeWatch{path: exe, started: stamp, lastSeen: stamp},
	}
	return h, exe
}

// check runs one executable check as if the tick fired now.
func check(t *testing.T, h *home) tea.Cmd {
	t.Helper()
	stamp, err := statExe(h.exeWatch.path)
	return h.handleExeCheck(exeCheckMsg{stamp: stamp, err: err})
}

// verify runs `version` on the current executable and handles the result.
func verify(t *testing.T, h *home) tea.Cmd {
	t.Helper()
	stamp, err := statExe(h.exeWatch.path)
	require.NoError(t, err)
	msg := verifyExeCmd(h.exeWatch.path, stamp)().(exeVerifiedMsg)
	return h.handleExeVerified(msg)
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestRestartsAfterExecutableReplaced(t *testing.T) {
	h, exe := newRestartTestHome(t)

	check(t, h)
	require.False(t, h.exeWatch.restart, "unchanged executable must not restart")

	writeFakeExe(t, exe, "0", "v2 is longer")
	check(t, h) // first sighting: wait for the file to settle
	require.Nil(t, h.exeWatch.verified)
	check(t, h) // unchanged since last check: verify it

	require.True(t, isQuit(verify(t, h)))
	require.True(t, h.exeWatch.restart)
}

func TestRestartWaitsUntilIdle(t *testing.T) {
	h, exe := newRestartTestHome(t)
	writeFakeExe(t, exe, "0", "v2 is longer")
	check(t, h)
	check(t, h)

	h.state = stateHelp
	require.False(t, isQuit(verify(t, h)), "must not restart while an overlay is open")
	require.False(t, isQuit(check(t, h)))
	require.False(t, h.exeWatch.restart)

	h.state = stateDefault
	require.True(t, isQuit(check(t, h)), "restarts on the next check once idle")
	require.True(t, h.exeWatch.restart)
}

func TestNoRestartWhenNewExecutableFails(t *testing.T) {
	h, exe := newRestartTestHome(t)
	writeFakeExe(t, exe, "1", "broken build")
	check(t, h)
	check(t, h)

	require.False(t, isQuit(verify(t, h)))
	require.NotNil(t, h.exeWatch.failed)
	require.False(t, isQuit(check(t, h)))
	require.False(t, h.exeWatch.restart)

	// A fixed build is picked up.
	writeFakeExe(t, exe, "0", "fixed build, longer")
	check(t, h)
	check(t, h)
	require.True(t, isQuit(verify(t, h)))
}

func TestNoRestartWhenExecutableReplacedDuringVerify(t *testing.T) {
	h, exe := newRestartTestHome(t)
	writeFakeExe(t, exe, "0", "v2 is longer")
	check(t, h)
	check(t, h)
	stamp, err := statExe(exe)
	require.NoError(t, err)

	writeFakeExe(t, exe, "0", "v3 is longer still")
	check(t, h)
	require.False(t, isQuit(h.handleExeVerified(exeVerifiedMsg{stamp: stamp})))
	require.False(t, h.exeWatch.restart)
}
