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

// reasonMergedPR tags a target picked because a merged pull request sits at
// exactly the branch tip, whatever the branch's upstream state.
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

// findMergedPRWorktrees returns eligible worktrees whose branch proof holds,
// in input order. A proof error is no proof. gh calls run in parallel.
func findMergedPRWorktrees(wts []wt.Worktree, env prScanEnv, proof integrationFn) []taggedTarget {
	var cands []wt.Worktree
	for _, w := range wts {
		if env.eligible(w) {
			cands = append(cands, w)
		}
	}
	ok := make([]bool, len(cands))
	var wg sync.WaitGroup
	sem := make(chan struct{}, prScanConcurrency)
	for i, w := range cands {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			yes, err := proof(w.Branch)
			ok[i] = err == nil && yes
		}()
	}
	wg.Wait()
	var out []taggedTarget
	for i, w := range cands {
		if ok[i] {
			out = append(out, taggedTarget{wt: w, reason: reasonMergedPR})
		}
	}
	return out
}

// defaultBranchName is origin's default branch short name, "" when unknown.
func defaultBranchName(ctx context.Context) string {
	ref, err := git.Run(ctx, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(ref, "origin/")
}

// scanMergedPRs finds worktrees with a merged PR at their tip among those
// not already targeted or kept by the other scans.
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
	return findMergedPRWorktrees(wts, env, prProof(tipOf(ctx), ghMergedHeads(ctx)))
}
