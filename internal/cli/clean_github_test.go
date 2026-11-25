package cli

import (
	"errors"
	"reflect"
	"testing"
)

// countingProof returns a proof with a fixed answer that counts its calls.
func countingProof(ok bool, err error, calls *int) integrationFn {
	return func(string) (bool, error) {
		*calls++
		return ok, err
	}
}

func TestIntegratedBy_GitProofWinsWithoutSeam(t *testing.T) {
	var gitCalls, prCalls int
	got, err := integratedBy(countingProof(true, nil, &gitCalls), countingProof(false, nil, &prCalls))("feat")
	if err != nil || !got {
		t.Errorf("git yes: got (%v, %v), want (true, nil)", got, err)
	}
	if prCalls != 0 {
		t.Errorf("GitHub consulted %d times after git said yes", prCalls)
	}
}

func TestIntegratedBy_GitErrorIsFinal(t *testing.T) {
	var gitCalls, prCalls int
	boom := errors.New("origin/HEAD unresolvable")
	_, err := integratedBy(countingProof(false, boom, &gitCalls), countingProof(true, nil, &prCalls))("feat")
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
	if prCalls != 0 {
		t.Errorf("GitHub consulted %d times after git failed", prCalls)
	}
}

func TestIntegratedBy_GitNoAsksGitHub(t *testing.T) {
	var gitCalls, prCalls int
	got, err := integratedBy(countingProof(false, nil, &gitCalls), countingProof(true, nil, &prCalls))("feat")
	if err != nil || !got {
		t.Errorf("git no, GitHub yes: got (%v, %v), want (true, nil)", got, err)
	}
	if gitCalls != 1 || prCalls != 1 {
		t.Errorf("calls git=%d github=%d, want 1 and 1", gitCalls, prCalls)
	}
}

func TestPRProof(t *testing.T) {
	tip := func(string) (string, error) { return "tip111", nil }
	emptyTip := func(string) (string, error) { return "", nil }
	noBranch := func(string) (string, error) { return "", errors.New("no branch") }
	heads := func(oids ...string) mergedHeadsFn {
		return func(string) ([]string, error) { return oids, nil }
	}
	failing := func(string) ([]string, error) { return nil, errors.New("gh: not logged in") }

	cases := []struct {
		name    string
		tip     tipFn
		heads   mergedHeadsFn
		want    bool
		wantErr bool
	}{
		{"merged PR at the tip", tip, heads("other", "tip111"), true, false},
		{"merged PR at a different tip", tip, heads("old999"), false, false},
		{"no merged PR", tip, heads(), false, false},
		{"empty tip never matches", emptyTip, heads(""), false, false},
		{"tip cannot be resolved", noBranch, heads("tip111"), false, true},
		{"GitHub error", tip, failing, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := prProof(c.tip, c.heads)("feat")
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
			if got != c.want {
				t.Errorf("integrated = %v, want %v", got, c.want)
			}
		})
	}
}

func TestParseHeadOids(t *testing.T) {
	got, err := parseHeadOids([]byte(`[{"headRefOid":"aaa"},{"headRefOid":"bbb"}]`))
	if err != nil || !reflect.DeepEqual(got, []string{"aaa", "bbb"}) {
		t.Errorf("got (%v, %v), want [aaa bbb]", got, err)
	}
	if got, err := parseHeadOids([]byte(`[]`)); err != nil || len(got) != 0 {
		t.Errorf("empty list: got (%v, %v)", got, err)
	}
	if _, err := parseHeadOids([]byte(`not json`)); err == nil {
		t.Error("garbage output must be an error, not an empty list")
	}
}
