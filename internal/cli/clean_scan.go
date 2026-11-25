package cli

import (
	"context"
	"strings"

	"github.com/shhac/git-wt/internal/git"
	"github.com/shhac/git-wt/internal/wt"
)

// reasonUpstreamGone tags a target picked because its branch's upstream is
// gone. Only targets carrying it are checked for integration before removal.
const reasonUpstreamGone = "upstream gone"

// goneBranchesFn is the DI seam for the upstream-gone scan, mirroring
// branchExistsFn for the orphaned scan. The cli wires the real git-backed
// implementation; tests pass a fake.
type goneBranchesFn func() ([]string, error)

// collectCleanTargets runs the requested checks and returns a deduped target
// list. doOrphaned/doGone gate the two scans. branchExists and listGone are
// the DI seams over git so the function is unit-testable end-to-end.
func collectCleanTargets(
	wts []wt.Worktree,
	repo *wt.RepoInfo,
	doOrphaned, doGone bool,
	branchExists branchExistsFn,
	listGone goneBranchesFn,
) ([]taggedTarget, error) {
	var targets []taggedTarget
	if doOrphaned {
		ts, err := findOrphanedWorktrees(wts, repo, branchExists)
		if err != nil {
			return nil, err
		}
		targets = append(targets, ts...)
	}
	if doGone {
		gone, err := listGone()
		if err != nil {
			return nil, err
		}
		ts := findUpstreamGoneWorktrees(wts, repo, goneSetFromList(gone))
		seen := pathSet(targets)
		for _, t := range ts {
			if _, dupe := seen[t.wt.Path]; !dupe {
				targets = append(targets, t)
			}
		}
	}
	return targets, nil
}

// taggedTarget pairs a worktree with the reason it was selected for cleanup.
type taggedTarget struct {
	wt     wt.Worktree
	reason string
}

func pathSet(ts []taggedTarget) map[string]struct{} {
	out := make(map[string]struct{}, len(ts))
	for _, t := range ts {
		out[t.wt.Path] = struct{}{}
	}
	return out
}

// branchExistsFn is the dependency-injection seam for BranchExists. The cli
// uses wt.BranchExists; tests pass a fake.
type branchExistsFn func(branch string) (bool, error)

// findOrphanedWorktrees returns worktrees whose local branch no longer exists,
// or who report as detached/prunable (i.e. the branch is gone or never was).
// branchExists is injected so the classification logic is testable without
// running git.
func findOrphanedWorktrees(wts []wt.Worktree, repo *wt.RepoInfo, branchExists branchExistsFn) ([]taggedTarget, error) {
	var out []taggedTarget
	for _, t := range wts {
		if t.Path == repo.MainRoot {
			continue
		}
		if t.Prunable {
			out = append(out, taggedTarget{wt: t, reason: "prunable"})
			continue
		}
		if t.Branch == "" {
			// Detached or bare — not a deletable-branch case but a worktree
			// without a branch is functionally orphaned for our purposes.
			out = append(out, taggedTarget{wt: t, reason: "detached"})
			continue
		}
		exists, err := branchExists(t.Branch)
		if err != nil {
			return nil, err
		}
		if !exists {
			out = append(out, taggedTarget{wt: t, reason: "branch deleted"})
		}
	}
	return out, nil
}

// findUpstreamGoneWorktrees returns worktrees whose branch is in goneSet.
// Pure: caller computes goneSet (typically via goneBranches→parseGoneBranches).
func findUpstreamGoneWorktrees(wts []wt.Worktree, repo *wt.RepoInfo, goneSet map[string]struct{}) []taggedTarget {
	var out []taggedTarget
	for _, t := range wts {
		if t.Path == repo.MainRoot || t.Branch == "" {
			continue
		}
		if _, ok := goneSet[t.Branch]; ok {
			out = append(out, taggedTarget{wt: t, reason: reasonUpstreamGone})
		}
	}
	return out
}

// goneBranches returns local branches whose upstream has been pruned ([gone]).
// Wraps the git call + the pure parser; tests target parseGoneBranches.
func goneBranches(ctx context.Context) ([]string, error) {
	out, err := git.Run(ctx,
		"for-each-ref",
		"--format=%(refname:short)\t%(upstream:track)",
		"refs/heads/",
	)
	if err != nil {
		return nil, err
	}
	return parseGoneBranches(out), nil
}

// parseGoneBranches extracts branch names whose tracking column contains
// "gone" from the output of `git for-each-ref --format=…\t%(upstream:track)`.
func parseGoneBranches(output string) []string {
	var gone []string
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		name, track, _ := strings.Cut(line, "\t")
		if strings.Contains(track, "gone") {
			gone = append(gone, name)
		}
	}
	return gone
}

// goneSetFromList is a small helper for tests and callers that need the
// gone-branches list as a set.
func goneSetFromList(names []string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, n := range names {
		out[n] = struct{}{}
	}
	return out
}
