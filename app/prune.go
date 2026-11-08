package app

import (
	"claude-squad/config"
	"claude-squad/session"
	"claude-squad/session/git"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Pressing shift+p on the main screen does what `cs prune` does: it finds
// worktree directories that no saved session (in any repo) uses, asks for
// confirmation and removes them, keeping their branches. Worktrees whose removal
// could lose work (uncommitted changes, or a detached HEAD on a commit no branch
// has) get a second confirmation that lists what would be lost, the TUI's
// equivalent of `cs prune --force`.

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

// handlePruneScan asks to remove the leftovers: first those that are safe to
// remove, then, separately, those with unsaved work.
func (m *home) handlePruneScan(msg pruneScanMsg) tea.Cmd {
	if msg.err != nil {
		m.errBox.Clear()
		return m.handleError(fmt.Errorf("could not look for leftover worktrees: %w", msg.err))
	}

	var safe, unsaved []git.LeftoverWorktree
	for _, w := range msg.leftovers {
		if w.NeedsForce() {
			unsaved = append(unsaved, w)
		} else {
			safe = append(safe, w)
		}
	}

	if len(safe) == 0 && len(unsaved) == 0 {
		return m.showInfoFor("No leftover worktrees.", pruneNoticeDuration)
	}
	if m.state != stateDefault {
		// Something else was opened while scanning; don't cover it with a prompt.
		m.errBox.Clear()
		return nil
	}
	m.errBox.Clear()

	if len(safe) == 0 {
		m.confirmPruneUnsaved(unsaved)
		return nil
	}
	message := fmt.Sprintf("[!] Remove %d leftover worktree(s)?\n\nBranches are kept.", len(safe))
	if len(unsaved) > 0 {
		message += fmt.Sprintf(" %d more have unsaved work; you'll be asked about them next.", len(unsaved))
	}
	m.confirmActionAsync(message, func() tea.Cmd {
		m.errBox.SetInfo(fmt.Sprintf("Removing %d leftover worktree(s)…", len(safe)))
		if len(unsaved) > 0 {
			m.confirmPruneUnsaved(unsaved)
		}
		return pruneRemoveCmd(safe)
	})
	return nil
}

// confirmPruneUnsaved asks to remove leftovers whose removal loses work, naming
// each one and what it has.
func (m *home) confirmPruneUnsaved(unsaved []git.LeftoverWorktree) {
	var b strings.Builder
	fmt.Fprintf(&b, "[!] Remove %d leftover worktree(s) with unsaved work?\n", len(unsaved))
	for _, w := range unsaved {
		fmt.Fprintf(&b, "\n• %s: %s", leftoverName(w), describeUnsaved(w))
	}
	b.WriteString("\n\nThis work will be lost. Branches are kept.")
	m.confirmActionAsync(b.String(), func() tea.Cmd {
		m.errBox.SetInfo(fmt.Sprintf("Removing %d leftover worktree(s)…", len(unsaved)))
		return pruneRemoveCmd(unsaved)
	})
}

// leftoverName is a short name for a leftover worktree: its branch, or its
// directory name without cs's timestamp suffix.
func leftoverName(w git.LeftoverWorktree) string {
	if w.Branch != "" {
		return w.Branch
	}
	name := filepath.Base(w.Path)
	if i := strings.LastIndexByte(name, '_'); i > 0 {
		name = name[:i]
	}
	return name
}

func describeUnsaved(w git.LeftoverWorktree) string {
	var parts []string
	if w.Dirty {
		parts = append(parts, git.DescribeChanges(w.Changes))
	}
	if w.Unreachable {
		parts = append(parts, "commits on no branch")
	}
	return strings.Join(parts, ", ")
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
