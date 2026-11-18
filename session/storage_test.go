package session

import (
	"claude-squad/config"
	"claude-squad/session/git"
	"encoding/json"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// pausedInstanceData builds stored data for a paused instance, which FromInstanceData can
// restore without touching tmux or git.
func pausedInstanceData(title, repo string) InstanceData {
	return InstanceData{
		Title:   title,
		Path:    repo,
		Status:  Paused,
		Program: "claude",
		Worktree: GitWorktreeData{
			RepoPath:     repo,
			WorktreePath: repo + "-worktree-" + title,
			SessionName:  title,
			BranchName:   "branch-" + title,
		},
	}
}

func seedState(t *testing.T, data ...InstanceData) {
	t.Helper()
	raw, err := json.Marshal(data)
	require.NoError(t, err)
	require.NoError(t, config.LoadState().SaveInstances(raw))
}

func storedTitles(t *testing.T) []string {
	t.Helper()
	var data []InstanceData
	require.NoError(t, json.Unmarshal(config.LoadState().GetInstances(), &data))
	titles := make([]string, 0, len(data))
	for _, d := range data {
		titles = append(titles, d.Title)
	}
	sort.Strings(titles)
	return titles
}

func titles(instances []*Instance) []string {
	out := make([]string, 0, len(instances))
	for _, i := range instances {
		out = append(out, i.Title)
	}
	sort.Strings(out)
	return out
}

func TestRepoStorageLoadsOnlyOwnRepo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	seedState(t,
		pausedInstanceData("a1", "/repos/a"),
		pausedInstanceData("b1", "/repos/b"),
		pausedInstanceData("a2", "/repos/a/"),
	)

	storageA, err := NewRepoStorage(config.LoadState(), "/repos/a")
	require.NoError(t, err)
	instances, err := storageA.LoadInstances()
	require.NoError(t, err)
	require.Equal(t, []string{"a1", "a2"}, titles(instances))

	all, err := NewStorage(config.LoadState())
	require.NoError(t, err)
	instances, err = all.LoadInstances()
	require.NoError(t, err)
	require.Equal(t, []string{"a1", "a2", "b1"}, titles(instances))
}

func TestRepoStorageSavePreservesOtherRepos(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	seedState(t,
		pausedInstanceData("a1", "/repos/a"),
		pausedInstanceData("a2", "/repos/a"),
		pausedInstanceData("b1", "/repos/b"),
	)

	storageA, err := NewRepoStorage(config.LoadState(), "/repos/a")
	require.NoError(t, err)
	instances, err := storageA.LoadInstances()
	require.NoError(t, err)

	// Drop a2 and save: b1 must survive, a2 must be gone.
	var kept []*Instance
	for _, i := range instances {
		if i.Title != "a2" {
			kept = append(kept, i)
		}
	}
	require.NoError(t, storageA.SaveInstances(kept))
	require.Equal(t, []string{"a1", "b1"}, storedTitles(t))
}

// Two cs processes in different repos each hold their own in-memory state. Saving from
// one must not overwrite instances the other wrote after it started.
func TestRepoStorageConcurrentProcesses(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	stateA := config.LoadState()
	stateB := config.LoadState()
	storageA, err := NewRepoStorage(stateA, "/repos/a")
	require.NoError(t, err)
	storageB, err := NewRepoStorage(stateB, "/repos/b")
	require.NoError(t, err)

	a1, err := FromInstanceData(pausedInstanceData("a1", "/repos/a"))
	require.NoError(t, err)
	b1, err := FromInstanceData(pausedInstanceData("b1", "/repos/b"))
	require.NoError(t, err)

	require.NoError(t, storageA.SaveInstances([]*Instance{a1}))
	require.NoError(t, storageB.SaveInstances([]*Instance{b1}))
	require.Equal(t, []string{"a1", "b1"}, storedTitles(t))

	// A stale in-memory state writing an unrelated field must not drop instances either.
	require.NoError(t, stateA.SetHelpScreensSeen(1))
	require.Equal(t, []string{"a1", "b1"}, storedTitles(t))

	instancesB, err := storageB.LoadInstances()
	require.NoError(t, err)
	require.Equal(t, []string{"b1"}, titles(instancesB))
}

func TestTrackedWorktreePaths(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	seedState(t,
		pausedInstanceData("a1", "/repos/a"),
		pausedInstanceData("b1", "/repos/b"),
	)
	paths, err := TrackedWorktreePaths(config.LoadState())
	require.NoError(t, err)
	sort.Strings(paths)
	require.Equal(t, []string{"/repos/a-worktree-a1", "/repos/b-worktree-b1"}, paths)
}

// The Diff tab lists changed files, so they're saved for paused sessions. State files
// from before that hold the full diff under "content", which is ignored.
func TestDiffStatsDataSavesFilesAndIgnoresOldContent(t *testing.T) {
	data := DiffStatsData{Added: 3, Removed: 1, Files: []git.FileChange{
		{Path: "a.go", Added: 3, Removed: 1},
		{Path: "logo.png", Binary: true},
	}}
	raw, err := json.Marshal(data)
	require.NoError(t, err)
	var back DiffStatsData
	require.NoError(t, json.Unmarshal(raw, &back))
	require.Equal(t, data, back)

	var old DiffStatsData
	require.NoError(t, json.Unmarshal([]byte(`{"added":2,"removed":0,"content":"diff --git a/x b/x\n+x\n"}`), &old))
	require.Equal(t, DiffStatsData{Added: 2}, old)
}

// The list shows each session's pull request, so it's saved for paused sessions.
func TestPausedInstanceKeepsPullRequest(t *testing.T) {
	pr := &git.PullRequest{Number: 31, State: git.PRMerged, URL: "https://github.com/me/app/pull/31"}
	inst, err := FromInstanceData(InstanceData{
		Title:       "paused",
		Status:      Paused,
		Program:     "claude",
		Worktree:    GitWorktreeData{RepoPath: "/repos/a", WorktreePath: "/wt/a", BranchName: "me/feat"},
		PullRequest: pr,
	})
	require.NoError(t, err)
	require.Equal(t, pr, inst.PullRequest())
	ref, ok := inst.PullRequestRef()
	require.True(t, ok)
	require.Equal(t, git.BranchRef{RepoPath: "/repos/a", Branch: "me/feat"}, ref)

	raw, err := json.Marshal(inst.ToInstanceData())
	require.NoError(t, err)
	var back InstanceData
	require.NoError(t, json.Unmarshal(raw, &back))
	require.Equal(t, pr, back.PullRequest)

	inst.SetPullRequest(nil)
	raw, err = json.Marshal(inst.ToInstanceData())
	require.NoError(t, err)
	require.NotContains(t, string(raw), "pull_request")
}
