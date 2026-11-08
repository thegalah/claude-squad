package app

import (
	"claude-squad/log"
	"claude-squad/session"
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// cs restarts itself when its executable is replaced on disk (for example by
// `go build -o ~/.local/bin/cs .`), so open windows pick up a new build without
// the user quitting and relaunching each one. Instances live in tmux, so they
// keep running across the restart.
//
// The restart only happens once the new binary has stopped changing and runs
// (`cs version` succeeds), and waits until it wouldn't interrupt anything: no
// overlay or prompt open and no instance still starting up. While a session is
// attached the event loop is blocked, so no restart can happen then either.

// exeCheckInterval is how often the executable is checked for changes.
var exeCheckInterval = 2 * time.Second

const (
	// exeVerifyTimeout bounds how long `cs version` may take on the new binary.
	exeVerifyTimeout = 10 * time.Second
	// restartedEnv is set on the restarted process so it can show a notice.
	restartedEnv = "CS_RESTARTED_FOR_UPDATE"
)

// exeStamp identifies one version of the executable file.
type exeStamp struct {
	ino     uint64
	size    int64
	modTime time.Time
}

func statExe(path string) (exeStamp, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return exeStamp{}, err
	}
	stamp := exeStamp{size: fi.Size(), modTime: fi.ModTime()}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		stamp.ino = uint64(st.Ino)
	}
	return stamp, nil
}

// exeWatch tracks the executable this process was started from.
type exeWatch struct {
	path string
	// started is the executable as it was when this process started.
	started exeStamp
	// lastSeen is the result of the previous check, used to wait until a
	// replaced binary has finished being written.
	lastSeen exeStamp
	// verified is a replacement binary that passed `cs version`.
	verified *exeStamp
	// failed is a replacement binary that failed `cs version`; it isn't retried.
	failed *exeStamp
	// restart is set when the program quit so that Run re-execs it.
	restart bool
}

// newExeWatch returns a watch on the running executable, or nil if it can't be
// found.
func newExeWatch() *exeWatch {
	path, err := os.Executable()
	if err != nil {
		log.WarningLog.Printf("auto-restart disabled: can't find executable: %v", err)
		return nil
	}
	stamp, err := statExe(path)
	if err != nil {
		log.WarningLog.Printf("auto-restart disabled: can't stat %s: %v", path, err)
		return nil
	}
	return &exeWatch{path: path, started: stamp, lastSeen: stamp}
}

type exeCheckMsg struct {
	stamp exeStamp
	err   error
}

type exeVerifiedMsg struct {
	stamp exeStamp
	err   error
}

func (w *exeWatch) checkCmd() tea.Cmd {
	return tea.Tick(exeCheckInterval, func(time.Time) tea.Msg {
		stamp, err := statExe(w.path)
		return exeCheckMsg{stamp: stamp, err: err}
	})
}

func verifyExeCmd(path string, stamp exeStamp) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), exeVerifyTimeout)
		defer cancel()
		out, err := exec.CommandContext(ctx, path, "version").CombinedOutput()
		if err != nil {
			err = fmt.Errorf("%w: %s", err, out)
		}
		return exeVerifiedMsg{stamp: stamp, err: err}
	}
}

// handleExeCheck processes a periodic check of the executable.
func (m *home) handleExeCheck(msg exeCheckMsg) tea.Cmd {
	w := m.exeWatch
	next := w.checkCmd()
	if msg.err != nil {
		// Mid-replace the file can briefly be missing; check again later.
		return next
	}
	prev := w.lastSeen
	w.lastSeen = msg.stamp

	switch {
	case msg.stamp == w.started:
		// Unchanged, or put back to the original.
		w.verified = nil
		return next
	case w.verified != nil && *w.verified == msg.stamp:
		if m.canRestartNow() {
			return m.restartForUpdate()
		}
		return next
	case w.failed != nil && *w.failed == msg.stamp:
		return next
	case msg.stamp != prev:
		// Still being written (or replaced again); wait for it to settle.
		w.verified = nil
		return next
	default:
		return tea.Batch(next, verifyExeCmd(w.path, msg.stamp))
	}
}

func (m *home) handleExeVerified(msg exeVerifiedMsg) tea.Cmd {
	w := m.exeWatch
	if msg.err != nil {
		log.WarningLog.Printf("not restarting: new %s doesn't run: %v", w.path, msg.err)
		w.failed = &msg.stamp
		return nil
	}
	if msg.stamp != w.lastSeen {
		// Replaced again while verifying; the next checks will handle it.
		return nil
	}
	w.verified = &msg.stamp
	if m.canRestartNow() {
		return m.restartForUpdate()
	}
	return nil
}

// canRestartNow reports whether restarting would interrupt nothing the user is doing.
func (m *home) canRestartNow() bool {
	if m.state != stateDefault {
		return false
	}
	for _, inst := range m.list.GetInstances() {
		if inst.Status == session.Loading {
			return false
		}
	}
	return true
}

// restartForUpdate saves state and quits; Run then re-execs the new binary.
func (m *home) restartForUpdate() tea.Cmd {
	if err := m.storage.SaveInstances(m.list.GetInstances()); err != nil {
		// Don't restart without saving; try again on the next check.
		log.ErrorLog.Printf("not restarting: failed to save instances: %v", err)
		return m.exeWatch.checkCmd()
	}
	log.InfoLog.Printf("executable %s changed; restarting", m.exeWatch.path)
	m.exeWatch.restart = true
	return tea.Quit
}

// execRestart replaces this process with a fresh run of the executable, in the
// same terminal and working directory. It only returns on failure.
func execRestart(path string) error {
	env := append(os.Environ(), restartedEnv+"=1")
	return syscall.Exec(path, os.Args, env)
}

// restartedNotice returns the notice to show after an automatic restart, or "".
// It clears the marker so a later manual relaunch from this process's
// environment doesn't show it.
func restartedNotice() string {
	if os.Getenv(restartedEnv) == "" {
		return ""
	}
	_ = os.Unsetenv(restartedEnv)
	return "Restarted to load the updated cs binary."
}
