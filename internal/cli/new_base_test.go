package cli

import "testing"

func TestPickRemote(t *testing.T) {
	cases := []struct {
		name    string
		from    string
		remotes []string
		want    string
	}{
		{"no from, origin", "", []string{"origin"}, "origin"},
		{"no from, no remotes", "", nil, ""},
		{"no from, no origin", "", []string{"upstream"}, ""},
		{"from remote prefix wins", "upstream/main", []string{"origin", "upstream"}, "upstream"},
		{"from local ref falls back to origin", "main", []string{"origin"}, "origin"},
		{"from local ref, no origin", "main", []string{"upstream"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pickRemote(c.from, c.remotes); got != c.want {
				t.Errorf("pickRemote(%q, %v) = %q, want %q", c.from, c.remotes, got, c.want)
			}
		})
	}
}

func TestDescribeBase(t *testing.T) {
	got := describeBase(newBase{Ref: "origin/main", SHA: "1a2b3c4"})
	if want := "branched from origin/main at 1a2b3c4"; got != want {
		t.Errorf("describeBase = %q, want %q", got, want)
	}
}
