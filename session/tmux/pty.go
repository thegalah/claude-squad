package tmux

import (
	"os"
	"os/exec"

	"github.com/creack/pty"
)

type PtyFactory interface {
	Start(cmd *exec.Cmd) (*os.File, error)
	Close()
}

// Pty starts a "real" pseudo-terminal (PTY) using the creack/pty package.
type Pty struct{}

// Start starts cmd on a new PTY. The process is reaped when it exits (tmux new-session -d
// exits once the session is up, and attach-session when its PTY is closed on detach), so
// it doesn't linger as a zombie.
func (pt Pty) Start(cmd *exec.Cmd) (*os.File, error) {
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}
	go func() { _ = cmd.Wait() }()
	return ptmx, nil
}

func (pt Pty) Close() {}

func MakePtyFactory() PtyFactory {
	return Pty{}
}
