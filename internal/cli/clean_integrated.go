package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/shhac/git-wt/internal/git"
)

// integrationFn reports whether a local branch is integrated into origin's
// default branch. An error means the check could not run; the caller keeps
// the branch rather than guess.
type integrationFn func(branch string) (bool, error)

// keptTarget is an upstream-gone worktree clean will not remove, with why.
type keptTarget struct {
	target taggedTarget
	why    string
}

// splitByIntegration keeps every upstream-gone target whose branch is not
// integrated into the default branch. Targets picked for other reasons pass
// through unchecked, as before. Pure: the check is injected.
func splitByIntegration(targets []taggedTarget, integrated integrationFn) ([]taggedTarget, []keptTarget) {
	var remove []taggedTarget
	var kept []keptTarget
	for _, t := range targets {
		if t.reason != reasonUpstreamGone {
			remove = append(remove, t)
			continue
		}
		ok, err := integrated(t.wt.Branch)
		switch {
		case err != nil:
			kept = append(kept, keptTarget{t, "integration check failed: " + err.Error()})
		case !ok:
			kept = append(kept, keptTarget{t, "not merged into origin/HEAD, and no merged PR at this exact commit"})
		default:
			remove = append(remove, t)
		}
	}
	return remove, kept
}

// printKept reports each kept worktree with its reason and the command that
// removes it anyway. Pure: takes any io.Writer.
func printKept(w io.Writer, kept []keptTarget) {
	for _, k := range kept {
		b := k.target.wt.Branch
		_, _ = fmt.Fprintf(w, "skipping %s: %s, %s. Insist: %s\n",
			b, k.target.reason, k.why, insistCommand(b))
	}
}

// insistCommand is the rm invocation that removes a kept branch and its
// worktree anyway. `--` ends the flags, so a branch named like a flag is still
// read as a branch. Pure.
func insistCommand(branch string) string {
	return "git-wt rm --force --delete-branch --force-branch -- " + branch
}

// integratedBy composes the proofs: git first, GitHub only when git says no.
// An error from git is final, so an unresolvable origin/HEAD never reaches
// GitHub. Pure: both proofs are injected.
func integratedBy(gitProof, prProof integrationFn) integrationFn {
	return func(branch string) (bool, error) {
		ok, err := gitProof(branch)
		if err != nil || ok {
			return ok, err
		}
		return prProof(branch)
	}
}

// gitIntegration is the integrationFn clean uses. origin/HEAD is resolved
// once; when it does not resolve, every check fails with that reason.
func gitIntegration(ctx context.Context) integrationFn {
	head, headErr := originHead(ctx)
	gitProof := func(branch string) (bool, error) {
		if headErr != nil {
			return false, headErr
		}
		return branchIntegrated(ctx, branch, head)
	}
	return integratedBy(gitProof, prProof(tipOf(ctx), ghMergedHeads(ctx)))
}

// originHead resolves refs/remotes/origin/HEAD, as clean's fetch left it, to
// a commit. Never the local main.
func originHead(ctx context.Context) (string, error) {
	sha, err := git.Run(ctx, "rev-parse", "--verify", "-q", "refs/remotes/origin/HEAD^{commit}")
	if err != nil {
		return "", errors.New("origin/HEAD unresolvable (run git remote set-head origin --auto)")
	}
	return sha, nil
}

// branchIntegrated reports whether branch adds nothing to head: its tip is an
// ancestor of head (a regular or fast-forward merge), or merging it into head
// yields head's own tree (a squash merge). A merge that would conflict is not
// integrated.
func branchIntegrated(ctx context.Context, branch, head string) (bool, error) {
	tip := "refs/heads/" + branch
	_, err := git.Run(ctx, "merge-base", "--is-ancestor", tip, head)
	if err == nil {
		return true, nil
	}
	if !exitCodeIs(err, 1) {
		return false, err
	}
	merged, err := git.Run(ctx, "merge-tree", "--write-tree", head, tip)
	if exitCodeIs(err, 1) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	tree, err := git.Run(ctx, "rev-parse", "--verify", "-q", head+"^{tree}")
	if err != nil {
		return false, err
	}
	return mergeIsNoop(merged, tree), nil
}

// mergeIsNoop reports whether `git merge-tree --write-tree` output, whose first
// line is the merged tree id, equals head's tree. Pure.
func mergeIsNoop(mergeOut, headTree string) bool {
	first, _, _ := strings.Cut(mergeOut, "\n")
	first = strings.TrimSpace(first)
	return first != "" && first == strings.TrimSpace(headTree)
}

// exitCodeIs reports whether err is a git failure that exited with code.
func exitCodeIs(err error, code int) bool {
	var ee *git.ExitError
	return errors.As(err, &ee) && ee.ExitCode() == code
}
