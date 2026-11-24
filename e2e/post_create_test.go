package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

// setPostCreate stores command as wt.postCreate in the repo's local config.
func setPostCreate(t *testing.T, repo, command string) {
	t.Helper()
	if res := runWT(t, repo, "config", "postCreate", command); res.ExitCode != 0 {
		t.Fatalf("config set postCreate: exit %d: %s", res.ExitCode, res.Stderr)
	}
}

// samePath reports whether two paths name the same directory, after
// resolving symlinks (macOS temp dirs sit behind one).
func samePath(t *testing.T, got, want string) bool {
	t.Helper()
	g, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatalf("resolve %q: %v", got, err)
	}
	w, err := filepath.EvalSymlinks(want)
	if err != nil {
		t.Fatalf("resolve %q: %v", want, err)
	}
	return g == w
}

// TestPostCreate_RunsInWorktreeWithEnv: the command runs with the new
// worktree as cwd and sees both GIT_WT_* variables.
func TestPostCreate_RunsInWorktreeWithEnv(t *testing.T) {
	repo := newRepo(t)
	setPostCreate(t, repo, `printf '%s\n' "$(pwd -P)" "$GIT_WT_MAIN_ROOT" "$GIT_WT_WORKTREE" > post-create.txt`)
	mustGit(t, repo, "branch", "feat-hook")

	res := runWT(t, repo, "add", "feat-hook", "--non-interactive", "--no-copy")
	if res.ExitCode != 0 {
		t.Fatalf("add exit %d: %s", res.ExitCode, res.Stderr)
	}
	wtDir := filepath.Join(repo, ".worktrees", "feat-hook")
	raw, err := readFile(filepath.Join(wtDir, "post-create.txt"))
	if err != nil {
		t.Fatalf("hook marker not written: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) != 3 {
		t.Fatalf("marker = %q, want 3 lines", raw)
	}
	if !samePath(t, lines[0], wtDir) {
		t.Errorf("cwd = %q, want %q", lines[0], wtDir)
	}
	if !samePath(t, lines[1], repo) {
		t.Errorf("GIT_WT_MAIN_ROOT = %q, want %q", lines[1], repo)
	}
	if !samePath(t, lines[2], wtDir) {
		t.Errorf("GIT_WT_WORKTREE = %q, want %q", lines[2], wtDir)
	}
}

// TestPostCreate_StdoutIsOnlyThePath: in bare mode the hook's stdout and
// stderr go to stderr, so stdout holds nothing but the path.
func TestPostCreate_StdoutIsOnlyThePath(t *testing.T) {
	repo := newRepo(t)
	setPostCreate(t, repo, `echo HOOK-OUT; echo HOOK-ERR >&2`)
	mustGit(t, repo, "branch", "feat-quiet")

	res := runWT(t, repo, "add", "feat-quiet", "--non-interactive", "--no-copy")
	if res.ExitCode != 0 {
		t.Fatalf("add exit %d: %s", res.ExitCode, res.Stderr)
	}
	wtDir := filepath.Join(repo, ".worktrees", "feat-quiet")
	if got := strings.TrimSpace(res.Stdout); !samePath(t, got, wtDir) || strings.Count(res.Stdout, "\n") != 1 {
		t.Errorf("stdout = %q, want only the worktree path", res.Stdout)
	}
	if !strings.Contains(res.Stderr, "HOOK-OUT") || !strings.Contains(res.Stderr, "HOOK-ERR") {
		t.Errorf("hook output missing from stderr: %q", res.Stderr)
	}
}

// TestPostCreate_FailureIsWarningNotFatal: a failing command leaves the
// worktree in place, still emits the path, and exits 0 with a warning.
func TestPostCreate_FailureIsWarningNotFatal(t *testing.T) {
	repo := newRepo(t)
	setPostCreate(t, repo, `exit 3`)
	mustGit(t, repo, "branch", "feat-fail")

	res := runWT(t, repo, "add", "feat-fail", "--non-interactive", "--no-copy")
	if res.ExitCode != 0 {
		t.Fatalf("add exit %d, want 0 (hook failure must not be fatal): %s", res.ExitCode, res.Stderr)
	}
	wtDir := filepath.Join(repo, ".worktrees", "feat-fail")
	mustExist(t, wtDir)
	if got := strings.TrimSpace(res.Stdout); !samePath(t, got, wtDir) {
		t.Errorf("stdout = %q, want the worktree path", res.Stdout)
	}
	if !strings.Contains(res.Stderr, "warning") || !strings.Contains(res.Stderr, "exit status 3") {
		t.Errorf("stderr lacks warning with exit status: %q", res.Stderr)
	}
}

// TestPostCreate_UnsetRunsNothing: once the key is removed, nothing runs.
func TestPostCreate_UnsetRunsNothing(t *testing.T) {
	repo := newRepo(t)
	setPostCreate(t, repo, `touch ran.marker`)
	if res := runWT(t, repo, "config", "--unset", "postCreate"); res.ExitCode != 0 {
		t.Fatalf("config --unset: exit %d: %s", res.ExitCode, res.Stderr)
	}
	mustGit(t, repo, "branch", "feat-unset")

	res := runWT(t, repo, "add", "feat-unset", "--non-interactive", "--no-copy")
	if res.ExitCode != 0 {
		t.Fatalf("add exit %d: %s", res.ExitCode, res.Stderr)
	}
	mustNotExist(t, filepath.Join(repo, ".worktrees", "feat-unset", "ran.marker"))
	if strings.Contains(res.Stderr, "warning") {
		t.Errorf("unexpected warning with key unset: %q", res.Stderr)
	}
}

// TestPostCreate_WrapperFDStaysOffHook: a hook that writes to fd 3 must not
// reach the wrapper channel, which carries only the path.
func TestPostCreate_WrapperFDStaysOffHook(t *testing.T) {
	repo := newRepo(t)
	setPostCreate(t, repo, `echo LEAK >&3 2>/dev/null; true`)
	mustGit(t, repo, "branch", "feat-fd3")

	res := runWTFD(t, repo, "add", "feat-fd3", "--non-interactive", "--no-copy")
	if res.ExitCode != 0 {
		t.Fatalf("add exit %d: %s", res.ExitCode, res.Stderr)
	}
	if strings.Contains(res.FD3, "LEAK") {
		t.Errorf("hook wrote to wrapper fd: %q", res.FD3)
	}
	if !strings.HasSuffix(strings.TrimSpace(res.FD3), "/.worktrees/feat-fd3") || strings.Count(res.FD3, "\n") != 1 {
		t.Errorf("fd3 = %q, want only the path", res.FD3)
	}
}

// TestPostCreate_RunsOnEject: eject builds its worktree outside
// finalizeWorktreeSite, so it needs the hook too.
func TestPostCreate_RunsOnEject(t *testing.T) {
	repo := newRepo(t)
	setPostCreate(t, repo, `touch ran.marker`)
	mustGit(t, repo, "checkout", "-q", "-b", "feat-eject-hook")

	res := runWT(t, repo, "eject", "--non-interactive")
	if res.ExitCode != 0 {
		t.Fatalf("eject exit %d: %s", res.ExitCode, res.Stderr)
	}
	mustExist(t, filepath.Join(repo, ".worktrees", "feat-eject-hook", "ran.marker"))
}
