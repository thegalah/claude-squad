package session

import (
	"claude-squad/session/git"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsClaudeProgram(t *testing.T) {
	require.True(t, isClaudeProgram("claude"))
	require.True(t, isClaudeProgram("/usr/local/bin/claude"))
	require.True(t, isClaudeProgram("claude --model opus"))
	require.False(t, isClaudeProgram("aider --model ollama_chat/gemma3:1b"))
	require.False(t, isClaudeProgram("gemini"))
	require.False(t, isClaudeProgram(""))
}

func TestHasContinueFlag(t *testing.T) {
	require.False(t, hasContinueFlag("claude"))
	require.False(t, hasContinueFlag("claude --model opus"))
	require.True(t, hasContinueFlag("claude --continue"))
	require.True(t, hasContinueFlag("claude -c"))
	require.True(t, hasContinueFlag("claude --resume abc"))
	require.True(t, hasContinueFlag("claude --resume=abc"))
}

func TestClaudeProjectDirName(t *testing.T) {
	require.Equal(t,
		"-home-me--claude-squad-worktrees-me-feature-18e35e5392d359f6",
		claudeProjectDirName("/home/me/.claude-squad/worktrees/me-feature_18e35e5392d359f6"))
}

func newResumeTestInstance(t *testing.T, program string) (*Instance, string) {
	t.Helper()
	worktree := filepath.Join(t.TempDir(), "worktrees", "me-feature_1")
	require.NoError(t, os.MkdirAll(worktree, 0755))
	return &Instance{
		Program:     program,
		gitWorktree: git.NewGitWorktreeFromStorage("/repo", worktree, "feature", "me/feature", "", false),
	}, worktree
}

func writeConversation(t *testing.T, configDir, worktree string) {
	t.Helper()
	projectDir := filepath.Join(configDir, "projects", claudeProjectDirName(worktree))
	require.NoError(t, os.MkdirAll(projectDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "session.jsonl"), []byte("{}\n"), 0644))
}

func TestResumeCommandContinuesSavedClaudeConversation(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)

	instance, worktree := newResumeTestInstance(t, "claude --model opus")
	require.Equal(t, "claude --model opus", instance.resumeCommand(), "no conversation saved yet")

	writeConversation(t, configDir, worktree)
	require.Equal(t, "claude --model opus --continue", instance.resumeCommand())
}

func TestResumeCommandLeavesOtherProgramsAlone(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)

	aider, worktree := newResumeTestInstance(t, "aider")
	writeConversation(t, configDir, worktree)
	require.Equal(t, "aider", aider.resumeCommand())

	explicit, worktree := newResumeTestInstance(t, "claude --resume abc")
	writeConversation(t, configDir, worktree)
	require.Equal(t, "claude --resume abc", explicit.resumeCommand())
}
