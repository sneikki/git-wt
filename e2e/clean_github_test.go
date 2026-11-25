package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitOnlyPath returns a PATH directory holding only a link to git, so gh
// cannot be found. The e2e run cannot reach GitHub; this covers the fallback.
func gitOnlyPath(t *testing.T) string {
	t.Helper()
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Symlink(gitBin, filepath.Join(dir, "git")); err != nil {
		t.Fatal(err)
	}
	return dir
}

// Without gh the GitHub proof cannot run. A branch the git proofs reject
// stays kept, and the stderr reason names the gh failure.
func TestClean_NoGhKeepsUnmergedBranch(t *testing.T) {
	repo, _ := newRepoWithRemote(t)
	wtPath := addGoneWorktree(t, repo, "feat", "squash+new")
	t.Setenv("PATH", gitOnlyPath(t))

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
