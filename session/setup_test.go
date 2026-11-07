package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newSetupFixture returns a repo dir and a worktree dir, with HOME pointed at a temp dir.
func newSetupFixture(t *testing.T) (repo, worktree string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	repo = t.TempDir()
	worktree = filepath.Join(t.TempDir(), "me-feature_18e0c5f0e0b3528f")
	require.NoError(t, os.MkdirAll(worktree, 0755))
	return repo, worktree
}

func writeSetupScript(t *testing.T, repo, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(repo, SetupScriptPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(body), mode))
}

func TestWorktreeSetupWithoutScript(t *testing.T) {
	repo, worktree := newSetupFixture(t)
	ran, logPath, err := runWorktreeSetup(repo, worktree, "me/feature", "feature", nil)
	require.NoError(t, err)
	require.False(t, ran)
	require.Empty(t, logPath)
}

func TestWorktreeSetupRunsInWorktreeWithEnv(t *testing.T) {
	repo, worktree := newSetupFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".env"), []byte("SECRET=1\n"), 0644))
	writeSetupScript(t, repo, `#!/bin/sh
set -e
cp "$CS_REPO_PATH/.env" .
printf '%s\n' "$CS_WORKTREE_PATH" "$CS_BRANCH" "$CS_SESSION" "$(pwd)" > env.txt
echo "setup done"
`, 0755)

	ran, logPath, err := runWorktreeSetup(repo, worktree, "me/feature", "feature", nil)
	require.NoError(t, err)
	require.True(t, ran)

	env, err := os.ReadFile(filepath.Join(worktree, "env.txt"))
	require.NoError(t, err)
	resolved, _ := filepath.EvalSymlinks(worktree)
	lines := strings.Split(strings.TrimSpace(string(env)), "\n")
	require.Equal(t, worktree, lines[0])
	require.Equal(t, "me/feature", lines[1])
	require.Equal(t, "feature", lines[2])
	require.Contains(t, []string{worktree, resolved}, lines[3], "script runs inside the worktree")
	require.FileExists(t, filepath.Join(worktree, ".env"))

	logged, err := os.ReadFile(logPath)
	require.NoError(t, err)
	require.Contains(t, string(logged), "setup done")
}

func TestWorktreeSetupNonExecutableRunsWithSh(t *testing.T) {
	repo, worktree := newSetupFixture(t)
	writeSetupScript(t, repo, "touch ran-with-sh\n", 0644)

	ran, _, err := runWorktreeSetup(repo, worktree, "b", "s", nil)
	require.NoError(t, err)
	require.True(t, ran)
	require.FileExists(t, filepath.Join(worktree, "ran-with-sh"))
}

func TestWorktreeSetupFailureReportsLog(t *testing.T) {
	repo, worktree := newSetupFixture(t)
	writeSetupScript(t, repo, "#!/bin/sh\necho 'pnpm: not found' >&2\nexit 3\n", 0755)

	ran, logPath, err := runWorktreeSetup(repo, worktree, "b", "s", nil)
	require.True(t, ran)
	require.Error(t, err)
	require.Contains(t, err.Error(), logPath)
	logged, readErr := os.ReadFile(logPath)
	require.NoError(t, readErr)
	require.Contains(t, string(logged), "pnpm: not found")
}

func TestWorktreeSetupTimeout(t *testing.T) {
	repo, worktree := newSetupFixture(t)
	writeSetupScript(t, repo, "#!/bin/sh\nexec sleep 30\n", 0755)

	start := time.Now()
	ran, _, err := runWorktreeSetupWithTimeout(repo, worktree, "b", "s", 200*time.Millisecond, nil)
	require.True(t, ran)
	require.ErrorContains(t, err, "timed out")
	require.Less(t, time.Since(start), 10*time.Second)
}
