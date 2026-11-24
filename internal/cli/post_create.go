package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/shhac/git-wt/internal/config"
	"github.com/shhac/git-wt/internal/debug"
	"github.com/shhac/git-wt/internal/fd"
)

// Environment names handed to the wt.postCreate command.
const (
	envMainRoot = "GIT_WT_MAIN_ROOT"
	envWorktree = "GIT_WT_WORKTREE"
)

// runPostCreateHook runs the wt.postCreate command, if one is set, with the
// new worktree as its working directory. The command comes from git config
// only, never from a tracked file, so a cloned repo cannot make git-wt run
// anything. Its output goes to stderr, because stdout and the wrapper fd
// carry the destination path. A failure is a warning: the worktree stays and
// the caller still emits the path.
func runPostCreateHook(ctx context.Context, mainRoot, worktree string) {
	e, err := config.GetEffective(ctx, config.PostCreate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: read %s: %v\n", config.PostCreate.FullName(), err)
		return
	}
	if !e.IsSet || e.Value == "" {
		return
	}
	end := debug.Op("post-create", e.Value)
	err = runPostCreate(ctx, e.Value, mainRoot, worktree)
	end(err)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %s %q failed: %v\n", config.PostCreate.FullName(), e.Value, err)
	}
}

// runPostCreate executes command with `sh -c` in worktree. stdin is
// /dev/null, and stdout and stderr both go to git-wt's stderr.
func runPostCreate(ctx context.Context, command, mainRoot, worktree string) error {
	// Keep the wrapper fd out of the child: a shell redirect or a daemon the
	// command starts must not reach the path channel.
	fd.CloseOnExec(flagFD)
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = worktree
	cmd.Env = withPostCreateEnv(os.Environ(), mainRoot, worktree)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// withPostCreateEnv returns environ with GIT_WT_MAIN_ROOT and GIT_WT_WORKTREE
// set. Inherited values of either name are dropped first, so a git-wt run
// nested inside another hook sees its own paths.
func withPostCreateEnv(environ []string, mainRoot, worktree string) []string {
	out := make([]string, 0, len(environ)+2)
	for _, kv := range environ {
		if strings.HasPrefix(kv, envMainRoot+"=") || strings.HasPrefix(kv, envWorktree+"=") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, envMainRoot+"="+mainRoot, envWorktree+"="+worktree)
}
