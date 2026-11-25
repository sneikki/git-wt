package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNew_CopyKeepsCheckout: the copy step adds what the fresh checkout lacks
// and touches nothing it has. The main clone is stale and locally modified:
// its .env.example and its real .claude/skills dir must not replace the
// checkout's tracked files, nor be written through the tracked symlink.
func TestNew_CopyKeepsCheckout(t *testing.T) {
	repo := newRepo(t)
	mustWrite(t, filepath.Join(repo, ".env.example"), "committed\n")
	if err := os.MkdirAll(filepath.Join(repo, ".agents", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(repo, ".agents", "skills", "a.md"), "tracked\n")
	if err := os.MkdirAll(filepath.Join(repo, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../.agents/skills", filepath.Join(repo, ".claude", "skills")); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(repo, ".git-wt-copy-files"), ".env\n.env.example\n.claude\n")
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "config")

	// Main clone: modified tracked file, untracked .env, real dir where the
	// checkout has the symlink.
	mustWrite(t, filepath.Join(repo, ".env.example"), "stale local\n")
	mustWrite(t, filepath.Join(repo, ".env"), "secret\n")
	if err := os.Remove(filepath.Join(repo, ".claude", "skills")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(repo, ".claude", "skills", "main.md"), "main only\n")

	res := runWT(t, repo, "new", "feat-keep", "--non-interactive")
	if res.ExitCode != 0 {
		t.Fatalf("new exit %d: %s", res.ExitCode, res.Stderr)
	}
	if strings.Contains(res.Stderr, "warning: copy configs") {
		t.Errorf("unexpected copy warning: %s", res.Stderr)
	}
	wtDir := filepath.Join(repo, ".worktrees", "feat-keep")

	if got, _ := readFile(filepath.Join(wtDir, ".env")); got != "secret\n" {
		t.Errorf(".env = %q, want copied secret", got)
	}
	if got, _ := readFile(filepath.Join(wtDir, ".env.example")); got != "committed\n" {
		t.Errorf(".env.example = %q, want committed (checkout kept)", got)
	}
	mustNotExist(t, filepath.Join(wtDir, ".agents", "skills", "main.md"))

	status := mustGit(t, wtDir, "status", "--porcelain")
	for _, line := range strings.Split(strings.TrimSpace(status), "\n") {
		if line != "" && !strings.HasPrefix(line, "??") {
			t.Errorf("new worktree has changed tracked file: %q", line)
		}
	}
}
