package app

import (
	"claude-squad/ui"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// helpNotSeen is a config.AppState that hasn't seen any help screen, so attaching stops
// at the attach help screen instead of attaching for real.
type helpNotSeen struct{ seen uint32 }

func (s *helpNotSeen) GetHelpScreensSeen() uint32        { return s.seen }
func (s *helpNotSeen) SetHelpScreensSeen(v uint32) error { s.seen = v; return nil }

// attaching reports whether h started attaching: it's showing the attach help screen.
func attaching(h *home) bool {
	return h.state == stateHelp && strings.Contains(h.View(), "Attaching to Instance")
}

// lineWith returns the screen line holding text.
func lineWith(t *testing.T, h *home, text string) string {
	t.Helper()
	for _, line := range screenLines(h) {
		if strings.Contains(line, text) {
			return line
		}
	}
	t.Fatalf("%q not on screen", text)
	return ""
}

// listPart is the list's part of a screen line, left of the preview.
func listPart(h *home, line string) string {
	runes := []rune(line)
	end := min(len(runes), h.viewIndent+h.list.Width())
	return string(runes[h.viewIndent:end])
}

func TestOpenArrowShowsOnSelectedAndHoveredSessions(t *testing.T) {
	alpha, _ := startedInstance(t, "alpha", true)
	bravo, _ := startedInstance(t, "bravo", true)
	h := newResponsiveTestHome(t, alpha, bravo)

	require.Contains(t, listPart(h, lineWith(t, h, "alpha")), "→", "the selected session has an arrow")
	require.NotContains(t, listPart(h, lineWith(t, h, "bravo")), "→")

	x, y := screenPosition(t, h, "bravo", "bravo")
	hover(h, x, y)
	require.Contains(t, listPart(h, lineWith(t, h, "bravo")), "→", "so does the hovered one")
}

func TestClickingOpenArrowAttaches(t *testing.T) {
	alpha, _ := startedInstance(t, "alpha", true)
	bravo, _ := startedInstance(t, "bravo", true)
	h := newResponsiveTestHome(t, alpha, bravo)
	h.appState = &helpNotSeen{}

	// Clicking the row itself only selects it.
	x, y := screenPosition(t, h, "bravo", "bravo")
	click(h, x, y)
	require.Equal(t, "bravo", selectedTitle(h))
	require.Equal(t, stateDefault, h.state)

	// The arrow attaches, also from an unselected session.
	x, y = screenPosition(t, h, "alpha", "alpha")
	hover(h, x, y)
	ax, ay := screenPosition(t, h, "alpha", "→")
	hover(h, ax, ay)
	require.Equal(t, 0, h.list.OpenArrowAt(ax-h.viewIndent, ay-viewPaddingTop+h.viewOverflow), "the arrow's cell is its target")
	click(h, ax, ay)
	require.Equal(t, "alpha", selectedTitle(h))
	require.True(t, attaching(h))
}

func TestPausedSessionHasNoOpenArrow(t *testing.T) {
	alpha, _ := startedInstance(t, "alpha", false) // its tmux session is gone: paused
	h := newResponsiveTestHome(t, alpha)
	h.appState = &helpNotSeen{}

	require.NotContains(t, listPart(h, lineWith(t, h, "alpha")), "→")
	_, y := screenPosition(t, h, "alpha", "alpha")
	for x := 0; x < h.list.Width(); x++ {
		click(h, x, y)
	}
	require.Equal(t, stateDefault, h.state)
}

func TestClickingPreviewAttaches(t *testing.T) {
	alpha, pane := startedInstance(t, "alpha", true)
	pane.content.Store("hello from alpha")
	h := newResponsiveTestHome(t, alpha)
	h.Update(findMsg[previewCapturedMsg](t, runCmd(h.refreshPreview())))
	h.appState = &helpNotSeen{}

	x, y := screenPosition(t, h, "hello from alpha", "hello")
	hover(h, x, y)
	require.Contains(t, h.View(), "Preview → click to open")
	hover(h, 0, 0)
	require.NotContains(t, h.View(), "click to open")

	click(h, x, y)
	require.True(t, attaching(h))
}

func TestClickingDiffPaneDoesNotAttach(t *testing.T) {
	alpha, _ := startedInstance(t, "alpha", true)
	h := newResponsiveTestHome(t, alpha)
	h.appState = &helpNotSeen{}
	h.tabbedWindow.SetActiveTab(ui.DiffTab)

	_, y := tabPosition(t, h, "Diff")
	x := h.list.Width() + 5
	hover(h, x, y+5)
	require.NotContains(t, h.View(), "click to open")
	click(h, x, y+5)
	require.Equal(t, stateDefault, h.state)
}
