package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

// addGoneWorktree creates branch with its own worktree and a commit, pushes
// it, then brings it to the state named by how, and deletes the remote branch
// so its upstream is gone. Returns the worktree path.
//
//	squash      squash-merged into main
//	merge       merged into main with a merge commit
//	none        not merged
//	squash+new  squash-merged, then one more local commit in the worktree
func addGoneWorktree(t *testing.T, repo, branch, how string) string {
	t.Helper()
	mustGit(t, repo, "checkout", "-q", "-b", branch)
	mustWrite(t, filepath.Join(repo, branch+".txt"), branch+"\n")
	mustGit(t, repo, "add", branch+".txt")
	mustGit(t, repo, "commit", "-q", "-m", branch)
	mustGit(t, repo, "push", "-q", "-u", "origin", branch)
	mustGit(t, repo, "checkout", "-q", "main")

	wtPath := filepath.Join(repo, ".worktrees", branch)
	mustGit(t, repo, "worktree", "add", "-q", wtPath, branch)

	switch how {
	case "squash", "squash+new":
		mustGit(t, repo, "merge", "-q", "--squash", branch)
		mustGit(t, repo, "commit", "-q", "-m", "squash "+branch)
		mustGit(t, repo, "push", "-q", "origin", "main")
	case "merge":
		mustGit(t, repo, "merge", "-q", "--no-ff", "-m", "merge "+branch, branch)
		mustGit(t, repo, "push", "-q", "origin", "main")
	case "none":
	default:
		t.Fatalf("addGoneWorktree: unknown how %q", how)
	}
	if how == "squash+new" {
		mustWrite(t, filepath.Join(wtPath, "extra.txt"), "after the merge\n")
		mustGit(t, wtPath, "add", "extra.txt")
		mustGit(t, wtPath, "commit", "-q", "-m", "more work")
	}
	mustGit(t, repo, "push", "-q", "origin", "--delete", branch)
	return wtPath
}

func TestClean_SquashMergedRemoved(t *testing.T) {
	repo, _ := newRepoWithRemote(t)
	wtPath := addGoneWorktree(t, repo, "feat", "squash")

	res := runWT(t, repo, "clean", "--non-interactive", "--upstream-gone-only")
	if res.ExitCode != 0 {
		t.Fatalf("clean exit %d: %s", res.ExitCode, res.Stderr)
	}
	mustNotExist(t, wtPath)
	if !gitFails(t, repo, "rev-parse", "--verify", "-q", "refs/heads/feat") {
		t.Error("branch feat should be deleted")
	}
}

func TestClean_MergeCommitRemoved(t *testing.T) {
	repo, _ := newRepoWithRemote(t)
	wtPath := addGoneWorktree(t, repo, "feat", "merge")

	res := runWT(t, repo, "clean", "--non-interactive", "--upstream-gone-only")
	if res.ExitCode != 0 {
		t.Fatalf("clean exit %d: %s", res.ExitCode, res.Stderr)
	}
	mustNotExist(t, wtPath)
	if !gitFails(t, repo, "rev-parse", "--verify", "-q", "refs/heads/feat") {
		t.Error("branch feat should be deleted")
	}
}

func TestClean_UnmergedKept(t *testing.T) {
	repo, _ := newRepoWithRemote(t)
	wtPath := addGoneWorktree(t, repo, "feat", "none")

	res := runWT(t, repo, "clean", "--non-interactive", "--upstream-gone-only")
	if res.ExitCode != 0 {
		t.Fatalf("clean exit %d: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "skipping feat") {
		t.Errorf("stderr should name the kept branch, got: %s", res.Stderr)
	}
	mustExist(t, wtPath)
	if gitFails(t, repo, "rev-parse", "--verify", "-q", "refs/heads/feat") {
		t.Error("branch feat should be kept")
	}
}

func TestClean_SquashMergedWithNewCommitKept(t *testing.T) {
	repo, _ := newRepoWithRemote(t)
	wtPath := addGoneWorktree(t, repo, "feat", "squash+new")

	res := runWT(t, repo, "clean", "--non-interactive", "--upstream-gone-only")
	if res.ExitCode != 0 {
		t.Fatalf("clean exit %d: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "skipping feat") {
		t.Errorf("stderr should name the kept branch, got: %s", res.Stderr)
	}
	mustExist(t, wtPath)
	if gitFails(t, repo, "rev-parse", "--verify", "-q", "refs/heads/feat") {
		t.Error("branch feat should be kept")
	}
}

func TestClean_DryRunMarksKeptRemovesNothing(t *testing.T) {
	repo, _ := newRepoWithRemote(t)
	landedPath := addGoneWorktree(t, repo, "landed", "squash")
	lostPath := addGoneWorktree(t, repo, "lost", "none")

	res := runWT(t, repo, "clean", "--non-interactive", "--dry-run", "--upstream-gone-only")
	if res.ExitCode != 0 {
		t.Fatalf("clean exit %d: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "skipping lost") {
		t.Errorf("dry run should mark lost as kept, got: %s", res.Stderr)
	}
	if strings.Contains(res.Stderr, "skipping landed") {
		t.Errorf("integrated branch must not be marked kept, got: %s", res.Stderr)
	}
	mustExist(t, landedPath)
	mustExist(t, lostPath)
	if gitFails(t, repo, "rev-parse", "--verify", "-q", "refs/heads/landed") ||
		gitFails(t, repo, "rev-parse", "--verify", "-q", "refs/heads/lost") {
		t.Error("dry run must not delete any branch")
	}
}

func TestClean_OriginHeadUnresolvableKeeps(t *testing.T) {
	repo, _ := newRepoWithRemote(t)
	wtPath := addGoneWorktree(t, repo, "feat", "squash")
	mustGit(t, repo, "remote", "set-head", "origin", "-d")

	res := runWT(t, repo, "clean", "--non-interactive", "--upstream-gone-only")
	if res.ExitCode != 0 {
		t.Fatalf("clean exit %d: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "origin/HEAD unresolvable") {
		t.Errorf("stderr should give the origin/HEAD reason, got: %s", res.Stderr)
	}
	mustExist(t, wtPath)
	if gitFails(t, repo, "rev-parse", "--verify", "-q", "refs/heads/feat") {
		t.Error("branch feat should be kept when origin/HEAD does not resolve")
	}
}
