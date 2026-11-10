//go:build linux

package tmux

import (
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPtyStartReapsExitedProcess(t *testing.T) {
	cmd := exec.Command("true")
	ptmx, err := Pty{}.Start(cmd)
	require.NoError(t, err)
	defer ptmx.Close()

	// An exited process that isn't reaped stays in /proc as a zombie.
	proc := "/proc/" + strconv.Itoa(cmd.Process.Pid)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(proc); os.IsNotExist(err) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("process %d wasn't reaped after it exited", cmd.Process.Pid)
}
