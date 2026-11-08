package app

import (
	"claude-squad/config"
	"claude-squad/session"
	"claude-squad/session/git"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Pressing P on the main screen does what `cs prune` does: it finds worktree
// directories that no saved session (in any repo) uses, asks for confirmation
// and removes them, keeping their branches. As without --force, worktrees with
// uncommitted changes or a detached HEAD are kept.

// pruneScanMsg carries the leftover worktrees found by pruneScanCmd.
type pruneScanMsg struct {
	leftovers []git.LeftoverWorktree
	err       error
}

// pruneDoneMsg reports the result of removing leftover worktrees.
type pruneDoneMsg struct {
	removed  int
	failures []string
}

// pruneFuncs are the operations prune uses, replaceable in tests.
type pruneFuncs struct {
	find   func() ([]git.LeftoverWorktree, error)
	remove func(git.LeftoverWorktree) error
}

var pruneOps = pruneFuncs{
	find: func() ([]git.LeftoverWorktree, error) {
		tracked, err := session.TrackedWorktreePaths(config.LoadState())
		if err != nil {
			return nil, err
		}
		return git.FindLeftoverWorktrees(tracked, git.LeftoverMinAge, true)
	},
	remove: git.RemoveLeftoverWorktree,
}

// pruneNoticeDuration is how long prune results stay in the info bar.
const pruneNoticeDuration = 6 * time.Second

func pruneScanCmd() tea.Msg {
	leftovers, err := pruneOps.find()
	return pruneScanMsg{leftovers: leftovers, err: err}
}

func pruneRemoveCmd(toRemove []git.LeftoverWorktree) tea.Cmd {
	return func() tea.Msg {
		var done pruneDoneMsg
		for _, w := range toRemove {
			if err := pruneOps.remove(w); err != nil {
				done.failures = append(done.failures, err.Error())
				continue
			}
			done.removed++
		}
		return done
	}
}

// startPrune begins a prune from the main screen.
func (m *home) startPrune() tea.Cmd {
	m.errBox.SetInfo("Looking for leftover worktrees…")
	return pruneScanCmd
}

// handlePruneScan asks to remove the leftovers that are safe to remove.
func (m *home) handlePruneScan(msg pruneScanMsg) tea.Cmd {
	if msg.err != nil {
		m.errBox.Clear()
		return m.handleError(fmt.Errorf("could not look for leftover worktrees: %w", msg.err))
	}

	var toRemove []git.LeftoverWorktree
	for _, w := range msg.leftovers {
		if !w.NeedsForce() {
			toRemove = append(toRemove, w)
		}
	}
	kept := len(msg.leftovers) - len(toRemove)

	if len(toRemove) == 0 {
		if kept == 0 {
			return m.showInfoFor("No leftover worktrees.", pruneNoticeDuration)
		}
		return m.showInfoFor(fmt.Sprintf(
			"%d leftover worktree(s) have uncommitted changes or a detached HEAD. Run 'cs prune --force' to remove them.",
			kept), pruneNoticeDuration)
	}
	if m.state != stateDefault {
		// Something else was opened while scanning; don't cover it with a prompt.
		m.errBox.Clear()
		return nil
	}

	m.errBox.Clear()
	message := fmt.Sprintf("[!] Remove %d leftover worktree(s)?\n\nBranches are kept.", len(toRemove))
	if kept > 0 {
		message += fmt.Sprintf(" Skipping %d with uncommitted changes or a detached HEAD.", kept)
	}
	m.confirmActionAsync(message, func() tea.Cmd {
		m.errBox.SetInfo(fmt.Sprintf("Removing %d leftover worktree(s)…", len(toRemove)))
		return pruneRemoveCmd(toRemove)
	})
	return nil
}

func (m *home) handlePruneDone(msg pruneDoneMsg) tea.Cmd {
	if len(msg.failures) > 0 {
		m.errBox.Clear()
		return m.handleErrorFor(fmt.Errorf("removed %d, failed to remove %d worktree(s): %s",
			msg.removed, len(msg.failures), strings.Join(msg.failures, "; ")), pruneNoticeDuration)
	}
	return m.showInfoFor(fmt.Sprintf("Removed %d leftover worktree(s). Branches were kept.", msg.removed),
		pruneNoticeDuration)
}

// showInfoFor shows notice in the info bar for d.
func (m *home) showInfoFor(notice string, d time.Duration) tea.Cmd {
	m.errBox.SetInfo(notice)
	return func() tea.Msg {
		select {
		case <-m.ctx.Done():
		case <-time.After(d):
		}
		return hideErrMsg{}
	}
}
