package ui

import (
	"claude-squad/session"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/mattn/go-runewidth"
	"github.com/stretchr/testify/require"
)

func newTestList(titles ...string) *List {
	s := spinner.New()
	l := NewList(&s, false)
	for _, t := range titles {
		inst, _ := session.NewInstance(session.InstanceOptions{
			Title:   t,
			Path:    ".",
			Program: "echo",
		})
		l.AddInstance(inst)
	}
	return l
}

func TestMoveUp(t *testing.T) {
	l := newTestList("a", "b", "c")
	l.SetSelectedInstance(1) // select "b"

	moved := l.MoveUp()
	require.True(t, moved)
	require.Equal(t, 0, l.selectedIdx)
	require.Equal(t, "b", l.items[0].Title)
	require.Equal(t, "a", l.items[1].Title)
	require.Equal(t, "c", l.items[2].Title)
}

func TestMoveUp_AtTop(t *testing.T) {
	l := newTestList("a", "b", "c")
	l.SetSelectedInstance(0)

	moved := l.MoveUp()
	require.False(t, moved)
	require.Equal(t, 0, l.selectedIdx)
	require.Equal(t, "a", l.items[0].Title)
}

func TestMoveDown(t *testing.T) {
	l := newTestList("a", "b", "c")
	l.SetSelectedInstance(1) // select "b"

	moved := l.MoveDown()
	require.True(t, moved)
	require.Equal(t, 2, l.selectedIdx)
	require.Equal(t, "a", l.items[0].Title)
	require.Equal(t, "c", l.items[1].Title)
	require.Equal(t, "b", l.items[2].Title)
}

func TestMoveDown_AtBottom(t *testing.T) {
	l := newTestList("a", "b", "c")
	l.SetSelectedInstance(2)

	moved := l.MoveDown()
	require.False(t, moved)
	require.Equal(t, 2, l.selectedIdx)
	require.Equal(t, "c", l.items[2].Title)
}

func TestMoveWithSingleItem(t *testing.T) {
	l := newTestList("only")
	l.SetSelectedInstance(0)

	require.False(t, l.MoveUp())
	require.False(t, l.MoveDown())
}

func TestTitleIncludesRepoName(t *testing.T) {
	s := spinner.New()
	l := NewList(&s, false)
	require.Equal(t, " Instances ", l.title(80))

	l.SetRepoName("claude-squad")
	require.Equal(t, " Instances · claude-squad ", l.title(80))
	require.Equal(t, " Instances · claude-squad ", l.title(0), "no width limit")

	// Too narrow for the full name: truncate it with an ellipsis.
	got := l.title(20)
	require.Equal(t, " Instances · claud… ", got)
	require.LessOrEqual(t, runewidth.StringWidth(got), 20)

	// Too narrow for any of the name: fall back to the plain title.
	require.Equal(t, " Instances ", l.title(14))
}

func TestRemoveKeepsSelectionOnSameInstance(t *testing.T) {
	titles := func(l *List) []string {
		var out []string
		for _, inst := range l.items {
			out = append(out, inst.Title)
		}
		return out
	}
	for _, tc := range []struct {
		name             string
		selected, remove int
		wantSelected     string
	}{
		{"remove before selection", 2, 0, "c"},
		{"remove after selection", 0, 2, "a"},
		{"remove selected middle", 1, 1, "c"},
		{"remove selected last", 2, 2, "b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := newTestList("a", "b", "c")
			l.SetSelectedInstance(tc.selected)
			removed := l.items[tc.remove].Title
			l.Remove(l.items[tc.remove])
			require.NotContains(t, titles(l), removed)
			require.Len(t, l.items, 2)
			require.Equal(t, tc.wantSelected, l.GetSelectedInstance().Title)
		})
	}

	l := newTestList("a")
	l.Remove(l.items[0])
	require.Nil(t, l.GetSelectedInstance())
}
