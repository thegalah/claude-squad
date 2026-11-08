package app

import (
	"claude-squad/ui"
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/require"
)

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

// screenLines returns what the terminal shows for h's view: like Bubble Tea,
// it drops lines from the top when the view is taller than the terminal.
func screenLines(h *home) []string {
	lines := strings.Split(ansiEscape.ReplaceAllString(h.View(), ""), "\n")
	if len(lines) > h.termHeight {
		lines = lines[len(lines)-h.termHeight:]
	}
	return lines
}

// screenPosition returns the screen cell where text first appears on a line
// that also contains lineMatch.
func screenPosition(t *testing.T, h *home, lineMatch, text string) (x, y int) {
	t.Helper()
	for i, line := range screenLines(h) {
		if strings.Contains(line, lineMatch) && strings.Contains(line, text) {
			return lipgloss.Width(line[:strings.Index(line, text)]), i
		}
	}
	t.Fatalf("%q not found on screen", text)
	return 0, 0
}

// tabPosition returns the screen cell of name in the tab row.
func tabPosition(t *testing.T, h *home, name string) (x, y int) {
	t.Helper()
	return screenPosition(t, h, "Terminal", name)
}

func newTabTestHome(t *testing.T) *home {
	t.Helper()
	s := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	h := &home{
		ctx:          context.Background(),
		spinner:      s,
		list:         ui.NewList(&s, false),
		menu:         ui.NewMenu(),
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewTerminalPane()),
		errBox:       ui.NewErrBox(),
		state:        stateDefault,
	}
	h.updateHandleWindowSizeEvent(tea.WindowSizeMsg{Width: 160, Height: 50})
	return h
}

func click(h *home, x, y int) {
	_, _ = h.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
}

func hover(h *home, x, y int) {
	_, _ = h.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionMotion, Button: tea.MouseButtonNone})
}

func TestClickingTabSelectsIt(t *testing.T) {
	h := newTabTestHome(t)

	for _, tc := range []struct {
		name string
		tab  int
	}{{"Diff", ui.DiffTab}, {"Terminal", ui.TerminalTab}, {"Preview", ui.PreviewTab}} {
		x, y := tabPosition(t, h, tc.name)
		click(h, x, y)
		require.Equal(t, tc.tab, h.tabbedWindow.GetActiveTab(), "clicking %q", tc.name)
	}

	// The tab's borders are part of the tab too.
	x, y := tabPosition(t, h, "Diff")
	click(h, x, y-1)
	require.Equal(t, ui.DiffTab, h.tabbedWindow.GetActiveTab())

	// Clicks outside the tab row, or while an overlay is open, do nothing.
	click(h, x, y+3)
	click(h, 2, y)
	require.Equal(t, ui.DiffTab, h.tabbedWindow.GetActiveTab())

	x, y = tabPosition(t, h, "Terminal")
	h.state = stateHelp
	click(h, x, y)
	require.Equal(t, ui.DiffTab, h.tabbedWindow.GetActiveTab())
}

func TestHoveringTabHighlightsIt(t *testing.T) {
	// Tests don't run in a TTY, so lipgloss would strip all styling.
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	h := newTabTestHome(t)

	plain := h.View()
	diffX, diffY := tabPosition(t, h, "Diff")
	previewX, previewY := tabPosition(t, h, "Preview")

	hover(h, diffX, diffY)
	hovered := h.View()
	require.NotEqual(t, plain, hovered, "hovering an inactive tab should highlight it")
	require.Equal(t, ui.PreviewTab, h.tabbedWindow.GetActiveTab(), "hovering must not switch tabs")

	// Hovering the active tab looks the same as not hovering.
	hover(h, previewX, previewY)
	require.Equal(t, plain, h.View())

	// Moving off the tabs clears the highlight.
	hover(h, diffX, diffY)
	hover(h, diffX, diffY+5)
	require.Equal(t, plain, h.View())

	// No highlight while an overlay is open.
	h.state = stateHelp
	hover(h, diffX, diffY)
	h.state = stateDefault
	require.Equal(t, plain, h.View())
}
