package session

// BusyOp is a slow operation running on an instance in the background, such as resuming
// or pausing it. While one runs, the UI shows it in place of the branch and refuses other
// actions on the instance.
type BusyOp int

const (
	// NotBusy means no operation is running.
	NotBusy BusyOp = iota
	// BusyResuming is Resume: recreating the worktree and restarting the program.
	BusyResuming
	// BusyPausing is Pause: committing changes and removing the worktree.
	BusyPausing
	// BusyKilling is Kill: closing the tmux session and removing the worktree and branch.
	BusyKilling
	// BusyPushing is committing and pushing the branch.
	BusyPushing
)

// Label describes the operation in progress, e.g. "pausing".
func (op BusyOp) Label() string {
	switch op {
	case BusyResuming:
		return "resuming"
	case BusyPausing:
		return "pausing"
	case BusyKilling:
		return "killing"
	case BusyPushing:
		return "pushing"
	}
	return ""
}

// StopsSession reports whether the operation stops or starts the instance's session, so
// its pane and worktree shouldn't be read while it runs. Pushing leaves the session
// running.
func (op BusyOp) StopsSession() bool {
	return op == BusyResuming || op == BusyPausing || op == BusyKilling
}

// Busy returns the operation running on the instance in the background, if any. Call it
// only on the UI goroutine.
func (i *Instance) Busy() BusyOp {
	return i.busy
}

// SetBusy records the operation running on the instance in the background; NotBusy clears
// it. Call it only on the UI goroutine.
func (i *Instance) SetBusy(op BusyOp) {
	i.busy = op
}

// WithWorktreeLock runs fn while holding the lock that diffs, Pause and Kill take, so git
// commands fn runs in the worktree don't collide with theirs.
func (i *Instance) WithWorktreeLock(fn func() error) error {
	i.worktreeMu.Lock()
	defer i.worktreeMu.Unlock()
	return fn()
}
