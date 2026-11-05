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
5. Rebuild and install the local binary from `main`, then restart any open `cs` windows:
   ```sh
   go build -o ~/.local/bin/cs .
   ```

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
