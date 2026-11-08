package ui

import (
	"testing"

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
