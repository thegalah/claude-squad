package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrBoxInfo(t *testing.T) {
	e := NewErrBox()
	e.SetSize(80, 1)

	e.SetInfo("3 leftover worktree(s) found")
	require.Contains(t, e.String(), "3 leftover worktree(s) found")

	// An error takes precedence over the notice.
	e.SetError(errors.New("boom"))
	require.Contains(t, e.String(), "boom")
	require.NotContains(t, e.String(), "leftover")

	e.Clear()
	require.Empty(t, strings.TrimSpace(e.String()))
}
