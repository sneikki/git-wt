package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/shhac/git-wt/internal/debug"
	"github.com/shhac/git-wt/internal/git"
	"github.com/shhac/git-wt/internal/picker"
	"github.com/shhac/git-wt/internal/wt"
)

var (
	cleanDryRun       bool
	cleanNoFetch      bool
	cleanOrphanedOnly bool
	cleanGoneOnly     bool
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove worktrees that no longer have a useful branch",
	Long: "Remove worktrees in three cases:\n" +
		"  - orphaned: the local branch was deleted (or was never a branch)\n" +
		"  - upstream-gone: the local branch's upstream is gone AND the branch is\n" +
		"    integrated into origin's default branch (origin/HEAD). Integrated means\n" +
		"    its tip is an ancestor of origin/HEAD, or merging it changes nothing\n" +
		"    (squash merge), or a merged pull request exists at exactly this commit,\n" +
		"    wherever it merged (checked with `gh`, optional: without gh such a branch is\n" +
		"    kept). Anything else is kept and reported with its reason.\n\n" +
		"  - merged PR: a merged pull request exists at exactly the branch's tip,\n" +
		"    whatever its upstream state (still present, other branch, none). Needs gh;\n" +
		"    never deletes a remote branch. Skipped by --orphaned-only and\n" +
		"    --upstream-gone-only.\n\n" +
		"All checks run by default. Use --orphaned-only or --upstream-gone-only\n" +
		"to narrow. `git worktree prune` runs first to clean up worktrees whose\n" +
		"directory no longer exists. `git fetch --prune` runs before the\n" +
		"upstream-gone check (skip with --no-fetch). To remove a kept worktree and\n" +
		"its branch anyway, use `git-wt rm --force --delete-branch --force-branch -- <branch>`.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if cleanOrphanedOnly && cleanGoneOnly {
			return fmt.Errorf("--orphaned-only and --upstream-gone-only are mutually exclusive")
		}
		return runClean(cmd.Context(), cleanFlags{
			dryRun:       cleanDryRun,
			noFetch:      cleanNoFetch,
			orphanedOnly: cleanOrphanedOnly,
			goneOnly:     cleanGoneOnly,
		})
	},
}

// cleanFlags is the resolved flag state for one clean invocation. Pulled out
// of the global flagXxx vars so runClean is callable from tests without
// touching package state.
type cleanFlags struct {
	dryRun, noFetch        bool
	orphanedOnly, goneOnly bool
}

// runClean is the body of `git-wt clean`. Steps: setup → discover targets
// → integration gate → render → maybe-confirm → delegate to executeRm.
func runClean(ctx context.Context, flags cleanFlags) error {
	repo, err := wt.Inspect(ctx, "")
	if err != nil {
		return err
	}

	// Prune any worktrees whose directories have been removed manually.
	// Cheap and almost always desirable.
	if _, err := git.Run(ctx, "worktree", "prune"); err != nil {
		return fmt.Errorf("git worktree prune: %w", err)
	}

	doOrphaned := !flags.goneOnly
	doGone := !flags.orphanedOnly

	if doGone && !flags.noFetch {
		fmt.Fprintln(os.Stderr, "fetching with --prune…")
		if _, err := git.Run(ctx, "fetch", "--prune"); err != nil {
			// Soft-fail: maybe no remote, or offline. Carry on; the orphaned
			// check still works without a fetch.
			fmt.Fprintf(os.Stderr, "warning: git fetch --prune failed: %v\n", err)
		}
	}

	wts, cur, err := loadWorktrees(ctx)
	if err != nil {
		return err
	}

	branchExists := func(b string) (bool, error) { return wt.BranchExists(ctx, "", b) }
	listGone := func() ([]string, error) { return goneBranches(ctx) }
	targets, err := collectCleanTargets(wts, repo, doOrphaned, doGone, branchExists, listGone)
	if err != nil {
		return err
	}
	targets, kept := splitByIntegration(targets, gitIntegration(ctx))
	printKept(os.Stderr, kept)
	if !flags.orphanedOnly && !flags.goneOnly {
		targets = append(targets, scanMergedPRs(ctx, wts, repo, cur, targets, kept)...)
	}
	if len(targets) == 0 {
		fmt.Fprintln(os.Stderr, "nothing to clean")
		return nil
	}

	toRm := scanDirty(ctx, cleanTargetsToRm(targets))

	printCleanTargets(os.Stderr, targets, dirtyByPath(toRm))
	if flags.dryRun {
		return nil
	}

	// A gone branch says nothing about whether the worktree still holds
	// work, so clean refuses to destroy uncommitted changes even though the
	// user asked for a sweep. `rm --force <branch>` is the way to insist.
	toRm = skipLocked(os.Stderr, skipDirty(os.Stderr, toRm))
	if len(toRm) == 0 {
		fmt.Fprintln(os.Stderr, "nothing left to clean")
		return nil
	}

	if interactive() {
		ok, err := confirmClean(len(toRm))
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(os.Stderr, "cancelled")
			return nil
		}
	}

	// branchForce=true: these branches are gone or merged by definition of
	// how clean picked them, so -d would fail on exactly the ones it should
	// remove.
	return executeRm(ctx, repo, toRm, cur, rmTreeAndBranch, true)
}

// printCleanTargets writes the human-readable target list. Pure: takes any io.Writer.
func printCleanTargets(w io.Writer, targets []taggedTarget, dirty map[string]wt.DirtyStat) {
	_, _ = fmt.Fprintln(w, "worktrees to remove:")
	for _, t := range targets {
		note := ""
		if d, ok := dirty[t.wt.Path]; ok {
			note = fmt.Sprintf("  [dirty: %s]", d.Summary())
		}
		if t.wt.Locked {
			note += "  [" + lockLabel(t.wt) + "]"
		}
		_, _ = fmt.Fprintf(w, "  %s  [%s]  (%s)%s\n", t.wt.Display(), t.reason, t.wt.Path, note)
	}
}

// confirmClean prompts the user for final approval (interactive only). ESC /
// Ctrl-C are treated as a cancel response (returns false, nil) rather than
// an error.
func confirmClean(n int) (_ bool, err error) {
	end := debug.Op("pick.confirm", "clean")
	defer func() { end(err) }()

	choice, ok, err := picker.Confirm(
		fmt.Sprintf("Remove these %d worktree(s) and their branches?", n),
		[]picker.Option[bool]{
			{Label: "Remove", Value: true},
			{Label: "Cancel", Value: false},
		},
	)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	return choice, nil
}

func init() {
	rootCmd.AddCommand(cleanCmd)
	cleanCmd.Flags().BoolVar(&cleanDryRun, "dry-run", false, "list candidates without removing them")
	cleanCmd.Flags().BoolVar(&cleanNoFetch, "no-fetch", false, "skip the leading `git fetch --prune`")
	cleanCmd.Flags().BoolVar(&cleanOrphanedOnly, "orphaned-only", false, "only remove worktrees whose local branch is gone")
	cleanCmd.Flags().BoolVar(&cleanGoneOnly, "upstream-gone-only", false, "only remove worktrees whose upstream tracking is gone")
}

// cleanTargetsToRm unwraps the cleanup reasons, which only the listing
// needs, into the removal units the rm pipeline works in.
func cleanTargetsToRm(targets []taggedTarget) []rmTarget {
	out := make([]rmTarget, len(targets))
	for i, t := range targets {
		out[i] = rmTarget{Worktree: t.wt}
	}
	return out
}
