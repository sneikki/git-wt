package cli

import (
	"errors"
	"reflect"
	"testing"

	"github.com/shhac/git-wt/internal/wt"
)

func TestSplitByIntegration(t *testing.T) {
	gone := func(branch string) taggedTarget {
		return taggedTarget{wt: wt.Worktree{Branch: branch}, reason: reasonUpstreamGone}
	}
	deleted := taggedTarget{wt: wt.Worktree{Branch: "old"}, reason: "branch deleted"}
	integrated := map[string]bool{"landed": true}
	failing := map[string]error{"broken": errors.New("boom")}
	var asked []string
	fake := func(branch string) (bool, error) {
		asked = append(asked, branch)
		if err, ok := failing[branch]; ok {
			return false, err
		}
		return integrated[branch], nil
	}

	remove, kept := splitByIntegration(
		[]taggedTarget{gone("landed"), gone("lost"), gone("broken"), deleted}, fake)

	if want := []taggedTarget{gone("landed"), deleted}; !reflect.DeepEqual(remove, want) {
		t.Errorf("remove = %+v, want %+v", remove, want)
	}
	wantKept := []keptTarget{
		{gone("lost"), "not merged into origin/HEAD"},
		{gone("broken"), "integration check failed: boom"},
	}
	if !reflect.DeepEqual(kept, wantKept) {
		t.Errorf("kept = %+v, want %+v", kept, wantKept)
	}
	if want := []string{"landed", "lost", "broken"}; !reflect.DeepEqual(asked, want) {
		t.Errorf("integration asked for %v, want %v (branch deleted must not be checked)", asked, want)
	}
}

func TestMergeIsNoop(t *testing.T) {
	const head = "abc123"
	cases := []struct {
		name, out string
		want      bool
	}{
		{"tree equals head", "abc123\n", true},
		{"tree differs", "def456\n", false},
		{"empty output", "", false},
		{"conflict listing after tree", "def456\nCONFLICT (content)\n", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := mergeIsNoop(c.out, head); got != c.want {
				t.Errorf("mergeIsNoop(%q) = %v, want %v", c.out, got, c.want)
			}
		})
	}
}
