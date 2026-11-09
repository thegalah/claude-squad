package app

import (
	"claude-squad/log"
	"claude-squad/session"
	"claude-squad/session/git"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// How often the selected session's pane is re-captured, and how often every session's
// status and diff stats are refreshed. Both loops only reschedule once a pass is done,
// so slow tmux or git calls can't pile up.
const (
	previewInterval  = 50 * time.Millisecond
	metadataInterval = 250 * time.Millisecond
)

// readyAfter is how long a session's pane must stay unchanged before it's shown as Ready
// rather than Running. It's longer than metadataInterval so a program that redraws a
// little less often than that doesn't flicker between the two.
const readyAfter = 500 * time.Millisecond

// previewTickMsg triggers a refresh of the selected session's pane.
type previewTickMsg struct{}

func previewTickCmd() tea.Cmd {
	return tea.Tick(previewInterval, func(time.Time) tea.Msg { return previewTickMsg{} })
}

// previewCapturedMsg carries pane content captured in the background for instance while
// tab was the active tab.
type previewCapturedMsg struct {
	instance *session.Instance
	tab      int
	content  string
	err      error
}

// refreshPreview refreshes the panes for the selected instance. The tmux capture, the
// only slow part, runs in the background; the next tick is scheduled once it's applied.
func (m *home) refreshPreview() tea.Cmd {
	selected := m.list.GetSelectedInstance()
	capture := m.tabbedWindow.LiveCapturer(selected)
	if capture == nil {
		// Nothing live to capture: the active tab updates without touching tmux.
		return tea.Batch(m.instanceChanged(), previewTickCmd())
	}
	m.syncSelection()
	tab := m.tabbedWindow.GetActiveTab()
	return func() tea.Msg {
		content, err := capture()
		return previewCapturedMsg{instance: selected, tab: tab, content: content, err: err}
	}
}

// handlePreviewCaptured shows a background capture, unless the selection or tab changed
// while it ran, and schedules the next tick.
func (m *home) handlePreviewCaptured(msg previewCapturedMsg) tea.Cmd {
	if msg.instance != m.list.GetSelectedInstance() || msg.tab != m.tabbedWindow.GetActiveTab() {
		return previewTickCmd()
	}
	if msg.err != nil {
		// The session may have just gone away; the synchronous path works out what to
		// show instead (and reports the error if it persists).
		return tea.Batch(m.instanceChanged(), previewTickCmd())
	}
	m.tabbedWindow.SetLiveContent(msg.tab, msg.instance, msg.content)
	return previewTickCmd()
}

// syncSelection updates everything that follows the selected instance without slow work.
func (m *home) syncSelection() {
	selected := m.list.GetSelectedInstance()
	if selected != m.lastSelected {
		// Scroll mode freezes a pane on the instance it was entered for.
		m.tabbedWindow.ExitScrollModes()
		m.lastSelected = selected
	}

	m.tabbedWindow.UpdateDiff(selected)
	m.tabbedWindow.SetInstance(selected)
	m.menu.SetInstance(selected)
}

// setDiffStats stores stats computed in the background on inst.
func setDiffStats(inst *session.Instance, stats *git.DiffStats) {
	if stats != nil && stats.Error != nil {
		if !strings.Contains(stats.Error.Error(), "base commit SHA not set") {
			log.WarningLog.Printf("could not update diff stats: %v", stats.Error)
		}
		inst.SetDiffStats(nil)
		return
	}
	inst.SetDiffStats(stats)
}

// applyMetadata applies a metadata pass's results: statuses, auto-yes and diff stats.
func (m *home) applyMetadata(results []instanceMetaResult, now time.Time) {
	if m.lastOutputChange == nil {
		m.lastOutputChange = make(map[*session.Instance]time.Time)
	}
	listed := make(map[*session.Instance]bool)
	for _, inst := range m.list.GetInstances() {
		listed[inst] = true
	}
	for _, r := range results {
		inst := r.instance
		// Skip instances killed, paused, or being paused etc., while the pass ran.
		if !listed[inst] || inst.Paused() || inst.Busy() != session.NotBusy {
			continue
		}
		if r.updated {
			m.lastOutputChange[inst] = now
			inst.SetStatus(session.Running)
		} else if r.hasPrompt {
			inst.TapEnter()
		} else if now.Sub(m.lastOutputChange[inst]) >= readyAfter {
			inst.SetStatus(session.Ready)
		}
		setDiffStats(inst, r.diffStats)
	}
}
