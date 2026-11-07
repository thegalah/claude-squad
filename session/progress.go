package session

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// StartStage is a step in starting a new instance, shown in the UI while it is Loading.
type StartStage int

const (
	// StageNone means the instance hasn't started setting up yet.
	StageNone StartStage = iota
	// StageWorktree is creating the git worktree and branch.
	StageWorktree
	// StageSetupScript is running the repo's .claude-squad/setup.sh in the new worktree.
	StageSetupScript
	// StageProgram is starting the program (claude, ...) in a tmux session.
	StageProgram
)

// StartProgress is a snapshot of how far a new instance has got through Start.
type StartProgress struct {
	// Stage is the step currently running.
	Stage StartStage
	// StartedAt is when Start began.
	StartedAt time.Time
	// StageStartedAt is when the current stage began.
	StageStartedAt time.Time
	// StageDurations holds how long each finished stage took.
	StageDurations map[StartStage]time.Duration
	// HasSetupScript is true when the repo has a setup script, so StageSetupScript applies.
	HasSetupScript bool
	// SetupLogPath is the setup script's log file, once the script has started.
	SetupLogPath string
}

// Stages returns the stages this start goes through, in order.
func (p StartProgress) Stages() []StartStage {
	if p.HasSetupScript {
		return []StartStage{StageWorktree, StageSetupScript, StageProgram}
	}
	return []StartStage{StageWorktree, StageProgram}
}

// StageLabel describes a stage, e.g. "Running .claude-squad/setup.sh". program is the
// instance's program, used for StageProgram.
func StageLabel(stage StartStage, program string) string {
	switch stage {
	case StageWorktree:
		return "Creating git worktree"
	case StageSetupScript:
		return "Running " + SetupScriptPath
	case StageProgram:
		return "Starting " + programName(program)
	default:
		return "Preparing"
	}
}

// StageShortLabel is a compact stage description for the instance list.
func StageShortLabel(stage StartStage, program string) string {
	switch stage {
	case StageWorktree:
		return "creating worktree"
	case StageSetupScript:
		return "running " + filepath.Base(SetupScriptPath)
	case StageProgram:
		return "starting " + programName(program)
	default:
		return "preparing"
	}
}

// programName returns the executable name of a program command, e.g. "claude" for
// "/usr/bin/claude --model x".
func programName(program string) string {
	fields := strings.Fields(program)
	if len(fields) == 0 {
		return "program"
	}
	return filepath.Base(fields[0])
}

// FormatElapsed formats a duration for progress display: "4s", "1m05s".
func FormatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	secs := int(d.Seconds())
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	return fmt.Sprintf("%dm%02ds", secs/60, secs%60)
}

// StartProgress returns a snapshot of the instance's start progress. It is safe to call
// while Start runs in another goroutine.
func (i *Instance) StartProgress() StartProgress {
	i.progressMu.Lock()
	defer i.progressMu.Unlock()
	p := i.progress
	p.StageDurations = make(map[StartStage]time.Duration, len(i.progress.StageDurations))
	for k, v := range i.progress.StageDurations {
		p.StageDurations[k] = v
	}
	return p
}

// setStartStage records that Start moved on to stage.
func (i *Instance) setStartStage(stage StartStage) {
	i.progressMu.Lock()
	defer i.progressMu.Unlock()
	now := time.Now()
	if i.progress.StartedAt.IsZero() {
		i.progress.StartedAt = now
	}
	if i.progress.Stage != StageNone {
		if i.progress.StageDurations == nil {
			i.progress.StageDurations = make(map[StartStage]time.Duration)
		}
		i.progress.StageDurations[i.progress.Stage] = now.Sub(i.progress.StageStartedAt)
	}
	i.progress.Stage = stage
	i.progress.StageStartedAt = now
}

func (i *Instance) setHasSetupScript(has bool) {
	i.progressMu.Lock()
	defer i.progressMu.Unlock()
	i.progress.HasSetupScript = has
}

func (i *Instance) setSetupLogPath(path string) {
	i.progressMu.Lock()
	defer i.progressMu.Unlock()
	i.progress.SetupLogPath = path
}

// hasSetupScript reports whether the repository has a worktree setup script.
func hasSetupScript(repoPath string) bool {
	_, err := os.Stat(filepath.Join(repoPath, SetupScriptPath))
	return err == nil
}

// tailReadBytes bounds how much of a log file TailLog reads.
const tailReadBytes = 16 * 1024

var ansiEscape = regexp.MustCompile(`\x1b(\[[0-?]*[ -/]*[@-~]|\][^\x07\x1b]*(\x07|\x1b\\)|[@-Z\\-_])`)

// TailLog returns up to n of the last non-empty lines of the file at path, cleaned up for
// display: ANSI escapes are removed, carriage-return progress output keeps only its final
// state, and tabs become spaces. A missing or unreadable file gives no lines.
func TailLog(path string, n int) []string {
	if path == "" || n <= 0 {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil
	}
	offset := info.Size() - tailReadBytes
	if offset < 0 {
		offset = 0
	}
	buf := make([]byte, info.Size()-offset)
	if _, err := f.ReadAt(buf, offset); err != nil && err != io.EOF {
		return nil
	}
	if offset > 0 {
		// Drop the partial first line.
		if idx := bytes.IndexByte(buf, '\n'); idx >= 0 {
			buf = buf[idx+1:]
		}
	}

	var lines []string
	for _, line := range strings.Split(string(buf), "\n") {
		line = strings.TrimRight(line, "\r")
		if idx := strings.LastIndexByte(line, '\r'); idx >= 0 {
			line = line[idx+1:]
		}
		line = ansiEscape.ReplaceAllString(line, "")
		line = strings.ReplaceAll(line, "\t", "    ")
		line = strings.Map(func(r rune) rune {
			if r < 0x20 || r == 0x7f {
				return -1
			}
			return r
		}, line)
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}
