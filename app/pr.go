package app

import (
	"claude-squad/log"
	"claude-squad/session"
	"claude-squad/session/git"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Each session's row shows the GitHub pull request from its branch, if there is one, as a
// "#31" badge colored by its state; clicking it opens the pull request. The pull requests
// are looked up with gh in the background, at startup, every prInterval, and right after
// a push or a new session starts. There's one query per GitHub repository, however many
// sessions it has (see git.FindPullRequests). Without gh, nothing is looked up.

// prInterval is how often pull requests are looked up.
const prInterval = 30 * time.Second

// prTickMsg starts the next pull request lookup, unless it's from a schedule that a lookup
// started since has replaced (gen is behind).
type prTickMsg struct{ gen int }

// prCheckedMsg carries a lookup's results: the pull request (nil for none) of each
// instance whose lookup succeeded.
type prCheckedMsg struct {
	prs map[*session.Instance]*git.PullRequest
	err error
}

// findPullRequests looks up pull requests. Tests replace it.
var findPullRequests = git.FindPullRequests

// startPRCheck looks up every instance's pull request in the background. If a lookup is
// already running, another runs once it's done.
func (m *home) startPRCheck() tea.Cmd {
	if !m.prEnabled {
		return nil
	}
	if m.prChecking {
		m.prRecheck = true
		return nil
	}
	refs := make(map[*session.Instance]git.BranchRef)
	for _, inst := range m.list.GetInstances() {
		if ref, ok := inst.PullRequestRef(); ok {
			refs[inst] = ref
		}
	}
	m.prChecking = true
	m.prGen++ // a tick already scheduled is superseded by the one this lookup schedules
	return func() tea.Msg {
		var list []git.BranchRef
		seen := make(map[git.BranchRef]bool)
		for _, ref := range refs {
			if !seen[ref] {
				seen[ref] = true
				list = append(list, ref)
			}
		}
		var found map[git.BranchRef]*git.PullRequest
		var err error
		if len(list) > 0 {
			found, err = findPullRequests(list)
		}
		prs := make(map[*session.Instance]*git.PullRequest)
		for inst, ref := range refs {
			if pr, ok := found[ref]; ok {
				prs[inst] = pr
			}
		}
		return prCheckedMsg{prs: prs, err: err}
	}
}

// handlePRChecked shows a lookup's results and schedules the next lookup.
func (m *home) handlePRChecked(msg prCheckedMsg) tea.Cmd {
	m.prChecking = false
	listed := make(map[*session.Instance]bool)
	for _, inst := range m.list.GetInstances() {
		listed[inst] = true
	}
	for inst, pr := range msg.prs {
		if listed[inst] {
			inst.SetPullRequest(pr)
		}
	}

	// gh not being logged in fails every lookup the same way: log it once.
	errText := ""
	if msg.err != nil {
		errText = msg.err.Error()
	}
	if errText != m.prLastErr && errText != "" {
		log.WarningLog.Printf("could not look up pull requests: %v", msg.err)
	}
	m.prLastErr = errText

	if m.prRecheck {
		m.prRecheck = false
		return m.startPRCheck()
	}
	gen := m.prGen
	return tea.Tick(prInterval, func(time.Time) tea.Msg { return prTickMsg{gen: gen} })
}

// handlePRTick starts a scheduled lookup.
func (m *home) handlePRTick(msg prTickMsg) tea.Cmd {
	if msg.gen != m.prGen {
		return nil
	}
	return m.startPRCheck()
}

// prBadgeAt returns the list item whose pull request badge is at screen position (x, y),
// or -1.
func (m *home) prBadgeAt(x, y int) int {
	x, y = m.viewPos(x, y)
	if x < 0 || x >= m.list.Width() {
		return -1
	}
	return m.list.PRBadgeAt(x, y-viewPaddingTop)
}

// openPullRequest opens item's pull request in the browser.
func (m *home) openPullRequest(item int) tea.Cmd {
	pr := m.list.GetInstances()[item].PullRequest()
	if pr == nil || pr.URL == "" {
		return nil
	}
	url := pr.URL
	return tea.Batch(
		m.showInfo(fmt.Sprintf("Opening PR #%d (%s)…", pr.Number, pr.State), 3*time.Second),
		func() tea.Msg {
			if err := openBrowser(url); err != nil {
				return fmt.Errorf("could not open %s: %w", url, err)
			}
			return nil
		},
	)
}

// openBrowser opens url in the default browser. Tests replace it.
var openBrowser = func(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Run()
}
