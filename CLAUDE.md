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
- **`cs prune`**: removes worktree directories under `~/.claude-squad/worktrees` that no saved session (in any repo) uses (`session/git/leftover.go`). Keeps branches; skips dirty/detached worktrees unless `--force`; ignores directories changed in the last 10 minutes; leaves non-cs-named directories alone. The TUI shows a notice on startup when leftovers exist.
- **Worktree setup script**: if a repo has `.claude-squad/setup.sh` (read from the main checkout), it runs in each new session's worktree after creation and before the program starts (`session/setup.go`), with `CS_REPO_PATH`, `CS_WORKTREE_PATH`, `CS_BRANCH`, `CS_SESSION`. Output goes to `~/.claude-squad/setup-logs/`; a failure or 15-minute timeout is reported but doesn't stop the session. Not run on resume.
- **Setup progress**: while a new instance is starting, the list row shows the current step and elapsed time (`running setup.sh… 12s`) instead of the branch, and the preview pane shows a step checklist with timings plus the live tail of the setup script's log (`session/progress.go`, `ui/progress.go`).
- **Clickable tabs**: left-clicking the Preview / Diff / Terminal tabs switches to that tab, and hovering an inactive tab highlights it (`TabbedWindow.TabAt`, `home.handleTabClick`, `home.handleTabHover`). Hover needs `tea.WithMouseAllMotion`. Both are ignored while an overlay is open.
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
