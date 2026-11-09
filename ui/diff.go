package ui

import (
	"claude-squad/session"
	"claude-squad/session/git"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

var (
	AdditionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))
	DeletionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ef4444"))
	BinaryStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#808080"))
)

// DiffPane lists the files a session changed, with their added and removed lines.
type DiffPane struct {
	viewport viewport.Model
	// files is the list on display, kept to re-render it when the pane is resized.
	files  []git.FileChange
	width  int
	height int
}

func NewDiffPane() *DiffPane {
	return &DiffPane{
		viewport: viewport.New(0, 0),
	}
}

func (d *DiffPane) SetSize(width, height int) {
	d.width = width
	d.height = height
	d.viewport.Width = width
	d.viewport.Height = height
	if len(d.files) > 0 {
		d.viewport.SetContent(renderFileList(d.files, width))
	}
}

func (d *DiffPane) SetDiff(instance *session.Instance) {
	d.files = nil
	centered := func(message string) {
		d.viewport.SetContent(lipgloss.Place(d.width, d.height, lipgloss.Center, lipgloss.Center, message))
	}

	if instance == nil || !instance.Started() {
		centered("No changes")
		return
	}
	stats := instance.GetDiffStats()
	switch {
	case stats == nil:
		// The worktree isn't ready yet.
		centered("Setting up worktree...")
	case stats.Error != nil:
		centered(fmt.Sprintf("Error: %v", stats.Error))
	case stats.IsEmpty():
		centered("No changes")
	default:
		d.files = stats.Files
		d.viewport.SetContent(renderFileList(stats.Files, d.width))
	}
}

// renderFileList renders a summary line and one line per changed file, e.g.
//
//	2 files changed  +14 -3
//
//	  +12  -1  app/app.go
//	   +2  -2  ui/diff.go
//
// Paths too long for width keep their end, which holds the file name.
func renderFileList(files []git.FileChange, width int) string {
	var added, removed int
	addW, remW := 2, 2
	for _, f := range files {
		added += f.Added
		removed += f.Removed
		addW = max(addW, len(fmt.Sprintf("+%d", f.Added)))
		remW = max(remW, len(fmt.Sprintf("-%d", f.Removed)))
	}

	noun := "files"
	if len(files) == 1 {
		noun = "file"
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%d %s changed  %s %s\n\n", len(files), noun,
		AdditionStyle.Render(fmt.Sprintf("+%d", added)), DeletionStyle.Render(fmt.Sprintf("-%d", removed))))

	countsW := addW + 1 + remW
	pathW := width - 2 - countsW - 2
	for _, f := range files {
		var counts string
		if f.Binary {
			counts = BinaryStyle.Render(fmt.Sprintf("%*s", countsW, "binary"))
		} else {
			counts = AdditionStyle.Render(fmt.Sprintf("%*s", addW, fmt.Sprintf("+%d", f.Added))) + " " +
				DeletionStyle.Render(fmt.Sprintf("%*s", remW, fmt.Sprintf("-%d", f.Removed)))
		}
		b.WriteString("  " + counts + "  " + truncateLeft(f.Path, pathW) + "\n")
	}
	return b.String()
}

// truncateLeft shortens s to width cells by dropping its start, e.g. "…/deep/file.go".
func truncateLeft(s string, width int) string {
	if width <= 0 || runewidth.StringWidth(s) <= width {
		return s
	}
	runes := []rune(s)
	for i := range runes {
		if tail := string(runes[i:]); runewidth.StringWidth(tail)+1 <= width {
			return "…" + tail
		}
	}
	return "…"
}

func (d *DiffPane) String() string {
	return d.viewport.View()
}

// ScrollUp scrolls the viewport up
func (d *DiffPane) ScrollUp() {
	d.viewport.LineUp(1)
}

// ScrollDown scrolls the viewport down
func (d *DiffPane) ScrollDown() {
	d.viewport.LineDown(1)
}
