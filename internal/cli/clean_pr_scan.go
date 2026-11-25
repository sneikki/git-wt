package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/shhac/git-wt/internal/git"
	"github.com/shhac/git-wt/internal/wt"
)

// reasonMergedPR tags a target picked because a pull request merged into the
// default branch sits at exactly the branch tip, whatever the upstream state.
const reasonMergedPR = "merged PR at tip"

// prScanConcurrency bounds the parallel gh calls of the scan.
const prScanConcurrency = 8

// prScanEnv is what the scan must not touch or has already decided.
type prScanEnv struct {
	mainRoot      string
	curPath       string
	defaultBranch string
	skip          map[string]struct{} // worktree paths already targeted or kept
}

// eligible reports whether w may be tested for a merged PR: a plain, unlocked
// branch worktree that is neither main, current, nor the default branch.
func (e prScanEnv) eligible(w wt.Worktree) bool {
	if w.Path == e.mainRoot || w.Path == e.curPath || w.Branch == "" || w.Branch == e.defaultBranch {
		return false
	}
	if w.Detached || w.Bare || w.Locked || w.Prunable {
		return false
	}
	_, seen := e.skip[w.Path]
	return !seen
}

// prVerdict is the scan's answer for one branch: pick it, keep it with a
// printed reason, or say nothing (no proof).
type prVerdict struct {
	pick bool
	keep string
}

// prVerdictFn decides one branch. Errors are folded into the verdict.
type prVerdictFn func(branch string) prVerdict

// mergedPRVerdict proves a branch by a PR merged into base at exactly the
// tip, then fails closed when the branch is itself the base of any PR (a
// long-lived or parent branch). isBase is asked only for proven branches.
func mergedPRVerdict(tip tipFn, prs mergedPRsFn, isBase isBaseFn, base string) prVerdictFn {
	return func(branch string) prVerdict {
		t, err := tip(branch)
		if err != nil {
			return prVerdict{}
		}
		found, err := prs(branch)
		if err != nil || !hasMergedAt(found, t, base) {
			return prVerdict{}
		}
		isParent, err := isBase(branch)
		switch {
		case err != nil:
			return prVerdict{keep: "could not check whether it is the base of a pull request: " + err.Error()}
		case isParent:
			return prVerdict{keep: "branch is the base of a pull request"}
		}
		return prVerdict{pick: true}
	}
}

// hasMergedAt reports whether some PR was merged into base at commit tip.
// An empty tip matches nothing. Pure.
func hasMergedAt(prs []mergedPR, tip, base string) bool {
	if tip == "" {
		return false
	}
	for _, p := range prs {
		if p.HeadOid == tip && p.Base == base {
			return true
		}
	}
	return false
}

// findMergedPRWorktrees returns the picked worktrees and the kept ones with
// reasons, in input order. gh calls run in parallel.
func findMergedPRWorktrees(wts []wt.Worktree, env prScanEnv, decide prVerdictFn) ([]taggedTarget, []keptTarget) {
	var cands []wt.Worktree
	for _, w := range wts {
		if env.eligible(w) {
			cands = append(cands, w)
		}
	}
	verdicts := make([]prVerdict, len(cands))
	var wg sync.WaitGroup
	sem := make(chan struct{}, prScanConcurrency)
	for i, w := range cands {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			verdicts[i] = decide(w.Branch)
		}()
	}
	wg.Wait()
	var out []taggedTarget
	var kept []keptTarget
	for i, w := range cands {
		t := taggedTarget{wt: w, reason: reasonMergedPR}
		switch {
		case verdicts[i].pick:
			out = append(out, t)
		case verdicts[i].keep != "":
			kept = append(kept, keptTarget{t, verdicts[i].keep})
		}
	}
	return out, kept
}

// defaultBranchName is origin's default branch short name, "" when unknown.
func defaultBranchName(ctx context.Context) string {
	ref, err := git.Run(ctx, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(ref, "origin/")
}

// scanMergedPRs finds worktrees with a PR merged into the default branch at
// their tip among those not already targeted or kept by the other scans, and
// prints the ones it keeps. Without a known default branch it does nothing.
func scanMergedPRs(ctx context.Context, wts []wt.Worktree, repo *wt.RepoInfo, cur *wt.Worktree, targets []taggedTarget, kept []keptTarget) []taggedTarget {
	skip := pathSet(targets)
	for _, k := range kept {
		skip[k.target.wt.Path] = struct{}{}
	}
	def := defaultBranchName(ctx)
	if def == "" {
		fmt.Fprintln(os.Stderr, "merged-PR scan skipped: origin's default branch unknown (run git remote set-head origin --auto)")
		return nil
	}
	env := prScanEnv{mainRoot: repo.MainRoot, defaultBranch: def, skip: skip}
	if cur != nil {
		env.curPath = cur.Path
	}
	decide := mergedPRVerdict(tipOf(ctx), ghMergedPRs(ctx), ghIsBase(ctx), def)
	picked, keptHere := findMergedPRWorktrees(wts, env, decide)
	printKept(os.Stderr, keptHere)
	return picked
}
