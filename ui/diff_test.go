package ui

import (
	"claude-squad/session/git"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/require"
)

func TestRenderFileList(t *testing.T) {
	asciiColors(t)

	got := renderFileList([]git.FileChange{
		{Path: "app/app.go", Added: 120, Removed: 4},
		{Path: "ui/diff.go", Added: 2, Removed: 31},
		{Path: "logo.png", Binary: true},
	}, 80)

	require.Equal(t, strings.Join([]string{
		"3 files changed  +122 -35",
		"",
		"  +120  -4  app/app.go",
		"    +2 -31  ui/diff.go",
		"    binary  logo.png",
		"",
	}, "\n"), got)
}

func TestRenderFileListSingular(t *testing.T) {
	asciiColors(t)

	got := renderFileList([]git.FileChange{{Path: "a.go", Added: 1}}, 80)
	require.True(t, strings.HasPrefix(got, "1 file changed  +1 -0\n"), got)
}

func TestTruncateLeftKeepsFileName(t *testing.T) {
	require.Equal(t, "short.go", truncateLeft("short.go", 20))
	require.Equal(t, "…/deep/file.go", truncateLeft("very/long/path/to/deep/file.go", 14))
	require.Equal(t, "…", truncateLeft("abc", 1))
}

// asciiColors renders without colors for the rest of the test.
func asciiColors(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
}
