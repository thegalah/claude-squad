package main

import (
	"bufio"
	"claude-squad/config"
	"claude-squad/log"
	"claude-squad/session"
	"claude-squad/session/git"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	pruneYesFlag    bool
	pruneForceFlag  bool
	pruneDryRunFlag bool

	pruneCmd = &cobra.Command{
		Use:   "prune",
		Short: "Remove leftover worktrees that no session uses",
		Long: `Remove worktree directories under the claude-squad worktrees directory that no saved
session (in any repository) refers to. These are left behind by failed kills or older
versions and can take a lot of disk space.

Branches are never deleted. Worktrees with uncommitted changes or a detached HEAD are
skipped unless --force is given. Directories changed in the last 10 minutes are ignored,
since they may belong to a session that is still being created.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			log.Initialize(false)
			defer log.Close()
			return runPrune(os.Stdin, os.Stdout)
		},
	}
)

func init() {
	pruneCmd.Flags().BoolVarP(&pruneYesFlag, "yes", "y", false, "Remove without asking for confirmation")
	pruneCmd.Flags().BoolVar(&pruneForceFlag, "force", false,
		"Also remove worktrees with uncommitted changes or a detached HEAD")
	pruneCmd.Flags().BoolVar(&pruneDryRunFlag, "dry-run", false, "Only list what would be removed")
	rootCmd.AddCommand(pruneCmd)
}

func runPrune(in io.Reader, out io.Writer) error {
	tracked, err := session.TrackedWorktreePaths(config.LoadState())
	if err != nil {
		return err
	}
	leftovers, err := git.FindLeftoverWorktrees(tracked, git.LeftoverMinAge, true)
	if err != nil {
		return err
	}
	configDir, _ := config.GetConfigDir()
	root := filepath.Join(configDir, "worktrees")

	if len(leftovers) == 0 {
		fmt.Fprintf(out, "No leftover worktrees in %s\n", root)
		return nil
	}

	var toRemove []git.LeftoverWorktree
	fmt.Fprintf(out, "Leftover worktrees in %s:\n\n", root)
	for _, w := range leftovers {
		rel, err := filepath.Rel(root, w.Path)
		if err != nil {
			rel = w.Path
		}
		action := "remove"
		if w.NeedsForce() && !pruneForceFlag {
			action = "skip  "
		} else {
			toRemove = append(toRemove, w)
		}
		fmt.Fprintf(out, "  %s  %s  (%s)\n", action, rel, describeLeftover(w))
	}
	fmt.Fprintln(out)

	if skipped := len(leftovers) - len(toRemove); skipped > 0 {
		fmt.Fprintf(out, "%d skipped because they may contain unsaved work; use --force to remove them too.\n", skipped)
	}
	if len(toRemove) == 0 {
		return nil
	}
	if pruneDryRunFlag {
		fmt.Fprintf(out, "Dry run: would remove %d worktree(s).\n", len(toRemove))
		return nil
	}
	if !pruneYesFlag {
		fmt.Fprintf(out, "Remove %d worktree(s)? Branches are kept. [y/N] ", len(toRemove))
		answer, _ := bufio.NewReader(in).ReadString('\n')
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(out, "Aborted.")
			return nil
		}
	}

	removed := 0
	var failures []string
	for _, w := range toRemove {
		if err := git.RemoveLeftoverWorktree(w); err != nil {
			failures = append(failures, err.Error())
			continue
		}
		removed++
	}
	fmt.Fprintf(out, "Removed %d worktree(s).\n", removed)
	if len(failures) > 0 {
		return fmt.Errorf("failed to remove %d worktree(s):\n  %s", len(failures), strings.Join(failures, "\n  "))
	}
	return nil
}

func describeLeftover(w git.LeftoverWorktree) string {
	if !w.Valid {
		return "broken, no git worktree"
	}
	var parts []string
	if w.Branch != "" {
		parts = append(parts, "branch "+w.Branch)
	}
	if w.Unreachable {
		parts = append(parts, "detached HEAD on a commit no branch has")
	} else if w.Detached {
		parts = append(parts, "detached HEAD")
	}
	if w.Dirty {
		parts = append(parts, git.DescribeChanges(w.Changes))
	}
	parts = append(parts, "last changed "+w.ModTime.Format("2006-01-02"))
	return strings.Join(parts, ", ")
}
