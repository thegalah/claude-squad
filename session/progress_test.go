package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStartStagesRecordDurations(t *testing.T) {
	inst := &Instance{}
	require.Equal(t, StageNone, inst.StartProgress().Stage)

	inst.setStartStage(StageWorktree)
	time.Sleep(5 * time.Millisecond)
	inst.setHasSetupScript(true)
	inst.setStartStage(StageSetupScript)
	inst.setSetupLogPath("/tmp/x.log")

	p := inst.StartProgress()
	require.Equal(t, StageSetupScript, p.Stage)
	require.False(t, p.StartedAt.IsZero())
	require.GreaterOrEqual(t, p.StageDurations[StageWorktree], 5*time.Millisecond)
	require.Equal(t, "/tmp/x.log", p.SetupLogPath)
	require.Equal(t, []StartStage{StageWorktree, StageSetupScript, StageProgram}, p.Stages())

	// The snapshot is a copy: later stages don't change it.
	inst.setStartStage(StageProgram)
	require.NotContains(t, p.StageDurations, StageSetupScript)
}

func TestStagesWithoutSetupScript(t *testing.T) {
	require.Equal(t, []StartStage{StageWorktree, StageProgram}, StartProgress{}.Stages())
}

func TestStageLabels(t *testing.T) {
	require.Equal(t, "Starting claude", StageLabel(StageProgram, "/usr/local/bin/claude --model x"))
	require.Equal(t, "Running .claude-squad/setup.sh", StageLabel(StageSetupScript, "claude"))
	require.Equal(t, "running setup.sh", StageShortLabel(StageSetupScript, "claude"))
	require.Equal(t, "starting aider", StageShortLabel(StageProgram, "aider"))
}

func TestFormatElapsed(t *testing.T) {
	require.Equal(t, "0s", FormatElapsed(-time.Second))
	require.Equal(t, "4s", FormatElapsed(4900*time.Millisecond))
	require.Equal(t, "1m05s", FormatElapsed(65*time.Second))
}

func TestTailLogCleansOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "setup.log")
	content := "first\n\x1b[32mgreen\x1b[0m\n\nprogress 10%\rprogress 50%\rprogress 100%\r\n\tindented\nlast"
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	require.Equal(t, []string{"green", "progress 100%", "    indented", "last"}, TailLog(path, 4))
	require.Equal(t, []string{"last"}, TailLog(path, 1))
	require.Nil(t, TailLog(filepath.Join(t.TempDir(), "missing.log"), 3))
	require.Nil(t, TailLog("", 3))
}

func TestTailLogReadsOnlyTheEndOfLargeFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "setup.log")
	big := strings.Repeat(strings.Repeat("x", 99)+"\n", 1000) + "the end\n"
	require.NoError(t, os.WriteFile(path, []byte(big), 0644))
	lines := TailLog(path, 2)
	require.Equal(t, "the end", lines[len(lines)-1])
	require.Len(t, lines, 2)
}

func TestWorktreeSetupReportsLogPathBeforeScriptFinishes(t *testing.T) {
	repo, worktree := newSetupFixture(t)
	writeSetupScript(t, repo, "#!/bin/sh\necho installing\n", 0755)

	var reported string
	ran, logPath, err := runWorktreeSetup(repo, worktree, "b", "s", func(p string) {
		reported = p
		_, statErr := os.Stat(p)
		require.NoError(t, statErr, "log file should exist when reported")
	})
	require.NoError(t, err)
	require.True(t, ran)
	require.Equal(t, logPath, reported)
	require.Contains(t, TailLog(logPath, 5), "installing")
}
