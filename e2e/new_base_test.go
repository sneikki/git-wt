package e2e

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// cloneOfOrigin returns a clone of a bare origin whose main has one commit,
// plus a seed repo that pushes further commits to origin. The clone gets
// origin/HEAD from clone, as a real checkout does.
func cloneOfOrigin(t *testing.T) (clone, seed string) {
	t.Helper()
	base := t.TempDir()
	origin := filepath.Join(base, "origin.git")
	seed = filepath.Join(base, "seed")
	clone = filepath.Join(base, "clone")
	mustGit(t, "", "init", "-q", "--bare", "-b", "main", origin)
	mustGit(t, "", "init", "-q", "-b", "main", seed)
	commitFile(t, seed, "README", "init\n", "init")
	mustGit(t, seed, "remote", "add", "origin", origin)
	mustGit(t, seed, "push", "-q", "origin", "main")
	mustGit(t, "", "clone", "-q", origin, clone)
	return clone, seed
}

// commitFile writes name in repo, commits it and returns the new HEAD sha.
func commitFile(t *testing.T, repo, name, content, msg string) string {
	t.Helper()
	mustWrite(t, filepath.Join(repo, name), content)
	mustGit(t, repo, "add", name)
	mustGit(t, repo, "commit", "-q", "-m", msg)
	return mustGit(t, repo, "rev-parse", "HEAD")
}

// advanceOrigin pushes one commit to origin main from seed and returns its sha.
func advanceOrigin(t *testing.T, seed string) string {
	t.Helper()
	tip := commitFile(t, seed, "NEXT", "next\n", "advance")
	mustGit(t, seed, "push", "-q", "origin", "main")
	return tip
}

// gitFails reports whether git args exit non-zero in dir.
func gitFails(t *testing.T, dir string, args ...string) bool {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = hermeticEnv()
	return cmd.Run() != nil
}

func TestNew_BranchesFromFreshOriginTip(t *testing.T) {
	clone, seed := cloneOfOrigin(t)
	tip := advanceOrigin(t, seed)

	res := runWT(t, clone, "new", "feat", "--non-interactive", "--no-copy")
	if res.ExitCode != 0 {
		t.Fatalf("new exit %d, stderr: %s", res.ExitCode, res.Stderr)
	}
	wtPath := filepath.Join(clone, ".worktrees", "feat")
	if got := mustGit(t, wtPath, "rev-parse", "HEAD"); got != tip {
		t.Errorf("worktree HEAD = %s, want origin tip %s", got, tip)
	}
	if want := "branched from origin/main at " + tip[:7]; !strings.Contains(res.Stderr, want) {
		t.Errorf("stderr should contain %q, got: %s", want, res.Stderr)
	}
}

func TestNew_BranchHasNoUpstream(t *testing.T) {
	clone, _ := cloneOfOrigin(t)
	if res := runWT(t, clone, "new", "feat", "--non-interactive", "--no-copy"); res.ExitCode != 0 {
		t.Fatalf("new exit %d, stderr: %s", res.ExitCode, res.Stderr)
	}
	mustGit(t, clone, "rev-parse", "--verify", "feat")
	if !gitFails(t, clone, "rev-parse", "--verify", "-q", "feat@{upstream}") {
		t.Error("feat has an upstream; a bare push would aim at origin/main")
	}
}

func TestNew_NoFetchUsesStaleTip(t *testing.T) {
	clone, seed := cloneOfOrigin(t)
	stale := mustGit(t, clone, "rev-parse", "HEAD")
	advanceOrigin(t, seed)

	res := runWT(t, clone, "new", "feat", "--no-fetch", "--non-interactive", "--no-copy")
	if res.ExitCode != 0 {
		t.Fatalf("new exit %d, stderr: %s", res.ExitCode, res.Stderr)
	}
	if got := mustGit(t, filepath.Join(clone, ".worktrees", "feat"), "rev-parse", "HEAD"); got != stale {
		t.Errorf("worktree HEAD = %s, want stale tip %s", got, stale)
	}
}

func TestNew_NoRemoteBranchesFromHEAD(t *testing.T) {
	repo := newRepo(t)
	head := mustGit(t, repo, "rev-parse", "HEAD")

	res := runWT(t, repo, "new", "feat", "--non-interactive", "--no-copy")
	if res.ExitCode != 0 {
		t.Fatalf("new exit %d, stderr: %s", res.ExitCode, res.Stderr)
	}
	if got := mustGit(t, filepath.Join(repo, ".worktrees", "feat"), "rev-parse", "HEAD"); got != head {
		t.Errorf("worktree HEAD = %s, want HEAD %s", got, head)
	}
}

func TestNew_FromLocalRefHonoured(t *testing.T) {
	clone, seed := cloneOfOrigin(t)
	mustGit(t, clone, "checkout", "-q", "-b", "side")
	side := commitFile(t, clone, "SIDE", "side\n", "side")
	mustGit(t, clone, "checkout", "-q", "main")
	advanceOrigin(t, seed)

	res := runWT(t, clone, "new", "feat", "--from", "side", "--non-interactive", "--no-copy")
	if res.ExitCode != 0 {
		t.Fatalf("new exit %d, stderr: %s", res.ExitCode, res.Stderr)
	}
	if got := mustGit(t, filepath.Join(clone, ".worktrees", "feat"), "rev-parse", "HEAD"); got != side {
		t.Errorf("worktree HEAD = %s, want --from side %s", got, side)
	}
}

func TestNew_UnreachableOriginFailsNamingNoFetch(t *testing.T) {
	clone, _ := cloneOfOrigin(t)
	mustGit(t, clone, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone.git"))

	res := runWT(t, clone, "new", "feat", "--non-interactive", "--no-copy")
	if res.ExitCode == 0 {
		t.Fatal("new succeeded with an unreachable origin")
	}
	if !strings.Contains(res.Stderr, "--no-fetch") {
		t.Errorf("stderr should name --no-fetch, got: %s", res.Stderr)
	}
	mustNotExist(t, filepath.Join(clone, ".worktrees", "feat"))
	if !gitFails(t, clone, "rev-parse", "--verify", "-q", "feat") {
		t.Error("branch feat was created despite the failed fetch")
	}
}

func TestNew_NoFetchUnsetOriginHeadFailsWithoutNetwork(t *testing.T) {
	clone, _ := cloneOfOrigin(t)
	mustGit(t, clone, "remote", "set-head", "origin", "-d")
	mustGit(t, clone, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone.git"))

	res := runWT(t, clone, "new", "feat", "--no-fetch", "--non-interactive", "--no-copy")
	if res.ExitCode == 0 {
		t.Fatal("new succeeded with origin/HEAD unset under --no-fetch")
	}
	if !strings.Contains(res.Stderr, "origin/HEAD is not set") {
		t.Errorf("stderr should say origin/HEAD is not set (no set-head attempt), got: %s", res.Stderr)
	}
	if !gitFails(t, clone, "rev-parse", "--verify", "-q", "feat") {
		t.Error("branch feat was created")
	}
}

func TestNew_FollowsRenamedOriginDefault(t *testing.T) {
	clone, seed := cloneOfOrigin(t)
	origin := filepath.Join(filepath.Dir(clone), "origin.git")
	tip := commitFile(t, seed, "NEXT", "next\n", "advance")
	mustGit(t, seed, "push", "-q", "origin", "main:trunk")
	mustGit(t, origin, "symbolic-ref", "HEAD", "refs/heads/trunk")
	mustGit(t, seed, "push", "-q", "origin", "--delete", "main")

	res := runWT(t, clone, "new", "feat", "--non-interactive", "--no-copy")
	if res.ExitCode != 0 {
		t.Fatalf("new exit %d, stderr: %s", res.ExitCode, res.Stderr)
	}
	if got := mustGit(t, filepath.Join(clone, ".worktrees", "feat"), "rev-parse", "HEAD"); got != tip {
		t.Errorf("worktree HEAD = %s, want renamed default tip %s", got, tip)
	}
	if !strings.Contains(res.Stderr, "branched from origin/trunk at "+tip[:7]) {
		t.Errorf("stderr should name origin/trunk, got: %s", res.Stderr)
	}
}

func TestNew_SetsOriginHeadWhenUnset(t *testing.T) {
	clone, _ := cloneOfOrigin(t)
	mustGit(t, clone, "remote", "set-head", "origin", "-d")

	if res := runWT(t, clone, "new", "feat", "--non-interactive", "--no-copy"); res.ExitCode != 0 {
		t.Fatalf("new exit %d, stderr: %s", res.ExitCode, res.Stderr)
	}
	if got := mustGit(t, clone, "symbolic-ref", "-q", "--short", "refs/remotes/origin/HEAD"); got != "origin/main" {
		t.Errorf("origin/HEAD = %q, want origin/main", got)
	}
}
