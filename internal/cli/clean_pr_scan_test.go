package cli

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/shhac/git-wt/internal/wt"
)

func tipIs(sha string) tipFn { return func(string) (string, error) { return sha, nil } }

func prsAre(prs ...mergedPR) mergedPRsFn {
	return func(string) ([]mergedPR, error) { return prs, nil }
}

func baseIs(isBase bool, err error) isBaseFn {
	return func(string) (bool, error) { return isBase, err }
}

func TestMergedPRVerdict(t *testing.T) {
	good := mergedPR{HeadOid: "tip1", Base: "main"}
	cases := []struct {
		name string
		tip  tipFn
		prs  mergedPRsFn
		base isBaseFn
		want prVerdict
	}{
		{"merged into default at tip", tipIs("tip1"), prsAre(good), baseIs(false, nil), prVerdict{pick: true}},
		{"tip differs", tipIs("tip2"), prsAre(good), baseIs(false, nil), prVerdict{}},
		{"merged into another branch", tipIs("tip1"), prsAre(mergedPR{"tip1", "feat-parent"}), baseIs(false, nil), prVerdict{}},
		{"empty tip", tipIs(""), prsAre(mergedPR{"", "main"}), baseIs(false, nil), prVerdict{}},
		{"tip unresolvable", func(string) (string, error) { return "", errors.New("x") }, prsAre(good), baseIs(false, nil), prVerdict{}},
		{"gh error", tipIs("tip1"), func(string) ([]mergedPR, error) { return nil, errors.New("x") }, baseIs(false, nil), prVerdict{}},
		{"branch is a PR base", tipIs("tip1"), prsAre(good), baseIs(true, nil), prVerdict{keep: "branch is the base of a pull request"}},
		{"base check fails", tipIs("tip1"), prsAre(good), baseIs(false, errors.New("timeout")),
			prVerdict{keep: "could not check whether it is the base of a pull request: timeout"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := mergedPRVerdict(c.tip, c.prs, c.base, "main")("feat"); got != c.want {
				t.Errorf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestMergedPRVerdict_BaseAskedOnlyWithProof(t *testing.T) {
	asked := 0
	isBase := func(string) (bool, error) { asked++; return false, nil }
	mergedPRVerdict(tipIs("tip2"), prsAre(mergedPR{"tip1", "main"}), isBase, "main")("feat")
	if asked != 0 {
		t.Errorf("base asked %d times without proof", asked)
	}
}

func TestFindMergedPRWorktrees_PicksAndKeeps(t *testing.T) {
	wts := []wt.Worktree{
		{Path: "/r/a", Branch: "a"},
		{Path: "/r/b", Branch: "b"},
		{Path: "/r/c", Branch: "c"},
	}
	decide := func(b string) prVerdict {
		return map[string]prVerdict{"a": {pick: true}, "b": {keep: "why"}}[b]
	}
	got, kept := findMergedPRWorktrees(wts, prScanEnv{mainRoot: "/r"}, decide)
	if want := []taggedTarget{{wt: wts[0], reason: reasonMergedPR}}; !reflect.DeepEqual(got, want) {
		t.Errorf("picked %+v, want %+v", got, want)
	}
	if want := []keptTarget{{taggedTarget{wt: wts[1], reason: reasonMergedPR}, "why"}}; !reflect.DeepEqual(kept, want) {
		t.Errorf("kept %+v, want %+v", kept, want)
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
	decide := func(b string) prVerdict { asked = append(asked, b); return prVerdict{pick: true} }
	env := prScanEnv{mainRoot: "/r", curPath: cur.Path, defaultBranch: "trunk", skip: map[string]struct{}{"/r/dup": {}}}
	got, _ := findMergedPRWorktrees(wts, env, decide)
	if !reflect.DeepEqual(asked, []string{"ok"}) || len(got) != 1 || got[0].wt.Branch != "ok" {
		t.Errorf("asked %v, got %+v; only ok is eligible", asked, got)
	}
}

func TestPrintKept_NamesTheReasonOfTheTarget(t *testing.T) {
	var buf strings.Builder
	k := keptTarget{taggedTarget{wt: wt.Worktree{Branch: "develop"}, reason: reasonMergedPR}, "branch is the base of a pull request"}
	printKept(&buf, []keptTarget{k})
	if want := "skipping develop: merged PR at tip, branch is the base of a pull request."; !strings.Contains(buf.String(), want) {
		t.Errorf("got %q, want it to contain %q", buf.String(), want)
	}
}
