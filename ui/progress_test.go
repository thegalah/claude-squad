package ui

import (
	"claude-squad/session"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/mattn/go-runewidth"
	"github.com/stretchr/testify/require"
)

func newLoadingInstance(t *testing.T) *session.Instance {
	t.Helper()
	inst, err := session.NewInstance(session.InstanceOptions{Title: "feat", Path: ".", Program: "claude"})
	require.NoError(t, err)
	inst.SetStatus(session.Loading)
	return inst
}

func TestPreviewShowsSetupSteps(t *testing.T) {
	p := NewPreviewPane()
	s := spinner.New()
	p.SetSpinner(&s)
	p.SetSize(80, 30)

	require.NoError(t, p.UpdateContent(newLoadingInstance(t)))
	out := p.String()
	require.Contains(t, out, "Setting up workspace")
	require.Contains(t, out, "Creating git worktree")
	require.Contains(t, out, "Starting claude")
	// No setup script in this repo state, so that step isn't listed.
	require.NotContains(t, out, "setup.sh")
}

func TestLoadingLineFillsWidth(t *testing.T) {
	inst := newLoadingInstance(t)
	line := loadingLine(inst, 4, 40)
	require.Equal(t, 40, runewidth.StringWidth(line))
	require.Contains(t, line, "preparing…")

	short := loadingLine(inst, 4, 8)
	require.Equal(t, 8, runewidth.StringWidth(short))
}

func TestListShowsLoadingStatusInsteadOfBranch(t *testing.T) {
	l := newTestList()
	inst := newLoadingInstance(t)
	inst.Branch = "me/feat"
	l.AddInstance(inst)
	l.SetSize(60, 20)
	out := l.String()
	require.Contains(t, out, "preparing…")
	require.False(t, strings.Contains(out, "me/feat"))
}
