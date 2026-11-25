package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// toolPath returns a fresh PATH directory holding a link to git and, when
// ghScript is not empty, an executable gh with that body. Nothing else is on
// PATH, so gh is absent unless the test provides it.
func toolPath(t *testing.T, ghScript string) string {
	t.Helper()
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Symlink(gitBin, filepath.Join(dir, "git")); err != nil {
		t.Fatal(err)
	}
	if ghScript != "" {
		if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(ghScript), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// fakeGh is a gh stand-in for `gh pr list --head feat --state merged --json
// headRefOid --limit 100`. It answers with one merged PR at headOid, and
// fails loudly on any other argument list, so a changed call breaks the test.
func fakeGh(branch, headOid string) string {
	return fmt.Sprintf(`#!/bin/sh
want='pr list --head %s --state merged --json headRefOid --limit 100'
if [ "$*" != "$want" ]; then
	echo "fake gh: unexpected args: $*" >&2
	exit 2
fi
printf '%%s\n' '[{"headRefOid":"%s"}]'
`, branch, headOid)
}

// Without gh the GitHub proof cannot run. A branch the git proofs reject
// stays kept, and the stderr reason names the gh failure.
func TestClean_NoGhKeepsUnmergedBranch(t *testing.T) {
	repo, _ := newRepoWithRemote(t)
	wtPath := addGoneWorktree(t, repo, "feat", "squash+new")
	t.Setenv("PATH", toolPath(t, ""))

	res := runWT(t, repo, "clean", "--non-interactive", "--upstream-gone-only")
	if res.ExitCode != 0 {
		t.Fatalf("clean exit %d: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "skipping feat") || !strings.Contains(res.Stderr, "gh pr list") {
		t.Errorf("stderr should name the kept branch and the gh failure, got: %s", res.Stderr)
	}
	mustExist(t, wtPath)
	if gitFails(t, repo, "rev-parse", "--verify", "-q", "refs/heads/feat") {
		t.Error("branch feat should be kept without gh")
	}
}

// A merged PR at the branch tip is the third proof: git proofs fail (the
// branch is unmerged), gh reports the PR at the same commit, so it is removed.
func TestClean_GhMergedPRAtTipRemoves(t *testing.T) {
	repo, _ := newRepoWithRemote(t)
	wtPath := addGoneWorktree(t, repo, "feat", "none")
	tip := mustGit(t, repo, "rev-parse", "refs/heads/feat")
	t.Setenv("PATH", toolPath(t, fakeGh("feat", tip)))

	res := runWT(t, repo, "clean", "--non-interactive", "--upstream-gone-only")
	if res.ExitCode != 0 {
		t.Fatalf("clean exit %d: %s", res.ExitCode, res.Stderr)
	}
	mustNotExist(t, wtPath)
	if !gitFails(t, repo, "rev-parse", "--verify", "-q", "refs/heads/feat") {
		t.Error("branch feat should be deleted")
	}
}

// A merged PR at some other commit does not prove this tip: kept, and the
// stderr names the branch.
func TestClean_GhMergedPRAtOtherShaKeeps(t *testing.T) {
	repo, _ := newRepoWithRemote(t)
	wtPath := addGoneWorktree(t, repo, "feat", "none")
	other := mustGit(t, repo, "rev-parse", "refs/heads/main")
	t.Setenv("PATH", toolPath(t, fakeGh("feat", other)))

	res := runWT(t, repo, "clean", "--non-interactive", "--upstream-gone-only")
	if res.ExitCode != 0 {
		t.Fatalf("clean exit %d: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "skipping feat") {
		t.Errorf("stderr should name the kept branch, got: %s", res.Stderr)
	}
	mustExist(t, wtPath)
	if gitFails(t, repo, "rev-parse", "--verify", "-q", "refs/heads/feat") {
		t.Error("branch feat should be kept when the merged PR is at another commit")
	}
}

// Without origin/HEAD the default branch is unknown, so the merged-PR scan
// cannot tell main from a feature branch: a non-root worktree on main with a
// merged PR at its tip is kept, and clean says why.
func TestClean_UnknownDefaultBranchSkipsPRScan(t *testing.T) {
	repo, _ := newRepoWithRemote(t)
	mustGit(t, repo, "checkout", "-q", "-b", "other")
	wtPath := filepath.Join(repo, ".worktrees", "main")
	mustGit(t, repo, "worktree", "add", "-q", wtPath, "main")
	mustGit(t, repo, "remote", "set-head", "origin", "-d")
	tip := mustGit(t, repo, "rev-parse", "refs/heads/main")
	t.Setenv("PATH", toolPath(t, fakeGh("main", tip)))

	res := runWT(t, repo, "clean", "--non-interactive")
	if res.ExitCode != 0 {
		t.Fatalf("clean exit %d: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "default branch unknown") {
		t.Errorf("stderr should say why the scan was skipped, got: %s", res.Stderr)
	}
	mustExist(t, wtPath)
}
