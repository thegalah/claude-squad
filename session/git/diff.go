package git

import (
	"strconv"
	"strings"
)

// DiffStats summarizes the changes in a session's worktree since its base commit.
type DiffStats struct {
	// Files lists the changed files, in git's order.
	Files []FileChange
	// Added is the number of added lines
	Added int
	// Removed is the number of removed lines
	Removed int
	// Error holds any error that occurred during diff computation
	// This allows propagating setup errors (like missing base commit) without breaking the flow
	Error error
}

// FileChange is one changed file and its line counts. It's saved in the state file.
type FileChange struct {
	// Path is the file's path as git prints it; a rename looks like "old => new".
	Path string `json:"path"`
	// Added and Removed are the file's added and removed lines.
	Added   int `json:"added"`
	Removed int `json:"removed"`
	// Binary is set for binary files, which have no line counts.
	Binary bool `json:"binary,omitempty"`
}

func (d *DiffStats) IsEmpty() bool {
	return d.Added == 0 && d.Removed == 0 && len(d.Files) == 0
}

// Diff returns the files changed between the worktree and the base commit, with their
// line counts. It runs `git diff --numstat`, so it's cheap even for large changes.
func (g *GitWorktree) Diff() *DiffStats {
	stats := &DiffStats{}

	// -N stages untracked files (intent to add), including them in the diff
	_, err := g.runGitCommand(g.worktreePath, "add", "-N", ".")
	if err != nil {
		stats.Error = err
		return stats
	}

	out, err := g.runGitCommand(g.worktreePath, "--no-pager", "diff", "--numstat", g.GetBaseCommitSHA())
	if err != nil {
		stats.Error = err
		return stats
	}

	stats.Files = parseNumstat(out)
	for _, f := range stats.Files {
		stats.Added += f.Added
		stats.Removed += f.Removed
	}
	return stats
}

// parseNumstat parses `git diff --numstat` output. Each line is formatted as
// <added>\t<removed>\t<path>; binary files report "-\t-\t<path>".
func parseNumstat(out string) []FileChange {
	var files []FileChange
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) < 3 {
			continue
		}
		if fields[0] == "-" && fields[1] == "-" {
			files = append(files, FileChange{Path: fields[2], Binary: true})
			continue
		}
		a, aerr := strconv.Atoi(fields[0])
		r, rerr := strconv.Atoi(fields[1])
		if aerr != nil || rerr != nil {
			continue
		}
		files = append(files, FileChange{Path: fields[2], Added: a, Removed: r})
	}
	return files
}
