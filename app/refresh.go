package app

import (
	"claude-squad/log"
	"claude-squad/session"
	"claude-squad/session/git"
	"claude-squad/ui"
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
	syncCmd := m.syncSelection()
	tab := m.tabbedWindow.GetActiveTab()
	return tea.Batch(syncCmd, func() tea.Msg {
		content, err := capture()
		return previewCapturedMsg{instance: selected, tab: tab, content: content, err: err}
	})
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

// syncSelection updates everything that follows the selected instance and active tab
// without slow work. When either changed and the Diff tab is showing, it returns a
// command that computes the selected instance's full diff right away, rather than leaving
// the pane empty until the next metadata pass.
func (m *home) syncSelection() tea.Cmd {
	selected := m.list.GetSelectedInstance()
	tab := m.tabbedWindow.GetActiveTab()
	var cmd tea.Cmd
	if selected != m.lastSelected || tab != m.lastTab {
		if selected != m.lastSelected {
			// Scroll mode freezes a pane on the instance it was entered for.
			m.tabbedWindow.ExitScrollModes()
		}
		if tab == ui.DiffTab {
			cmd = computeDiffCmd(selected)
		}
		m.lastSelected = selected
		m.lastTab = tab
	}

	m.tabbedWindow.UpdateDiff(selected)
	m.tabbedWindow.SetInstance(selected)
	m.menu.SetInstance(selected)
	return cmd
}

// diffComputedMsg carries a full diff computed in the background for instance.
type diffComputedMsg struct {
	instance *session.Instance
	stats    *git.DiffStats
}

// computeDiffCmd computes inst's full diff in the background, or returns nil if inst has
// no worktree to diff right now.
func computeDiffCmd(inst *session.Instance) tea.Cmd {
	if !hasLiveSession(inst) {
		return nil
	}
	return func() tea.Msg {
		return diffComputedMsg{instance: inst, stats: inst.ComputeDiff()}
	}
}

func (m *home) handleDiffComputed(msg diffComputedMsg) tea.Cmd {
	if !hasLiveSession(msg.instance) {
		return nil
	}
	setDiffStats(msg.instance, msg.stats)
	if msg.instance == m.list.GetSelectedInstance() {
		m.tabbedWindow.UpdateDiff(msg.instance)
	}
	return nil
}

// hasLiveSession reports whether inst has a running session and worktree that nothing
// is stopping or starting.
func hasLiveSession(inst *session.Instance) bool {
	return inst != nil && inst.Started() && !inst.Paused() &&
		inst.Status != session.Loading && !inst.Busy().StopsSession()
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
	selected := m.list.GetSelectedInstance()
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

		// The pass only has counts for instances that weren't selected when it began. If
		// one is selected now, it already has (or is getting) its full diff; keep that
		// rather than blanking the Diff tab until the next pass.
		if !r.full && inst == selected {
			if cur := inst.GetDiffStats(); cur != nil && cur.Content != "" {
				continue
			}
		}
		setDiffStats(inst, r.diffStats)
	}
}
