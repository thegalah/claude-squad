package ui

import (
	"claude-squad/session"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// menuText renders m on one line without padding.
func menuText(m *Menu) string {
	m.SetSize(300, 1)
	return strings.TrimSpace(m.String())
}

func TestMenuGroups(t *testing.T) {
	inst, err := session.NewInstance(session.InstanceOptions{Title: "t", Path: t.TempDir(), Program: "claude"})
	require.NoError(t, err)

	m := NewMenu()
	require.Equal(t, "n new • N new with prompt │ shift+p prune • ? help • q quit", menuText(m))

	m.SetInstance(inst)
	require.Equal(t,
		"n new • D kill │ ↵/o open • p push branch • c checkout │ tab switch tab • shift+p prune • ? help • q quit",
		menuText(m))

	m.SetActiveTab(DiffTab)
	require.Equal(t,
		"n new • D kill │ ↵/o open • p push branch • c checkout │ shift+↑ scroll • tab switch tab • shift+p prune • ? help • q quit",
		menuText(m))

	inst.SetStatus(session.Paused)
	m.SetActiveTab(PreviewTab)
	require.Contains(t, menuText(m), "│ ↵/o open • p push branch • r resume │")

	inst.SetStatus(session.Loading)
	m.SetInstance(inst)
	require.Equal(t, "n new │ shift+p prune • ? help • q quit", menuText(m))

	m.SetState(StatePrompt)
	require.Equal(t, "enter submit name", menuText(m))
}
