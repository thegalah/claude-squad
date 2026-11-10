# CLAUDE.md

This is **thegalah's personal fork** of [smtg-ai/claude-squad](https://github.com/smtg-ai/claude-squad), a TUI (`cs`) that runs multiple AI coding agents, each in its own tmux session and git worktree.

## Remotes

| Remote | Repo | Access |
|---|---|---|
| `origin` | `thegalah/claude-squad` (this fork) | write: all PRs go here |
| `upstream` | `smtg-ai/claude-squad` | read-only |

**`gh` resolves to the upstream repo by default.** Always pass `-R thegalah/claude-squad` to `gh pr create`, `gh pr merge`, `gh pr view` etc., or the PR targets upstream.

To pull upstream changes in: `git fetch upstream && git merge upstream/main` on a branch, then PR it to the fork like any other change.

## Fork changes (not in upstream)

- **mise.toml**: pins Go to the `toolchain` version in `go.mod`.
- **Per-repo session scoping**: each `cs` process only loads and saves instances whose repo root matches the git repo it was started in (`session.NewRepoStorage`). Saves read-modify-write `~/.claude-squad/state.json` so concurrent `cs` processes in different repos don't clobber each other; state is written atomically. The daemon and `cs reset` remain global.
- **Resume continues the claude conversation**: when Resume has to start a new tmux session, claude is relaunched with `--continue` if it has a saved conversation for the worktree (see `Instance.resumeCommand` in `session/instance.go`).
- **Repo name in the list title**: `Instances · <repo>`, so `cs` windows in different repos can be told apart.
- **Terminal title**: `cs · <repo>` (set on startup and re-set after detaching from a session), so terminal tabs can be told apart.
- **`cs prune`**: removes worktree directories under `~/.claude-squad/worktrees` that no saved session (in any repo) uses (`session/git/leftover.go`). Keeps branches; skips worktrees that would lose work (uncommitted changes, or a detached HEAD on a commit no branch contains) unless `--force`; ignores directories changed in the last 10 minutes; leaves non-cs-named directories alone. The TUI shows a notice on startup when leftovers exist, and pressing `shift+p` (shown in the bottom menu) on the main screen confirms and removes the safe ones, then asks separately about the ones with unsaved work, listing what each would lose (`app/prune.go`).
- **Worktree setup script**: if a repo has `.claude-squad/setup.sh` (read from the main checkout), it runs in each new session's worktree after creation and before the program starts (`session/setup.go`), with `CS_REPO_PATH`, `CS_WORKTREE_PATH`, `CS_BRANCH`, `CS_SESSION`. Output goes to `~/.claude-squad/setup-logs/`; a failure or 15-minute timeout is reported but doesn't stop the session. Not run on resume.
- **Setup progress**: while a new instance is starting, the list row shows the current step and elapsed time (`running setup.sh… 12s`) instead of the branch, and the preview pane shows a step checklist with timings plus the live tail of the setup script's log (`session/progress.go`, `ui/progress.go`).
- **Mouse support**: clicking a session in the list selects it and clicking a Preview / Diff / Terminal tab switches to it; hovering either highlights it (`app/mouse.go`, `List.ItemAt`, `TabbedWindow.TabAt`). The selected or hovered session's row shows a `→` that attaches when clicked (`List.OpenArrowAt`), and clicking inside the Preview or Terminal pane of an attachable session attaches too; hovering the pane adds `→ click to open` to its tab. Both go through `home.attachSelected`, same as Enter. Needs `tea.WithMouseAllMotion`. Ignored while an overlay is open. The view is one line taller than the terminal, so Bubble Tea drops its top line(s); `home.viewOverflow` corrects mouse rows for that, and `home.viewIndent` corrects columns when the menu is wider than the list and panes, so `JoinVertical` centers them. While attached, `ctrl+q` is found anywhere in a read, since mouse-motion bytes can arrive with it (`splitAtDetachKey`).
- **Back button while attached**: attaching moves the session's tmux status line to the top, with a `← back` button on its left in place of the window list (`session/tmux/back_button.go`). tmux only draws it: cs reads the click from the terminal's SGR mouse reports before forwarding input to tmux, and detaches when the button is released over it, like ctrl+q. A click whose report is split inside its `ESC [ <` prefix across two reads reaches tmux instead (harmless).
- **Responsiveness**: the UI goroutine doesn't run slow tmux/git work. The preview tick (50ms) captures the pane in the background and drops stale captures (`app/refresh.go`); status/diff polling runs every 250ms, with Running→Ready held until output has been quiet for 500ms. Resume, pause (checkout), kill and push run in the background (`app/busy.go`): the instance is marked busy (`session.BusyOp`), its row shows `pausing…` etc., other actions on it are refused, and `q`/auto-restart wait (ctrl+c doesn't). `Instance.worktreeMu` keeps diffs from colliding with pause/kill/push git commands. The status check reuses the preview's capture of the selected pane when it's under 100ms old (`TmuxSession.recentCapture`), so that pane isn't captured twice.
- **Diff tab lists changed files**: instead of the full patch, it shows each changed file with its `+`/`-` line counts (binary files marked), from `git diff --numstat` for every session (`session/git/diff.go`, `ui/diff.go`). The file list is saved in `state.json` so paused sessions keep it; the old `content` field is ignored.
- **Auto-restart on new binary**: `cs` watches its own executable and, when it's replaced (e.g. `go build -o ~/.local/bin/cs .`), saves and re-execs itself in the same terminal (`app/restart.go`). It waits until the file stops changing and `cs version` succeeds on it, and until it's idle: no overlay/prompt open, no instance starting, not attached. Instances live in tmux, so they're unaffected. The restarted process shows a notice.

## Development workflow

Tooling: Go via mise (`mise install` in this folder), plus tmux, git and gh. Node is only needed for `web/` (the marketing site), which is not part of the binary.

1. Branch off `main`: `git checkout -b <type>/<short-name>` (`fix/`, `feat/`, `chore/`).
2. Make the change and add tests.
3. Check:
   ```sh
   gofmt -l .                                   # must print nothing (CI enforces it)
   go vet ./...
   go test -skip TestGetClaudeCommand ./...
   ```
   `config.TestGetClaudeCommand` hangs in non-interactive shells because it sources `~/.zshrc`; this is pre-existing, not a regression.
4. Commit, push to `origin`, open a PR on the fork and squash-merge it:
   ```sh
   git push -u origin <branch>
   gh pr create -R thegalah/claude-squad --base main --head <branch> --title "..." --body "..."
   gh pr merge <url> -R thegalah/claude-squad --squash --delete-branch
   git checkout main && git pull origin main && git branch -D <branch>
   ```
5. Rebuild and install the local binary from `main`:
   ```sh
   go build -o ~/.local/bin/cs .
   ```
   Open `cs` windows restart themselves onto the new binary within a few seconds (once idle).

Don't bump the version (`bump-version.sh`) or touch release workflows; those are upstream's release process.

## Code map

- `main.go`: cobra CLI (`cs`, `cs reset`, `cs version`, daemon flag).
- `app/`: Bubble Tea TUI; key handling in `app.go`.
- `session/instance.go`: instance lifecycle (Start, Pause/checkout, Resume, Kill).
- `session/storage.go`: (de)serialising instances, per-repo scoping.
- `session/tmux/`: tmux session management and prompt detection.
- `session/git/`: worktrees, branches, diffs.
- `config/`: `~/.claude-squad/config.json` (config) and `state.json` (instances, help-screen state).
- `daemon/`: background AutoYes process.
- `ui/`: list, panes, overlays.
