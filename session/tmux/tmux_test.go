package tmux

import (
	cmd2 "claude-squad/cmd"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"claude-squad/cmd/cmd_test"

	"github.com/stretchr/testify/require"
)

type MockPtyFactory struct {
	t *testing.T

	// Array of commands and the corresponding file handles representing PTYs.
	cmds  []*exec.Cmd
	files []*os.File
}

func (pt *MockPtyFactory) Start(cmd *exec.Cmd) (*os.File, error) {
	filePath := filepath.Join(pt.t.TempDir(), fmt.Sprintf("pty-%s-%d", pt.t.Name(), rand.Int31()))
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR, 0644)
	if err == nil {
		pt.cmds = append(pt.cmds, cmd)
		pt.files = append(pt.files, f)
	}
	return f, err
}

func (pt *MockPtyFactory) Close() {}

func NewMockPtyFactory(t *testing.T) *MockPtyFactory {
	return &MockPtyFactory{
		t: t,
	}
}

func TestSanitizeName(t *testing.T) {
	session := NewTmuxSession("asdf", "program")
	require.Equal(t, TmuxPrefix+"asdf", session.sanitizedName)

	session = NewTmuxSession("a sd f . . asdf", "program")
	require.Equal(t, TmuxPrefix+"asdf__asdf", session.sanitizedName)
}

func TestStartTmuxSession(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	created := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("session already exists")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("output"), nil
		},
	}

	workdir := t.TempDir()
	session := newTmuxSession("test-session", "claude", ptyFactory, cmdExec)

	err := session.Start(workdir)
	require.NoError(t, err)
	require.Equal(t, 2, len(ptyFactory.cmds))
	require.Equal(t, fmt.Sprintf("tmux new-session -d -s claudesquad_test-session -c %s claude", workdir),
		cmd2.ToString(ptyFactory.cmds[0]))
	require.Equal(t, "tmux attach-session -t claudesquad_test-session",
		cmd2.ToString(ptyFactory.cmds[1]))

	require.Equal(t, 2, len(ptyFactory.files))

	// File should be closed.
	_, err = ptyFactory.files[0].Stat()
	require.Error(t, err)
	// File should be open
	_, err = ptyFactory.files[1].Stat()
	require.NoError(t, err)
}

// A tmux server that has gone away (reboot, crash, `tmux kill-server`) takes every session
// with it. attach-session against a missing session still forks successfully, so Restore
// has to check for the session itself or it reports success while attached to nothing.
func TestRestoreReturnsErrSessionNotFoundWhenSessionIsGone(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") {
				return fmt.Errorf("can't find session")
			}
			return nil
		},
	}

	session := NewTmuxSessionWithDeps("gone", "program", ptyFactory, cmdExec)
	err := session.Restore()

	require.ErrorIs(t, err, ErrSessionNotFound)
	require.Empty(t, ptyFactory.cmds, "should not have opened a PTY for a session that does not exist")
}

func TestRestoreAttachesWhenSessionExists(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error { return nil },
	}

	session := NewTmuxSessionWithDeps("alive", "program", ptyFactory, cmdExec)
	require.NoError(t, session.Restore())
	require.Len(t, ptyFactory.cmds, 1)
	require.Contains(t, ptyFactory.cmds[0].String(), "attach-session")
}

func TestSplitAtDetachKey(t *testing.T) {
	motion := []byte("\x1b[<35;120;55M")
	for _, tc := range []struct {
		name    string
		input   []byte
		forward []byte
		detach  bool
	}{
		{"plain input", []byte("ls\r"), []byte("ls\r"), false},
		{"ctrl+q alone", []byte{17}, []byte{}, true},
		{"ctrl+q after mouse motion", append(append([]byte{}, motion...), 17), motion, true},
		{"ctrl+q before mouse motion", append([]byte{17}, motion...), []byte{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			forward, detach := splitAtDetachKey(tc.input)
			require.Equal(t, tc.detach, detach)
			require.Equal(t, string(tc.forward), string(forward))
		})
	}
}

// newCaptureCountingSession returns a session whose capture-pane returns *content and
// counts its calls in *captures.
func newCaptureCountingSession(t *testing.T, content *string, captures *int) *TmuxSession {
	exec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			*captures++
			return []byte(*content), nil
		},
	}
	s := newTmuxSession("shared", "claude", NewMockPtyFactory(t), exec)
	s.monitor = newStatusMonitor()
	return s
}

// The preview captures the selected session's pane every tick; the status check must
// reuse that rather than capture the same pane again.
func TestHasUpdatedReusesFreshCapture(t *testing.T) {
	content, captures := "working", 0
	s := newCaptureCountingSession(t, &content, &captures)

	got, err := s.CapturePaneContent()
	require.NoError(t, err)
	require.Equal(t, "working", got)
	updated, _ := s.HasUpdated()
	require.True(t, updated)
	require.Equal(t, 1, captures, "status check captured a pane the preview had just captured")
}

func TestHasUpdatedCapturesWhenNothingFresh(t *testing.T) {
	content, captures := "working", 0
	s := newCaptureCountingSession(t, &content, &captures)

	updated, _ := s.HasUpdated() // nothing captured yet
	require.True(t, updated)
	require.Equal(t, 1, captures)

	// An old capture isn't reused: the output has moved on since.
	s.lastCaptureAt = s.lastCaptureAt.Add(-2 * statusCaptureMaxAge)
	content = "still working"
	updated, _ = s.HasUpdated()
	require.True(t, updated)
	require.Equal(t, 2, captures)

	// Fresh and unchanged: reused, and reported as not updated.
	updated, _ = s.HasUpdated()
	require.False(t, updated)
	require.Equal(t, 2, captures)
}

func TestCloseForgetsCapture(t *testing.T) {
	content, captures := "old session", 0
	s := newCaptureCountingSession(t, &content, &captures)
	_, err := s.CapturePaneContent()
	require.NoError(t, err)

	_ = s.Close()
	_, _ = s.HasUpdated()
	require.Equal(t, 2, captures, "reused a capture from a closed session")
}
