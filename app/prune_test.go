package app

import (
	"claude-squad/session/git"
	"claude-squad/ui"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

// fakePrune replaces pruneOps for a test and records removals.
func fakePrune(t *testing.T, leftovers []git.LeftoverWorktree, removeErr error) *[]string {
	t.Helper()
	removed := &[]string{}
	prev := pruneOps
	pruneOps = pruneFuncs{
		find: func() ([]git.LeftoverWorktree, error) { return leftovers, nil },
		remove: func(w git.LeftoverWorktree) error {
			if removeErr != nil {
				return removeErr
			}
			*removed = append(*removed, w.Path)
			return nil
		},
	}
	t.Cleanup(func() { pruneOps = prev })
	return removed
}

func newPruneTestHome(t *testing.T) *home {
	t.Helper()
	s := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	h := &home{
		ctx:          context.Background(),
		spinner:      s,
		list:         ui.NewList(&s, false),
		menu:         ui.NewMenu(),
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewTerminalPane()),
		errBox:       ui.NewErrBox(),
		state:        stateDefault,
	}
	h.updateHandleWindowSizeEvent(tea.WindowSizeMsg{Width: 300, Height: 50})
	return h
}

// press sends a key the way Bubble Tea does: the first Update only highlights
// the menu and re-sends the key, the second handles it.
func press(h *home, key string) tea.Cmd {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	_, cmd := h.Update(msg)
	if h.keySent {
		_, cmd = h.Update(msg)
	}
	return cmd
}

// deliver runs cmd and feeds its message back into Update.
func deliver(t *testing.T, h *home, cmd tea.Cmd) tea.Cmd {
	t.Helper()
	require.NotNil(t, cmd)
	_, next := h.Update(cmd())
	return next
}

var sampleLeftovers = []git.LeftoverWorktree{
	{Path: "/wt/a_1234567890", Valid: true, Branch: "a"},
	{Path: "/wt/b_1234567890"}, // broken
	{Path: "/wt/me/c_1234567890", Valid: true, Branch: "me/c", Dirty: true, Changes: 1},
	{Path: "/wt/d_1234567890", Valid: true, Detached: true, Unreachable: true},
}

// confirmText returns the confirmation prompt without the overlay's word
// wrapping and borders.
func confirmText(h *home) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(h.confirmationOverlay.Render(), "│", "")), " ")
}

func TestPruneKeyRemovesSafeThenUnsavedLeftovers(t *testing.T) {
	removed := fakePrune(t, sampleLeftovers, nil)
	h := newPruneTestHome(t)

	deliver(t, h, press(h, "P"))
	require.Equal(t, stateConfirm, h.state)
	view := confirmText(h)
	require.Contains(t, view, "Remove 2 leftover worktree(s)?")
	require.Contains(t, view, "Branches are kept. 2 more have unsaved work; you'll be asked about them next.")
	require.Empty(t, *removed, "nothing is removed before confirming")

	cmd := press(h, "y")
	require.Equal(t, stateConfirm, h.state, "asks about the unsaved ones next")
	view = confirmText(h)
	require.Contains(t, view, "Remove 2 leftover worktree(s) with unsaved work?")
	require.Contains(t, view, "• me/c: 1 uncommitted file")
	require.Contains(t, view, "• d: commits on no branch")
	require.Contains(t, view, "This work will be lost.")
	deliver(t, h, cmd)
	require.Equal(t, []string{"/wt/a_1234567890", "/wt/b_1234567890"}, *removed)
	require.Contains(t, h.errBox.String(), "Removed 2 leftover worktree(s)")

	deliver(t, h, press(h, "y"))
	require.Equal(t, stateDefault, h.state)
	require.Equal(t, []string{"/wt/a_1234567890", "/wt/b_1234567890", "/wt/me/c_1234567890", "/wt/d_1234567890"}, *removed)
}

func TestPruneKeyKeepsUnsavedWhenDeclined(t *testing.T) {
	removed := fakePrune(t, sampleLeftovers, nil)
	h := newPruneTestHome(t)

	deliver(t, h, press(h, "P"))
	deliver(t, h, press(h, "y"))
	require.Nil(t, press(h, "n"))
	require.Equal(t, stateDefault, h.state)
	require.Equal(t, []string{"/wt/a_1234567890", "/wt/b_1234567890"}, *removed)
}

func TestPruneKeyCancel(t *testing.T) {
	removed := fakePrune(t, sampleLeftovers, nil)
	h := newPruneTestHome(t)

	deliver(t, h, press(h, "P"))
	require.Nil(t, press(h, "n"))
	require.Equal(t, stateDefault, h.state, "declining the first prompt skips the second")
	require.Empty(t, *removed)
}

func TestPruneKeyNothingToRemove(t *testing.T) {
	fakePrune(t, nil, nil)
	h := newPruneTestHome(t)

	deliver(t, h, press(h, "P"))
	require.Equal(t, stateDefault, h.state)
	require.Contains(t, h.errBox.String(), "No leftover worktrees.")
}

func TestPruneKeyOnlyUnsavedLeftovers(t *testing.T) {
	removed := fakePrune(t, sampleLeftovers[2:3], nil)
	h := newPruneTestHome(t)

	deliver(t, h, press(h, "P"))
	require.Equal(t, stateConfirm, h.state)
	require.Contains(t, confirmText(h), "Remove 1 leftover worktree(s) with unsaved work?")
	deliver(t, h, press(h, "y"))
	require.Equal(t, []string{"/wt/me/c_1234567890"}, *removed)
}

func TestPruneKeyReportsFailures(t *testing.T) {
	fakePrune(t, sampleLeftovers[:1], errors.New("permission denied"))
	h := newPruneTestHome(t)

	deliver(t, h, press(h, "P"))
	deliver(t, h, press(h, "y"))
	require.Contains(t, h.errBox.String(), "failed to remove 1 worktree(s): permission denied")
}
