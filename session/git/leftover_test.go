package git

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type leftoverFixture struct {
	root string // the cs worktrees directory
	repo string
}

func newLeftoverFixture(t *testing.T) *leftoverFixture {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	mustRunGit(t, "", "init", "-q", repo)
	mustRunGit(t, repo, "config", "user.name", "Test User")
	mustRunGit(t, repo, "config", "user.email", "test@example.com")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0644))
	mustRunGit(t, repo, "add", "README.md")
	mustRunGit(t, repo, "commit", "-q", "-m", "initial")
	return &leftoverFixture{root: filepath.Join(t.TempDir(), "worktrees"), repo: repo}
}

// addWorktree creates a real git worktree for branch at <root>/<rel>.
func (f *leftoverFixture) addWorktree(t *testing.T, rel, branch string) string {
	t.Helper()
	path := filepath.Join(f.root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	mustRunGit(t, f.repo, "worktree", "add", "-q", "-b", branch, path)
	return path
}

func (f *leftoverFixture) mkdir(t *testing.T, rel string, files ...string) string {
	t.Helper()
	path := filepath.Join(f.root, rel)
	require.NoError(t, os.MkdirAll(path, 0755))
	for _, name := range files {
		require.NoError(t, os.WriteFile(filepath.Join(path, name), []byte("x"), 0644))
	}
	return path
}

func leftoverPaths(root string, ws []LeftoverWorktree) []string {
	out := make([]string, 0, len(ws))
	for _, w := range ws {
		rel, _ := filepath.Rel(root, w.Path)
		out = append(out, rel)
	}
	sort.Strings(out)
	return out
}

func byRel(t *testing.T, root string, ws []LeftoverWorktree, rel string) LeftoverWorktree {
	t.Helper()
	for _, w := range ws {
		if w.Path == filepath.Join(root, rel) {
			return w
		}
	}
	t.Fatalf("no leftover %s", rel)
	return LeftoverWorktree{}
}

func TestFindLeftoverWorktrees(t *testing.T) {
	f := newLeftoverFixture(t)
	tracked := f.addWorktree(t, "me/tracked_18e0c5f0e0b3528f", "me/tracked")
	f.addWorktree(t, "me/clean_18e0c5f0e0b3528f", "me/clean")
	dirty := f.addWorktree(t, "me/dirty_18e0c5f0e0b3528f", "me/dirty")
	require.NoError(t, os.WriteFile(filepath.Join(dirty, "new.txt"), []byte("wip"), 0644))
	detached := f.addWorktree(t, "detached_18e0c5f0e0b3528f", "detached")
	mustRunGit(t, detached, "checkout", "-q", "--detach")
	f.mkdir(t, "me/broken_18dec0c4da4ea3fc", "leftover.txt") // cs-named, no .git
	f.mkdir(t, "me/empty-prefix")                            // empty directory
	f.mkdir(t, "me/k206", "page_to_pdf.py")                  // not cs-named: left alone

	got, err := findLeftoverWorktrees(f.root, []string{tracked}, 0, true, time.Now())
	require.NoError(t, err)
	require.Equal(t, []string{
		"detached_18e0c5f0e0b3528f",
		"me/broken_18dec0c4da4ea3fc",
		"me/clean_18e0c5f0e0b3528f",
		"me/dirty_18e0c5f0e0b3528f",
		"me/empty-prefix",
	}, leftoverPaths(f.root, got))

	clean := byRel(t, f.root, got, "me/clean_18e0c5f0e0b3528f")
	require.True(t, clean.Valid)
	require.Equal(t, "me/clean", clean.Branch)
	require.False(t, clean.NeedsForce())

	require.True(t, byRel(t, f.root, got, "me/dirty_18e0c5f0e0b3528f").Dirty)
	require.True(t, byRel(t, f.root, got, "me/dirty_18e0c5f0e0b3528f").NeedsForce())
	require.True(t, byRel(t, f.root, got, "detached_18e0c5f0e0b3528f").Detached)
	require.True(t, byRel(t, f.root, got, "detached_18e0c5f0e0b3528f").NeedsForce())

	broken := byRel(t, f.root, got, "me/broken_18dec0c4da4ea3fc")
	require.False(t, broken.Valid)
	require.False(t, broken.NeedsForce())
}

func TestFindLeftoverWorktreesSkipsRecentDirectories(t *testing.T) {
	f := newLeftoverFixture(t)
	f.addWorktree(t, "me/new_18e0c5f0e0b3528f", "me/new")

	got, err := findLeftoverWorktrees(f.root, nil, time.Hour, true, time.Now())
	require.NoError(t, err)
	require.Empty(t, got, "a worktree created moments ago may belong to a session being created")

	got, err = findLeftoverWorktrees(f.root, nil, time.Hour, false, time.Now().Add(2*time.Hour))
	require.NoError(t, err)
	require.Equal(t, []string{"me/new_18e0c5f0e0b3528f"}, leftoverPaths(f.root, got))
}

func TestFindLeftoverWorktreesMissingRoot(t *testing.T) {
	got, err := findLeftoverWorktrees(filepath.Join(t.TempDir(), "nope"), nil, 0, true, time.Now())
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestRemoveLeftoverWorktree(t *testing.T) {
	f := newLeftoverFixture(t)
	valid := f.addWorktree(t, "me/clean_18e0c5f0e0b3528f", "me/clean")
	broken := f.mkdir(t, "other/broken_18dec0c4da4ea3fc", "leftover.txt")
	keep := f.mkdir(t, "me/k206", "page_to_pdf.py")

	got, err := findLeftoverWorktrees(f.root, nil, 0, true, time.Now())
	require.NoError(t, err)
	for _, w := range got {
		require.NoError(t, removeLeftoverWorktree(f.root, w))
	}

	require.NoDirExists(t, valid)
	require.NoDirExists(t, broken)
	require.NoDirExists(t, filepath.Join(f.root, "other"), "empty parent directory is removed")
	require.DirExists(t, keep)

	// The repository no longer lists the worktree, and the branch is kept.
	require.NotContains(t, mustRunGit(t, f.repo, "worktree", "list"), valid)
	require.True(t, strings.Contains(mustRunGit(t, f.repo, "branch", "--list", "me/clean"), "me/clean"))
}

func TestRemoveLeftoverWorktreeRefusesPathsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	err := removeLeftoverWorktree(root, LeftoverWorktree{Path: outside})
	require.Error(t, err)
	require.DirExists(t, outside)

	err = removeLeftoverWorktree(root, LeftoverWorktree{Path: root})
	require.Error(t, err)
	require.DirExists(t, root)
}
