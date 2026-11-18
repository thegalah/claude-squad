package app

import (
	"claude-squad/session"
	"claude-squad/session/git"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// stubPRLookup makes pull request lookups return prs (by branch ref) and fail with err.
func stubPRLookup(t *testing.T, h *home, prs map[git.BranchRef]*git.PullRequest, err error) {
	t.Helper()
	h.prEnabled = true
	orig := findPullRequests
	findPullRequests = func([]git.BranchRef) (map[git.BranchRef]*git.PullRequest, error) { return prs, err }
	t.Cleanup(func() { findPullRequests = orig })
}

// lookUpPRs runs a pull request lookup to completion.
func lookUpPRs(t *testing.T, h *home) {
	t.Helper()
	cmd := h.startPRCheck()
	require.NotNil(t, cmd)
	h.Update(findMsg[prCheckedMsg](t, runCmd(cmd)))
}

func prRef(t *testing.T, inst *session.Instance) git.BranchRef {
	t.Helper()
	ref, ok := inst.PullRequestRef()
	require.True(t, ok)
	return ref
}

func TestPRBadgeShowsLeftOfDiffStatsAndOpensOnClick(t *testing.T) {
	alpha, _ := startedInstance(t, "alpha", true)
	bravo, _ := startedInstance(t, "bravo", true)
	alpha.SetDiffStats(&git.DiffStats{Added: 12, Removed: 3})
	h := newResponsiveTestHome(t, alpha, bravo)
	h.list.SetSelectedInstance(1)
	pr := &git.PullRequest{Number: 31, State: git.PRMerged, URL: "https://github.com/me/app/pull/31"}
	stubPRLookup(t, h, map[git.BranchRef]*git.PullRequest{prRef(t, alpha): pr, prRef(t, bravo): nil}, nil)
	var opened []string
	origOpen := openBrowser
	openBrowser = func(url string) error { opened = append(opened, url); return nil }
	t.Cleanup(func() { openBrowser = origOpen })

	lookUpPRs(t, h)
	require.Equal(t, pr, alpha.PullRequest())
	require.Nil(t, bravo.PullRequest())
	require.Contains(t, listPart(h, lineWith(t, h, "#31")), "#31 +12,-3")
	require.NotContains(t, h.list.String(), "#0")

	x, y := screenPosition(t, h, "#31", "#31")
	hover(h, x, y)
	require.Equal(t, 0, h.list.PRBadgeAt(x-h.viewIndent, y-viewPaddingTop+h.viewOverflow))
	runCmd(h.handleMouseClick(x+2, y)) // anywhere on the "#31"
	require.Equal(t, []string{pr.URL}, opened)
	require.Equal(t, "alpha", selectedTitle(h), "clicking the badge selects its session")
	require.Equal(t, stateDefault, h.state, "and doesn't attach")

	// Off the badge, the branch line just selects.
	h.list.SetSelectedInstance(1)
	runCmd(h.handleMouseClick(x-3, y))
	require.Len(t, opened, 1)
	require.Equal(t, "alpha", selectedTitle(h))
}

// Without diff stats, the badge sits at the right end of the branch line, under the
// selected session's arrow, whose target reaches the branch line: the badge wins.
func TestPRBadgeWinsOverOpenArrowTarget(t *testing.T) {
	alpha, _ := startedInstance(t, "alpha", true)
	h := newResponsiveTestHome(t, alpha)
	h.appState = &helpNotSeen{}
	pr := &git.PullRequest{Number: 7, State: git.PROpen, URL: "https://github.com/me/app/pull/7"}
	stubPRLookup(t, h, map[git.BranchRef]*git.PullRequest{prRef(t, alpha): pr}, nil)
	var opened []string
	origOpen := openBrowser
	openBrowser = func(url string) error { opened = append(opened, url); return nil }
	t.Cleanup(func() { openBrowser = origOpen })
	lookUpPRs(t, h)

	x, y := screenPosition(t, h, "#7", "#7")
	runCmd(h.handleMouseClick(x+1, y))
	require.Equal(t, []string{pr.URL}, opened)
	require.False(t, attaching(h))
}

func TestFailedPRLookupKeepsWhatWasKnown(t *testing.T) {
	alpha, _ := startedInstance(t, "alpha", true)
	bravo, _ := startedInstance(t, "bravo", true)
	h := newResponsiveTestHome(t, alpha, bravo)
	known := &git.PullRequest{Number: 4, State: git.PROpen, URL: "u"}
	alpha.SetPullRequest(known)
	bravo.SetPullRequest(known)

	// alpha's lookup failed (no entry); bravo's found that it has none.
	stubPRLookup(t, h, map[git.BranchRef]*git.PullRequest{prRef(t, bravo): nil}, errors.New("gh: not logged in"))
	lookUpPRs(t, h)
	require.Equal(t, known, alpha.PullRequest())
	require.Nil(t, bravo.PullRequest())
	require.Equal(t, "gh: not logged in", h.prLastErr)
}

func TestPRLookupsDontOverlap(t *testing.T) {
	alpha, _ := startedInstance(t, "alpha", true)
	h := newResponsiveTestHome(t, alpha)
	stubPRLookup(t, h, map[git.BranchRef]*git.PullRequest{}, nil)

	first := h.startPRCheck()
	require.NotNil(t, first)
	staleTick := prTickMsg{gen: h.prGen - 1}
	require.Nil(t, h.startPRCheck(), "a push during a lookup asks for another afterwards")
	require.Nil(t, h.handlePRTick(staleTick), "ticks scheduled before the lookup started are dropped")

	again := h.handlePRChecked(findMsg[prCheckedMsg](t, runCmd(first)))
	require.True(t, h.prChecking, "the requested lookup runs straight away")
	h.handlePRChecked(findMsg[prCheckedMsg](t, runCmd(again)))
	require.False(t, h.prChecking)
	require.False(t, h.prRecheck)
}

func TestNoPRLookupWithoutGH(t *testing.T) {
	alpha, _ := startedInstance(t, "alpha", true)
	h := newResponsiveTestHome(t, alpha)
	require.False(t, h.prEnabled)
	require.Nil(t, h.startPRCheck())
}
