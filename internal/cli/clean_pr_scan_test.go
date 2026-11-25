package cli

import (
	"errors"
	"reflect"
	"testing"

	"github.com/shhac/git-wt/internal/wt"
)

// Upstream state (gone, present, tracking another branch, none) is not an
// input of the scan: a merged head at the local tip decides, so each state
// is the same worktree, which clean must pick.
func TestFindMergedPRWorktrees_AnyUpstreamState(t *testing.T) {
	for _, state := range []string{"upstream gone", "upstream present", "tracks main", "no upstream"} {
		t.Run(state, func(t *testing.T) {
			w := wt.Worktree{Path: "/r/.worktrees/feat", Branch: "feat"}
			proof := func(string) (bool, error) { return true, nil }
			got := findMergedPRWorktrees([]wt.Worktree{w}, prScanEnv{mainRoot: "/r"}, proof)
			want := []taggedTarget{{wt: w, reason: reasonMergedPR}}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("got %+v, want %+v", got, want)
			}
		})
	}
}

func TestFindMergedPRWorktrees_TipDiffersKept(t *testing.T) {
	w := wt.Worktree{Path: "/r/w", Branch: "feat"}
	proof := func(string) (bool, error) { return false, nil }
	if got := findMergedPRWorktrees([]wt.Worktree{w}, prScanEnv{mainRoot: "/r"}, proof); len(got) != 0 {
		t.Errorf("tip differs from merged head, got %+v", got)
	}
}

func TestFindMergedPRWorktrees_ProofErrorIsNoProof(t *testing.T) {
	w := wt.Worktree{Path: "/r/w", Branch: "feat"}
	proof := func(string) (bool, error) { return false, errors.New("gh missing") }
	if got := findMergedPRWorktrees([]wt.Worktree{w}, prScanEnv{mainRoot: "/r"}, proof); len(got) != 0 {
		t.Errorf("gh failure must not pick, got %+v", got)
	}
}

func TestFindMergedPRWorktrees_NeverAsksForIneligible(t *testing.T) {
	cur := wt.Worktree{Path: "/r/cur", Branch: "cur"}
	wts := []wt.Worktree{
		{Path: "/r", Branch: "main"},
		cur,
		{Path: "/r/def", Branch: "trunk"},
		{Path: "/r/lock", Branch: "lock", Locked: true},
		{Path: "/r/det", Detached: true},
		{Path: "/r/pru", Branch: "pru", Prunable: true},
		{Path: "/r/dup", Branch: "dup"},
		{Path: "/r/ok", Branch: "ok"},
	}
	var asked []string
	proof := func(b string) (bool, error) { asked = append(asked, b); return true, nil }
	env := prScanEnv{mainRoot: "/r", curPath: cur.Path, defaultBranch: "trunk", skip: map[string]struct{}{"/r/dup": {}}}
	got := findMergedPRWorktrees(wts, env, proof)
	if !reflect.DeepEqual(asked, []string{"ok"}) || len(got) != 1 || got[0].wt.Branch != "ok" {
		t.Errorf("asked %v, got %+v; only ok is eligible", asked, got)
	}
}
