package main

import (
	"bytes"
	"claude-squad/log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	log.Initialize(false)
	defer log.Close()
	os.Exit(m.Run())
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

// setupPruneHome creates a fake HOME with one clean and one dirty leftover worktree,
// both older than the minimum age.
func setupPruneHome(t *testing.T) (clean, dirty string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	pruneYesFlag, pruneForceFlag, pruneDryRunFlag = false, false, false

	repo := filepath.Join(t.TempDir(), "repo")
	require.NoError(t, os.MkdirAll(repo, 0755))
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "config", "user.name", "Test")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "commit", "-q", "--allow-empty", "-m", "init")

	root := filepath.Join(home, ".claude-squad", "worktrees", "me")
	require.NoError(t, os.MkdirAll(root, 0755))
	clean = filepath.Join(root, "clean_18e0c5f0e0b3528f")
	dirty = filepath.Join(root, "dirty_18e0c5f0e0b3528f")
	runGit(t, repo, "worktree", "add", "-q", "-b", "me/clean", clean)
	runGit(t, repo, "worktree", "add", "-q", "-b", "me/dirty", dirty)
	require.NoError(t, os.WriteFile(filepath.Join(dirty, "wip.txt"), []byte("wip"), 0644))

	old := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(clean, old, old))
	require.NoError(t, os.Chtimes(dirty, old, old))
	return clean, dirty
}

func TestPruneDeclineKeepsEverything(t *testing.T) {
	clean, dirty := setupPruneHome(t)
	var out bytes.Buffer
	require.NoError(t, runPrune(strings.NewReader("n\n"), &out))
	require.Contains(t, out.String(), "remove  me/clean_18e0c5f0e0b3528f")
	require.Contains(t, out.String(), "skip    me/dirty_18e0c5f0e0b3528f")
	require.Contains(t, out.String(), "Aborted.")
	require.DirExists(t, clean)
	require.DirExists(t, dirty)
}

func TestPruneDryRun(t *testing.T) {
	clean, _ := setupPruneHome(t)
	pruneDryRunFlag = true
	var out bytes.Buffer
	require.NoError(t, runPrune(strings.NewReader(""), &out))
	require.Contains(t, out.String(), "would remove 1 worktree(s)")
	require.DirExists(t, clean)
}

func TestPruneConfirmRemovesCleanOnly(t *testing.T) {
	clean, dirty := setupPruneHome(t)
	var out bytes.Buffer
	require.NoError(t, runPrune(strings.NewReader("y\n"), &out))
	require.Contains(t, out.String(), "Removed 1 worktree(s).")
	require.NoDirExists(t, clean)
	require.DirExists(t, dirty)
}

func TestPruneForceRemovesDirty(t *testing.T) {
	clean, dirty := setupPruneHome(t)
	pruneYesFlag, pruneForceFlag = true, true
	var out bytes.Buffer
	require.NoError(t, runPrune(strings.NewReader(""), &out))
	require.Contains(t, out.String(), "Removed 2 worktree(s).")
	require.NoDirExists(t, clean)
	require.NoDirExists(t, dirty)
}

func TestPruneNothingToDo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var out bytes.Buffer
	require.NoError(t, runPrune(strings.NewReader(""), &out))
	require.Contains(t, out.String(), "No leftover worktrees")
}
