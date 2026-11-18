package app

import (
	"claude-squad/ui"

	tea "github.com/charmbracelet/bubbletea"
)

// Mouse support: clicking a session in the list selects it, and clicking a
// Preview / Diff / Terminal tab switches to it. Hovering either highlights it.
// Clicking the → on the selected or hovered session, or clicking inside the Preview or
// Terminal pane, attaches like Enter. Clicking a session's pull request badge opens the
// pull request. All of it is ignored while an overlay or prompt is open.
//
// View() lays out the list on the left and the tabbed window to its right,
// both one line below the top (PaddingTop(1)).
const viewPaddingTop = 1

// viewPos converts a screen position into a position in View()'s output.
func (m *home) viewPos(x, y int) (int, int) {
	return x - m.viewIndent, y + m.viewOverflow
}

// listItemAt returns the list item at screen position (x, y), or -1.
func (m *home) listItemAt(x, y int) int {
	x, y = m.viewPos(x, y)
	if x < 0 || x >= m.list.Width() {
		return -1
	}
	return m.list.ItemAt(y - viewPaddingTop)
}

// tabAt returns the tab at screen position (x, y), or -1.
func (m *home) tabAt(x, y int) int {
	x, y = m.viewPos(x, y)
	return m.tabbedWindow.TabAt(x-m.list.Width(), y-viewPaddingTop)
}

// openArrowAt returns the list item whose → is at screen position (x, y), or -1.
func (m *home) openArrowAt(x, y int) int {
	x, y = m.viewPos(x, y)
	if x < 0 || x >= m.list.Width() {
		return -1
	}
	return m.list.OpenArrowAt(x, y-viewPaddingTop)
}

// paneOpensAt reports whether a click at screen position (x, y) is inside a pane that
// attaches when clicked: the Preview or Terminal pane of a session that can be attached.
func (m *home) paneOpensAt(x, y int) bool {
	if m.tabbedWindow.IsInDiffTab() {
		return false
	}
	selected := m.list.GetSelectedInstance()
	if selected == nil || !ui.CanAttach(selected) {
		return false
	}
	x, y = m.viewPos(x, y)
	return m.tabbedWindow.ContentAt(x-m.list.Width(), y-viewPaddingTop)
}

// handleMouseHover highlights the list item, open arrow, tab or pane under the pointer
// at (x, y).
func (m *home) handleMouseHover(x, y int) {
	item, tab, open := -1, -1, false
	if m.state == stateDefault {
		item, tab, open = m.listItemAt(x, y), m.tabAt(x, y), m.paneOpensAt(x, y)
	}
	m.list.SetHoverIdx(item) // the arrow shows on the hovered item, so set it first
	// The badge wins where it meets the arrow's target, as it does for clicks.
	onPR := item >= 0 && m.prBadgeAt(x, y) == item
	m.list.SetHover(item, !onPR && item >= 0 && m.openArrowAt(x, y) == item)
	m.list.SetHoverPR(onPR)
	m.tabbedWindow.SetHoverTab(tab)
	m.tabbedWindow.SetHoverOpen(open)
}

// handleMouseClick selects the list item or tab clicked at (x, y), if any.
func (m *home) handleMouseClick(x, y int) tea.Cmd {
	if m.state != stateDefault {
		return nil
	}
	// Before the arrow: its target reaches onto the branch line, where the badge can be.
	if item := m.prBadgeAt(x, y); item >= 0 {
		m.list.SetSelectedInstance(item)
		return tea.Batch(m.instanceChanged(), m.openPullRequest(item))
	}
	if item := m.openArrowAt(x, y); item >= 0 {
		m.list.SetSelectedInstance(item)
		changed := m.instanceChanged()
		return tea.Batch(changed, m.attachSelected())
	}
	if m.paneOpensAt(x, y) {
		return m.attachSelected()
	}
	if item := m.listItemAt(x, y); item >= 0 {
		if m.list.GetSelectedInstance() == m.list.GetInstances()[item] {
			return nil
		}
		m.list.SetSelectedInstance(item)
		return m.instanceChanged()
	}
	tab := m.tabAt(x, y)
	if tab < 0 || tab == m.tabbedWindow.GetActiveTab() {
		return nil
	}
	m.tabbedWindow.SetActiveTab(tab)
	m.menu.SetActiveTab(tab)
	return m.instanceChanged()
}
