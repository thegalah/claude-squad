package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/require"
)

func TestExitScrollModes(t *testing.T) {
	w := NewTabbedWindow(NewPreviewPane(), NewDiffPane(), NewTerminalPane())
	w.preview.isScrolling = true
	w.terminal.isScrolling = true

	w.ExitScrollModes()
	require.False(t, w.IsPreviewInScrollMode())
	require.False(t, w.IsTerminalInScrollMode())
}

func TestContentAtMatchesRenderedPane(t *testing.T) {
	w := NewTabbedWindow(NewPreviewPane(), NewDiffPane(), NewTerminalPane())
	w.SetSize(100, 30)
	lines := strings.Split(w.String(), "\n")
	tabHeight := activeTabStyle.GetVerticalFrameSize() + 1
	paneTop := tabRowOffset + tabHeight

	for y := range lines {
		require.Equal(t, y >= paneTop, w.ContentAt(5, y), "line %d: %q", y, lines[y])
	}
	require.False(t, w.ContentAt(5, len(lines)), "below the pane")
	require.False(t, w.ContentAt(lipgloss.Width(lines[paneTop]), paneTop), "right of the pane")
	require.True(t, w.ContentAt(lipgloss.Width(lines[paneTop])-1, paneTop))
}

func TestHoverOpenAddsHintToActiveTab(t *testing.T) {
	w := NewTabbedWindow(NewPreviewPane(), NewDiffPane(), NewTerminalPane())
	w.SetSize(100, 30)
	require.NotContains(t, w.String(), "click to open")
	w.SetHoverOpen(true)
	require.Contains(t, w.String(), "Preview → click to open")

	w.SetSize(40, 30) // too narrow for the hint: the tab keeps its name
	require.NotContains(t, w.String(), "click to open")
	require.Contains(t, w.String(), "Preview")
}

func TestStringRerendersWhenSomethingChanges(t *testing.T) {
	w := NewTabbedWindow(NewPreviewPane(), NewDiffPane(), NewTerminalPane())
	w.SetSize(100, 30)
	w.preview.SetLiveContent("first")
	require.Contains(t, w.String(), "first")
	require.Contains(t, w.String(), "first", "unchanged: the cached render")

	w.preview.SetLiveContent("second")
	require.Contains(t, w.String(), "second")
	require.NotContains(t, w.String(), "first")

	w.SetHoverOpen(true)
	require.Contains(t, w.String(), "click to open")
	before := lipgloss.Height(w.String())
	w.SetSize(100, 20)
	require.Equal(t, before-10, lipgloss.Height(w.String()), "resizing re-renders")
}
