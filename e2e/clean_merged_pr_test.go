package e2e

import (
	"fmt"
	"strings"
	"testing"
)

// fakeGhPR stands in for gh on the merged-PR scan: one merged PR for branch
// at headOid into base, and a PR-base answer of "[{...}]" when isBase.
func fakeGhPR(branch, headOid, base string, isBase bool) string {
	baseOf := "[]"
	if isBase {
		baseOf = `[{"number":7}]`
	}
	return fmt.Sprintf(`#!/bin/sh
case "$*" in
"pr list --head %[1]s --state merged --json headRefOid,baseRefName --limit 100")
	printf '%%s\n' '[{"headRefOid":"%[2]s","baseRefName":"%[3]s"}]' ;;
"pr list --base %[1]s --state all --json number --limit 1")
	printf '%%s\n' '%[4]s' ;;
*)
	echo "fake gh: unexpected args: $*" >&2
	exit 2 ;;
esac
`, branch, headOid, base, baseOf)
}

// newPresentUpstreamWorktree is a worktree whose remote branch still exists
// and whose git proofs fail: only a PR can prove it.
func newPresentUpstreamWorktree(t *testing.T, ghFor func(tip string) string) (repo, origin, wtPath string) {
	t.Helper()
	repo, origin = newRepoWithRemote(t)
	wtPath = addGoneWorktree(t, repo, "feat", "none")
	mustGit(t, repo, "push", "-q", "origin", "feat")
	tip := mustGit(t, repo, "rev-parse", "refs/heads/feat")
	t.Setenv("PATH", toolPath(t, ghFor(tip)))
	return repo, origin, wtPath
}

func branchKept(t *testing.T, repo string) bool {
	t.Helper()
	return !gitFails(t, repo, "rev-parse", "--verify", "-q", "refs/heads/feat")
}

// Remote branch still there, PR merged into main at the tip: removed, and the
// remote branch is left alone.
func TestClean_MergedPRWithRemoteBranchStillThere(t *testing.T) {
	repo, origin, wtPath := newPresentUpstreamWorktree(t, func(tip string) string {
		return fakeGhPR("feat", tip, "main", false)
	})
	res := runWT(t, repo, "clean", "--non-interactive")
	if res.ExitCode != 0 {
		t.Fatalf("clean exit %d: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "merged PR at tip") {
		t.Errorf("stderr should name the proof, got: %s", res.Stderr)
	}
	mustNotExist(t, wtPath)
	if branchKept(t, repo) {
		t.Error("local branch feat should be deleted")
	}
	if gitFails(t, origin, "rev-parse", "--verify", "-q", "refs/heads/feat") {
		t.Error("remote branch feat must stay")
	}
}

// A merged PR at another commit proves nothing: commits made after the merge.
func TestClean_MergedPRAtOtherShaKeptUpstreamPresent(t *testing.T) {
	repo, _, wtPath := newPresentUpstreamWorktree(t, func(string) string {
		return fakeGhPR("feat", "0000000000000000000000000000000000000000", "main", false)
	})
	res := runWT(t, repo, "clean", "--non-interactive")
	if res.ExitCode != 0 {
		t.Fatalf("clean exit %d: %s", res.ExitCode, res.Stderr)
	}
	mustExist(t, wtPath)
	if !branchKept(t, repo) {
		t.Error("branch feat should be kept")
	}
}

// A PR merged into some other branch is not a merge into the default branch.
func TestClean_MergedPRIntoOtherBaseKept(t *testing.T) {
	repo, _, wtPath := newPresentUpstreamWorktree(t, func(tip string) string {
		return fakeGhPR("feat", tip, "parent", false)
	})
	runWT(t, repo, "clean", "--non-interactive")
	mustExist(t, wtPath)
}

// A branch other PRs target is a parent or long-lived branch: kept, reason
// printed.
func TestClean_BranchThatIsAPRBaseKept(t *testing.T) {
	repo, _, wtPath := newPresentUpstreamWorktree(t, func(tip string) string {
		return fakeGhPR("feat", tip, "main", true)
	})
	res := runWT(t, repo, "clean", "--non-interactive")
	if !strings.Contains(res.Stderr, "skipping feat") || !strings.Contains(res.Stderr, "base of a pull request") {
		t.Errorf("stderr should name the kept branch and why, got: %s", res.Stderr)
	}
	mustExist(t, wtPath)
}

// Uncommitted work keeps a worktree whatever the PR says, in the dry run's
// listing and in the real run.
func TestClean_DirtyMergedPRWorktreeKept(t *testing.T) {
	repo, _, wtPath := newPresentUpstreamWorktree(t, func(tip string) string {
		return fakeGhPR("feat", tip, "main", false)
	})
	mustWrite(t, wtPath+"/wip.txt", "unsaved\n")

	dry := runWT(t, repo, "clean", "--non-interactive", "--dry-run")
	if !strings.Contains(dry.Stderr, "dirty") {
		t.Errorf("dry run should flag the dirty worktree, got: %s", dry.Stderr)
	}
	mustExist(t, wtPath)

	res := runWT(t, repo, "clean", "--non-interactive")
	if res.ExitCode != 0 {
		t.Fatalf("clean exit %d: %s", res.ExitCode, res.Stderr)
	}
	mustExist(t, wtPath)
	if !branchKept(t, repo) {
		t.Error("dirty worktree's branch must be kept")
	}
}
