package app

import (
	"claude-squad/session"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/require"
)

// newListClickTestHome returns a home whose list holds sessions named titles.
func newListClickTestHome(t *testing.T, titles ...string) *home {
	t.Helper()
	h := newTabTestHome(t)
	for _, title := range titles {
		inst, err := session.NewInstance(session.InstanceOptions{Title: title, Path: t.TempDir(), Program: "claude"})
		require.NoError(t, err)
		inst.SetStatus(session.Paused) // nothing to preview, so no tmux needed
		h.list.AddInstance(inst)
	}
	return h
}

func selectedTitle(h *home) string {
	return h.list.GetSelectedInstance().Title
}

func TestClickingSessionSelectsIt(t *testing.T) {
	h := newListClickTestHome(t, "alpha", "bravo", "charlie")
	require.Equal(t, "alpha", selectedTitle(h))

	// Click the title line, then the branch line below it.
	x, y := screenPosition(t, h, "charlie", "charlie")
	click(h, x, y)
	require.Equal(t, "charlie", selectedTitle(h))

	x, y = screenPosition(t, h, "bravo", "bravo")
	click(h, x, y+1)
	require.Equal(t, "bravo", selectedTitle(h))

	// The blank line between two sessions selects neither.
	_, alphaY := screenPosition(t, h, "alpha", "alpha")
	_, bravoY := screenPosition(t, h, "bravo", "bravo")
	click(h, 2, (alphaY+bravoY)/2+1)
	require.Equal(t, "bravo", selectedTitle(h))

	// Not while an overlay is open.
	x, y = screenPosition(t, h, "alpha", "alpha")
	h.state = stateHelp
	click(h, x, y)
	require.Equal(t, "bravo", selectedTitle(h))
}

func TestHoveringSessionHighlightsIt(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	h := newListClickTestHome(t, "alpha", "bravo")
	plain := h.View()
	ax, ay := screenPosition(t, h, "alpha", "alpha")
	bx, by := screenPosition(t, h, "bravo", "bravo")

	hover(h, bx, by)
	require.NotEqual(t, plain, h.View(), "hovering an unselected session highlights it")
	require.Equal(t, "alpha", selectedTitle(h), "hovering doesn't select")

	hover(h, ax, ay) // the selected session already stands out
	require.Equal(t, plain, h.View())

	hover(h, bx, by)
	hover(h, bx, by+10) // below the last session
	require.Equal(t, plain, h.View())
}

func TestTerminalTitleNamesSelectedSession(t *testing.T) {
	h := newListClickTestHome(t, "alpha", "bravo")
	h.repoName = "repo"
	h.Update(hideErrMsg{})
	require.Equal(t, "cs · repo · alpha", h.shownTitle)
	require.Nil(t, h.updateWindowTitle(), "unchanged title isn't re-sent")

	x, y := screenPosition(t, h, "bravo", "bravo")
	click(h, x, y)
	require.Equal(t, "cs · repo · bravo", h.shownTitle)

	h.list.Kill()
	h.list.Kill()
	h.Update(hideErrMsg{})
	require.Equal(t, "cs · repo", h.shownTitle)
}
