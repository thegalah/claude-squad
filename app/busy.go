package app

import (
	"claude-squad/log"
	"claude-squad/session"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Resume, pause (checkout), kill and push run git and tmux commands that can take seconds,
// so they run in the background. While one runs, the instance is marked busy: its row
// shows the operation, its panes and status aren't refreshed if the operation stops or
// starts its session, other actions on it are refused, and cs won't quit or restart.

// busyDoneMsg reports that a background resume, pause or push finished.
type busyDoneMsg struct {
	instance *session.Instance
	op       session.BusyOp
	err      error
}

// killDoneMsg reports that a background kill finished. checkErr is set if the kill was
// refused before anything was touched, so the instance stays.
type killDoneMsg struct {
	instance *session.Instance
	checkErr error
	killErr  error
}

// startBusy marks inst busy with op and runs work in the background.
func (m *home) startBusy(inst *session.Instance, op session.BusyOp, work func() error) tea.Cmd {
	inst.SetBusy(op)
	return tea.Batch(m.instanceChanged(), func() tea.Msg {
		return busyDoneMsg{instance: inst, op: op, err: work()}
	})
}

// startResume resumes a paused instance in the background.
func (m *home) startResume(inst *session.Instance) tea.Cmd {
	if !inst.Paused() {
		return m.handleError(fmt.Errorf("can only resume paused instances"))
	}
	return m.startBusy(inst, session.BusyResuming, inst.Resume)
}

// startPause pauses an instance in the background, so its branch can be checked out.
func (m *home) startPause(inst *session.Instance) tea.Cmd {
	return m.startBusy(inst, session.BusyPausing, inst.Pause)
}

// startPush commits and pushes an instance's changes in the background.
func (m *home) startPush(inst *session.Instance) tea.Cmd {
	return m.startBusy(inst, session.BusyPushing, func() error {
		commitMsg := fmt.Sprintf("[claudesquad] update from '%s' on %s", inst.Title, time.Now().Format(time.RFC822))
		worktree, err := inst.GetGitWorktree()
		if err != nil {
			return err
		}
		return inst.WithWorktreeLock(func() error { return worktree.PushChanges(commitMsg, true) })
	})
}

// startKill kills an instance in the background, unless its branch is checked out.
func (m *home) startKill(inst *session.Instance) tea.Cmd {
	inst.SetBusy(session.BusyKilling)
	return tea.Batch(m.instanceChanged(), func() tea.Msg {
		worktree, err := inst.GetGitWorktree()
		if err != nil {
			return killDoneMsg{instance: inst, checkErr: err}
		}
		checkedOut, err := worktree.IsBranchCheckedOut()
		if err != nil {
			return killDoneMsg{instance: inst, checkErr: err}
		}
		if checkedOut {
			return killDoneMsg{instance: inst, checkErr: fmt.Errorf("instance %s is currently checked out", inst.Title)}
		}
		return killDoneMsg{instance: inst, killErr: inst.Kill()}
	})
}

func (m *home) handleBusyDone(msg busyDoneMsg) tea.Cmd {
	inst := msg.instance
	inst.SetBusy(session.NotBusy)
	var cmds []tea.Cmd
	switch msg.op {
	case session.BusyPausing:
		m.tabbedWindow.CleanupTerminalForInstance(inst.Title)
	case session.BusyResuming:
		cmds = append(cmds, tea.WindowSize()) // size the restarted session's pane
	case session.BusyPushing:
		if msg.err == nil {
			cmds = append(cmds, m.showInfo(fmt.Sprintf("Pushed '%s'.", inst.Title), 3*time.Second))
		}
	}
	if msg.err != nil {
		cmds = append(cmds, m.handleError(msg.err))
	}
	return tea.Batch(append(cmds, m.instanceChanged())...)
}

func (m *home) handleKillDone(msg killDoneMsg) tea.Cmd {
	inst := msg.instance
	inst.SetBusy(session.NotBusy)
	if msg.checkErr != nil {
		return tea.Batch(m.handleError(msg.checkErr), m.instanceChanged())
	}
	if msg.killErr != nil {
		log.ErrorLog.Printf("could not kill instance: %v", msg.killErr)
	}
	m.tabbedWindow.CleanupTerminalForInstance(inst.Title)
	m.list.Remove(inst)
	delete(m.lastOutputChange, inst)
	var errCmd tea.Cmd
	if err := m.storage.DeleteInstance(inst.Title); err != nil {
		errCmd = m.handleError(err)
	}
	return tea.Batch(errCmd, m.instanceChanged())
}

// busyOrLoading reports whether inst is starting or has a background operation running,
// so other actions on it must wait.
func busyOrLoading(inst *session.Instance) bool {
	return inst.Status == session.Loading || inst.Busy() != session.NotBusy
}

// busyInstance returns an instance with a background operation running, or nil.
func (m *home) busyInstance() *session.Instance {
	for _, inst := range m.list.GetInstances() {
		if inst.Busy() != session.NotBusy {
			return inst
		}
	}
	return nil
}

// showInfo shows an info message in the error box for d.
func (m *home) showInfo(text string, d time.Duration) tea.Cmd {
	m.errBox.SetInfo(text)
	return func() tea.Msg {
		select {
		case <-m.ctx.Done():
		case <-time.After(d):
		}
		return hideErrMsg{}
	}
}
