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
	"github.com/stretchr/testify/require"
)

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

// tabPosition returns the screen cell of name in the tab row of view.
func tabPosition(t *testing.T, view, name string) (x, y int) {
	t.Helper()
	for i, line := range strings.Split(ansiEscape.ReplaceAllString(view, ""), "\n") {
		if strings.Contains(line, "Preview") && strings.Contains(line, "Terminal") {
			idx := strings.Index(line, name)
			require.GreaterOrEqual(t, idx, 0)
			return lipgloss.Width(line[:idx]), i
		}
	}
	t.Fatalf("tab row not found in view")
	return 0, 0
}

func TestClickingTabSelectsIt(t *testing.T) {
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

	click := func(x, y int) {
		_, _ = h.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	}

	for _, tc := range []struct {
		name string
		tab  int
	}{{"Diff", ui.DiffTab}, {"Terminal", ui.TerminalTab}, {"Preview", ui.PreviewTab}} {
		x, y := tabPosition(t, h.View(), tc.name)
		click(x, y)
		require.Equal(t, tc.tab, h.tabbedWindow.GetActiveTab(), "clicking %q", tc.name)
	}

	// The tab's borders are part of the tab too.
	x, y := tabPosition(t, h.View(), "Diff")
	click(x, y-1)
	require.Equal(t, ui.DiffTab, h.tabbedWindow.GetActiveTab())

	// Clicks outside the tab row, or while an overlay is open, do nothing.
	click(x, y+3)
	click(2, y)
	require.Equal(t, ui.DiffTab, h.tabbedWindow.GetActiveTab())

	x, y = tabPosition(t, h.View(), "Terminal")
	h.state = stateHelp
	click(x, y)
	require.Equal(t, ui.DiffTab, h.tabbedWindow.GetActiveTab())
}
