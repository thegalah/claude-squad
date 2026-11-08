package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// LeftoverMinAge is how old a worktree directory must be before it is treated as leftover.
// A cs process creates a session's worktree before it saves the session, so a very recent
// directory that no saved session references may still be in the middle of being created.
const LeftoverMinAge = 10 * time.Minute

// LeftoverWorktree is a directory under the cs worktrees directory that no saved session
// references, e.g. left behind by a failed kill or by an older version of cs.
type LeftoverWorktree struct {
	Path string
	// Valid is true when the directory is still a working git worktree. Otherwise it is a
	// broken leftover (no .git, or its repository metadata is gone).
	Valid bool
	// Branch is the checked-out branch of a valid worktree; empty if HEAD is detached.
	Branch string
	// Dirty is true when a valid worktree has uncommitted changes.
	Dirty bool
	// Changes is the number of uncommitted files in a dirty worktree (0 if unknown).
	Changes int
	// Detached is true when a valid worktree has a detached HEAD.
	Detached bool
	// Unreachable is true when a detached HEAD's commit isn't on any branch (local or
	// remote), so removing the worktree could lose commits.
	Unreachable bool
	ModTime     time.Time
}

// NeedsForce reports whether removing w could lose work that isn't saved on a branch.
func (w LeftoverWorktree) NeedsForce() bool {
	return w.Valid && (w.Dirty || w.Unreachable)
}

// cs names worktree directories <sanitized branch>_<hex unix nanos>.
var csWorktreeName = regexp.MustCompile(`_[0-9a-f]{8,}$`)

// FindLeftoverWorktrees returns the directories under the cs worktrees directory that are
// not referenced by tracked (the worktree paths of all saved sessions) and are older than
// minAge. When inspect is false, only paths and modification times are filled in, which
// avoids running git in every directory.
func FindLeftoverWorktrees(tracked []string, minAge time.Duration, inspect bool) ([]LeftoverWorktree, error) {
	root, err := getWorktreeDirectory()
	if err != nil {
		return nil, err
	}
	return findLeftoverWorktrees(root, tracked, minAge, inspect, time.Now())
}

func findLeftoverWorktrees(root string, tracked []string, minAge time.Duration, inspect bool, now time.Time) ([]LeftoverWorktree, error) {
	trackedSet := make(map[string]bool, len(tracked))
	for _, p := range tracked {
		if p != "" {
			trackedSet[filepath.Clean(p)] = true
		}
	}

	var leftovers []LeftoverWorktree
	var walk func(dir string) error
	walk = func(dir string) error {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("failed to read %s: %w", dir, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			_, gitErr := os.Lstat(filepath.Join(path, ".git"))
			isWorktree := gitErr == nil
			if !isWorktree && !csWorktreeName.MatchString(entry.Name()) {
				// A directory for a branch prefix (e.g. "user/" in "user/feature"); look
				// inside. Anything else that isn't named like a cs worktree is left alone,
				// except an empty directory, which is safe to remove.
				if isEmptyDir(path) {
					if info, err := os.Stat(path); err == nil && now.Sub(info.ModTime()) >= minAge {
						leftovers = append(leftovers, LeftoverWorktree{Path: path, ModTime: info.ModTime()})
					}
					continue
				}
				if err := walk(path); err != nil {
					return err
				}
				continue
			}
			if trackedSet[path] {
				continue
			}
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			if now.Sub(info.ModTime()) < minAge {
				continue
			}
			w := LeftoverWorktree{Path: path, ModTime: info.ModTime()}
			if isWorktree && inspect {
				inspectWorktree(&w)
			} else if isWorktree {
				w.Valid = true
			}
			leftovers = append(leftovers, w)
		}
		return nil
	}
	if err := walk(root); err != nil {
		return nil, err
	}
	return leftovers, nil
}

// inspectWorktree fills in the git state of a worktree directory.
func inspectWorktree(w *LeftoverWorktree) {
	if _, err := gitOutput(w.Path, "rev-parse", "--is-inside-work-tree"); err != nil {
		// .git exists but git can't use it (e.g. the main repository's worktree metadata
		// was pruned or the repository was deleted).
		return
	}
	w.Valid = true
	if status, err := gitOutput(w.Path, "status", "--porcelain"); err != nil {
		// Treat an unreadable status as dirty, so removal requires --force.
		w.Dirty = true
	} else if status != "" {
		w.Dirty = true
		w.Changes = strings.Count(status, "\n") + 1
	}
	if branch, err := gitOutput(w.Path, "symbolic-ref", "--short", "-q", "HEAD"); err == nil && branch != "" {
		w.Branch = branch
		return
	}
	w.Detached = true
	// A detached HEAD on a commit that some branch contains loses nothing when removed.
	onBranch, err := gitOutput(w.Path, "for-each-ref", "--count=1", "--contains", "HEAD", "--format=%(refname)",
		"refs/heads", "refs/remotes")
	w.Unreachable = err != nil || onBranch == ""
}

func gitOutput(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	return strings.TrimSpace(string(out)), err
}

// RemoveLeftoverWorktree deletes a leftover worktree directory. A valid worktree is removed
// with `git worktree remove` so its repository forgets it; its branch is kept. Empty parent
// directories inside the worktrees directory are removed too.
func RemoveLeftoverWorktree(w LeftoverWorktree) error {
	root, err := getWorktreeDirectory()
	if err != nil {
		return err
	}
	return removeLeftoverWorktree(root, w)
}

func removeLeftoverWorktree(root string, w LeftoverWorktree) error {
	if !isWithin(root, w.Path) {
		return fmt.Errorf("refusing to remove %s: not inside %s", w.Path, root)
	}
	if w.Valid {
		if commonDir, err := gitOutput(w.Path, "rev-parse", "--path-format=absolute", "--git-common-dir"); err == nil && commonDir != "" {
			// If this fails, the directory is deleted below and prune drops the metadata.
			_ = exec.Command("git", "--git-dir", commonDir, "worktree", "remove", "--force", w.Path).Run()
			defer exec.Command("git", "--git-dir", commonDir, "worktree", "prune").Run()
		}
	}
	if err := os.RemoveAll(w.Path); err != nil {
		return fmt.Errorf("failed to remove %s: %w", w.Path, err)
	}
	removeEmptyParents(root, filepath.Dir(w.Path))
	return nil
}

// isWithin reports whether path is strictly inside root.
func isWithin(root, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// removeEmptyParents removes dir and its ancestors while they are empty, stopping at root.
func removeEmptyParents(root, dir string) {
	for isWithin(root, dir) {
		if err := os.Remove(dir); err != nil {
			return // not empty, or already gone
		}
		dir = filepath.Dir(dir)
	}
}

func isEmptyDir(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) == 0
}

// DescribeChanges describes a dirty worktree's uncommitted changes.
func DescribeChanges(n int) string {
	switch n {
	case 0:
		return "uncommitted changes"
	case 1:
		return "1 uncommitted file"
	default:
		return fmt.Sprintf("%d uncommitted files", n)
	}
}
