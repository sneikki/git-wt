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

// dirtyEjectRepo puts a branch with a modified tracked file and an untracked
// file in the main tree, ready to eject.
func dirtyEjectRepo(t *testing.T, branch string) string {
	t.Helper()
	repo := newRepo(t)
	mustGit(t, repo, "checkout", "-q", "-b", branch)
	mustWrite(t, filepath.Join(repo, "README"), "v2\n")
	mustWrite(t, filepath.Join(repo, "u.txt"), "user untracked\n")
	return repo
}

// TestPostCreate_EjectHookRunsAfterStashRestore: the hook writes the same
// dirty and untracked files the user has. It must see them restored first,
// so its appends land after the user's content and nothing stays stashed.
func TestPostCreate_EjectHookRunsAfterStashRestore(t *testing.T) {
	repo := dirtyEjectRepo(t, "feat-eject-order")
	setPostCreate(t, repo, `echo hook >> README; echo hook >> u.txt`)

	res := runWT(t, repo, "eject", "--non-interactive")
	if res.ExitCode != 0 {
		t.Fatalf("eject exit %d: %s", res.ExitCode, res.Stderr)
	}
	wtDir := filepath.Join(repo, ".worktrees", "feat-eject-order")
	assertFile(t, filepath.Join(wtDir, "README"), "v2\nhook\n")
	assertFile(t, filepath.Join(wtDir, "u.txt"), "user untracked\nhook\n")
	if got := mustGit(t, repo, "stash", "list"); got != "" {
		t.Errorf("stash not drained, still holds: %q", got)
	}
}

// TestPostCreate_EjectFailingHookKeepsChanges: a failing hook after a dirty
// eject leaves the user's changes in the new worktree, exits 0 and warns.
func TestPostCreate_EjectFailingHookKeepsChanges(t *testing.T) {
	repo := dirtyEjectRepo(t, "feat-eject-fail")
	setPostCreate(t, repo, `exit 3`)

	res := runWT(t, repo, "eject", "--non-interactive")
	if res.ExitCode != 0 {
		t.Fatalf("eject exit %d, want 0: %s", res.ExitCode, res.Stderr)
	}
	wtDir := filepath.Join(repo, ".worktrees", "feat-eject-fail")
	assertFile(t, filepath.Join(wtDir, "README"), "v2\n")
	assertFile(t, filepath.Join(wtDir, "u.txt"), "user untracked\n")
	if !strings.Contains(res.Stderr, "warning") || !strings.Contains(res.Stderr, "exit status 3") {
		t.Errorf("stderr lacks warning with exit status: %q", res.Stderr)
	}
}

// assertFile fails unless the file at path holds exactly want.
func assertFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := readFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if got != want {
		t.Errorf("%s = %q, want %q", filepath.Base(path), got, want)
	}
}
