package tmux

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"

	cmd2 "claude-squad/cmd"
	"claude-squad/cmd/cmd_test"

	"github.com/stretchr/testify/require"
)

func press(col, row int) string   { return fmt.Sprintf("\x1b[<0;%d;%dM", col, row) }
func drag(col, row int) string    { return fmt.Sprintf("\x1b[<32;%d;%dM", col, row) }
func release(col, row int) string { return fmt.Sprintf("\x1b[<0;%d;%dm", col, row) }
func hover(col, row int) string   { return fmt.Sprintf("\x1b[<35;%d;%dM", col, row) }

// feed filters each read in turn and returns what was forwarded and whether it detached.
func feed(reads ...string) (forwarded string, detached bool) {
	var a attachInput
	for _, r := range reads {
		fwd, detach := a.filter([]byte(r))
		forwarded += string(fwd)
		if detach {
			return forwarded, true
		}
	}
	return forwarded, false
}

func TestBackButtonClickDetaches(t *testing.T) {
	fwd, detached := feed(hover(5, 3), press(3, 1), release(3, 1))
	require.True(t, detached)
	require.Equal(t, hover(5, 3), fwd, "the click must not reach tmux")
}

func TestBackButtonClickSplitAcrossReads(t *testing.T) {
	p, r := press(4, 1), release(4, 1)
	click := p + r
	prefix := len(sgrMousePrefix)
	for i := prefix; i < len(click); i++ {
		if i > len(p) && i < len(p)+prefix {
			continue // splits inside the "ESC [ <" prefix aren't handled: see attachInput.pending
		}
		fwd, detached := feed("ab"+click[:i], click[i:])
		require.True(t, detached, "split at %d", i)
		require.Equal(t, "ab", fwd, "split at %d", i)
	}
}

func TestBackButtonReleasedElsewhereDoesNothing(t *testing.T) {
	fwd, detached := feed(press(2, 1), drag(20, 5), release(20, 5), "x")
	require.False(t, detached)
	require.Equal(t, "x", fwd, "the whole drag started on the button is swallowed")
}

func TestClicksOffTheBackButtonAreForwarded(t *testing.T) {
	for _, click := range []string{
		press(30, 1) + release(30, 1),   // the rest of the status line
		press(3, 2) + release(3, 2),     // the pane, just below the button
		"\x1b[<2;3;1M" + "\x1b[<2;3;1m", // right button
		"\x1b[<64;3;1M",                 // wheel
		"\x1b[<0;3;1X",                  // not a mouse report
		"\x1b[A\x1b[<",                  // keys, then a cut-off report at the end
	} {
		fwd, detached := feed(click)
		require.False(t, detached, "%q", click)
		if strings.HasSuffix(click, "\x1b[<") {
			require.Equal(t, "\x1b[A", fwd, "the cut-off report waits for the next read")
			continue
		}
		require.Equal(t, click, fwd, "%q", click)
	}
}

func TestShiftClickOnBackButtonDetaches(t *testing.T) {
	_, detached := feed("\x1b[<4;3;1M", "\x1b[<4;3;1m")
	require.True(t, detached)
}

func TestCtrlQStillDetachesWithMouseInput(t *testing.T) {
	fwd, detached := feed(hover(5, 5) + "\x11" + hover(6, 6))
	require.True(t, detached)
	require.Equal(t, hover(5, 5), fwd)
}

func TestOverlongReportIsForwarded(t *testing.T) {
	junk := "\x1b[<" + strings.Repeat("1;", 20)
	fwd, detached := feed(junk, "z")
	require.False(t, detached)
	require.Equal(t, junk+"z", fwd)
}

func TestShowBackButtonSetsStatusLine(t *testing.T) {
	var ran []string
	exec := cmd_test.MockCmdExec{
		RunFunc: func(c *exec.Cmd) error { ran = append(ran, cmd2.ToString(c)); return nil },
	}
	s := newTmuxSession("my session", "claude", nil, exec)
	s.showBackButton()
	require.Len(t, ran, 1, "one tmux call")
	require.Contains(t, ran[0], "status-position top")
	require.Contains(t, ran[0], "← back")
	require.Contains(t, ran[0], "-t claudesquad_mysession")
	require.Contains(t, ran[0], "set-titles off")
}
