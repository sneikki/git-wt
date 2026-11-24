package cli

import (
	"slices"
	"testing"
)

func TestWithPostCreateEnv_SetsBothVarsOnce(t *testing.T) {
	base := []string{"PATH=/bin", "GIT_WT_MAIN_ROOT=/old/main", "GIT_WT_WORKTREE=/old/wt"}
	got := withPostCreateEnv(base, "/repo", "/repo/.worktrees/x")

	want := []string{"PATH=/bin", "GIT_WT_MAIN_ROOT=/repo", "GIT_WT_WORKTREE=/repo/.worktrees/x"}
	if !slices.Equal(got, want) {
		t.Errorf("env = %q, want %q", got, want)
	}
}

// The input carries both names, so an in-place filter would change it.
func TestWithPostCreateEnv_DoesNotMutateInput(t *testing.T) {
	base := []string{"GIT_WT_MAIN_ROOT=/old/main", "PATH=/bin", "GIT_WT_WORKTREE=/old/wt"}
	before := slices.Clone(base)
	_ = withPostCreateEnv(base, "/repo", "/wt")
	if !slices.Equal(base, before) {
		t.Errorf("input changed: %q, was %q", base, before)
	}
}
