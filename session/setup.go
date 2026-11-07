package session

import (
	"claude-squad/config"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// SetupScriptPath is where a repository's worktree setup script lives, relative to the
// repository root. It is read from the main checkout, so it can be git-ignored.
const SetupScriptPath = ".claude-squad/setup.sh"

// setupTimeout bounds how long a setup script may run before it is killed.
const setupTimeout = 15 * time.Minute

// runWorktreeSetup runs the repository's setup script, if it has one, inside a freshly
// created worktree. The script's output goes to a log file whose path is returned. ran is
// false when the repository has no setup script. onLog, if not nil, is called with the log
// path once the log file exists, so progress can be shown while the script runs.
func runWorktreeSetup(repoPath, worktreePath, branch, sessionTitle string, onLog func(logPath string)) (ran bool, logPath string, err error) {
	return runWorktreeSetupWithTimeout(repoPath, worktreePath, branch, sessionTitle, setupTimeout, onLog)
}

func runWorktreeSetupWithTimeout(repoPath, worktreePath, branch, sessionTitle string, timeout time.Duration, onLog func(logPath string)) (ran bool, logPath string, err error) {
	script := filepath.Join(repoPath, SetupScriptPath)
	info, err := os.Stat(script)
	if errors.Is(err, os.ErrNotExist) {
		return false, "", nil
	}
	if err != nil {
		return true, "", fmt.Errorf("failed to read setup script %s: %w", script, err)
	}
	if info.IsDir() {
		return true, "", fmt.Errorf("setup script %s is a directory", script)
	}

	configDir, err := config.GetConfigDir()
	if err != nil {
		return true, "", err
	}
	logDir := filepath.Join(configDir, "setup-logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return true, "", fmt.Errorf("failed to create setup log directory: %w", err)
	}
	logPath = filepath.Join(logDir, filepath.Base(worktreePath)+".log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return true, "", fmt.Errorf("failed to create setup log: %w", err)
	}
	defer logFile.Close()
	if onLog != nil {
		onLog(logPath)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Run executable scripts directly so their shebang is honoured; fall back to sh.
	var cmd *exec.Cmd
	if info.Mode()&0111 != 0 {
		cmd = exec.CommandContext(ctx, script)
	} else {
		cmd = exec.CommandContext(ctx, "sh", script)
	}
	cmd.Dir = worktreePath
	cmd.Env = append(os.Environ(),
		"CS_REPO_PATH="+repoPath,
		"CS_WORKTREE_PATH="+worktreePath,
		"CS_BRANCH="+branch,
		"CS_SESSION="+sessionTitle,
	)
	cmd.Stdin = nil // reads get EOF, so a script can't hang waiting for input
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	// Don't wait for background processes the script left holding the output open.
	cmd.WaitDelay = 5 * time.Second

	fmt.Fprintf(logFile, "$ %s (in %s)\n\n", script, worktreePath)
	runErr := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return true, logPath, fmt.Errorf("setup script timed out after %s, see %s", timeout, logPath)
	}
	if runErr != nil {
		return true, logPath, fmt.Errorf("setup script failed (%v), see %s", runErr, logPath)
	}
	return true, logPath, nil
}
