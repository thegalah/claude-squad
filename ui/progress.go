package ui

import (
	"claude-squad/session"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

var progressTitleStyle = lipgloss.NewStyle().Bold(true).
	Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#dddddd"})

var progressDimStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#777777"})

var progressDoneStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#51bd73", Dark: "#51bd73"})

var progressLogStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#999999"})

// maxSetupLogLines caps how many lines of setup script output the preview shows.
const maxSetupLogLines = 12

// renderStartProgress renders the steps of starting an instance, with the current one
// animated and timed, followed by the tail of the setup script's output while it runs.
func (p *PreviewPane) renderStartProgress(instance *session.Instance) string {
	prog := instance.StartProgress()
	now := time.Now()

	title := progressTitleStyle.Render("Setting up workspace")
	if !prog.StartedAt.IsZero() {
		title += progressDimStyle.Render("  " + session.FormatElapsed(now.Sub(prog.StartedAt)))
	}

	spin := "▸"
	if p.spinner != nil {
		spin = p.spinner.View()
	}

	stages := prog.Stages()
	current := -1
	for idx, st := range stages {
		if st == prog.Stage {
			current = idx
		}
	}
	var steps []string
	for idx, st := range stages {
		label := session.StageLabel(st, instance.Program)
		switch {
		case current >= 0 && idx < current:
			steps = append(steps, progressDoneStyle.Render("✓")+" "+label+
				progressDimStyle.Render("  "+session.FormatElapsed(prog.StageDurations[st])))
		case idx == current:
			steps = append(steps, spin+" "+label+
				progressDimStyle.Render("  "+session.FormatElapsed(now.Sub(prog.StageStartedAt))))
		default:
			steps = append(steps, progressDimStyle.Render("· "+label))
		}
	}

	parts := []string{title, "", lipgloss.JoinVertical(lipgloss.Left, steps...)}

	if prog.Stage == session.StageSetupScript && prog.SetupLogPath != "" {
		// Leave room for the title and steps above, plus the pane's borders and padding.
		room := p.height - 7 - len(parts) - len(steps) - 3
		n := min(room, maxSetupLogLines)
		width := min(p.width-4, 100)
		if n > 0 && width > 10 {
			header := progressDimStyle.Render("Output: " + shortenHome(prog.SetupLogPath))
			if runewidth.StringWidth(header) > width {
				header = progressDimStyle.Render("Output:")
			}
			logLines := session.TailLog(prog.SetupLogPath, n)
			block := []string{header, ""}
			if len(logLines) == 0 {
				block = append(block, progressLogStyle.Render("(no output yet)"))
			}
			for _, line := range logLines {
				block = append(block, progressLogStyle.Render(runewidth.Truncate(line, width, "…")))
			}
			// Pad every line to the same width so the block stays left-aligned when the
			// pane centers it.
			parts = append(parts, "", lipgloss.NewStyle().Width(width).Render(
				lipgloss.JoinVertical(lipgloss.Left, block...)))
		}
	}

	return lipgloss.JoinVertical(lipgloss.Center, parts...)
}

// shortenHome replaces the home directory prefix of path with "~".
func shortenHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if rest, ok := strings.CutPrefix(path, home); ok {
		return "~" + rest
	}
	return path
}
