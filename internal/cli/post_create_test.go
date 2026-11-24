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

func TestWithPostCreateEnv_DoesNotTouchInput(t *testing.T) {
	base := []string{"PATH=/bin"}
	_ = withPostCreateEnv(base, "/repo", "/wt")
	if len(base) != 1 {
		t.Errorf("input slice grew to %d entries", len(base))
	}
}
