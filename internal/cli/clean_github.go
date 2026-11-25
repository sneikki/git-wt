package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/shhac/git-wt/internal/git"
)

// mergedHeadsFn is the GitHub seam: the head commit ids of the merged pull
// requests whose head branch is branch. Tests fake it; the real one runs gh.
type mergedHeadsFn func(branch string) ([]string, error)

// tipFn resolves a local branch to its tip commit.
type tipFn func(branch string) (string, error)

// prProof is the GitHub proof: integrated when a merged pull request's head
// commit is exactly the branch tip. The sha equality is what protects a
// commit made after the merge; PR state alone never counts.
func prProof(tip tipFn, heads mergedHeadsFn) integrationFn {
	return func(branch string) (bool, error) {
		t, err := tip(branch)
		if err != nil {
			return false, err
		}
		oids, err := heads(branch)
		if err != nil {
			return false, err
		}
		return containsOID(oids, t), nil
	}
}

// containsOID reports whether want is one of oids. An empty want matches
// nothing. Pure.
func containsOID(oids []string, want string) bool {
	if want == "" {
		return false
	}
	for _, o := range oids {
		if o == want {
			return true
		}
	}
	return false
}

// parseHeadOids reads the output of `gh pr list --json headRefOid`. Pure.
func parseHeadOids(data []byte) ([]string, error) {
	var prs []struct {
		HeadRefOid string `json:"headRefOid"`
	}
	if err := json.Unmarshal(data, &prs); err != nil {
		return nil, fmt.Errorf("gh pr list: unreadable output: %w", err)
	}
	out := make([]string, 0, len(prs))
	for _, p := range prs {
		out = append(out, p.HeadRefOid)
	}
	return out, nil
}

// ghMergedHeads is the real mergedHeadsFn: one `gh pr list` per branch, run
// from the working directory's repository. gh missing, unauthenticated or
// without a GitHub remote is an error, and the caller keeps the branch.
func ghMergedHeads(ctx context.Context) mergedHeadsFn {
	return func(branch string) ([]string, error) {
		cmd := exec.CommandContext(ctx, "gh", "pr", "list",
			"--head", branch, "--state", "merged", "--json", "headRefOid", "--limit", "100")
		out, err := cmd.Output()
		if err != nil {
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				return nil, fmt.Errorf("gh pr list: %s", strings.TrimSpace(string(ee.Stderr)))
			}
			return nil, fmt.Errorf("gh pr list: %w", err)
		}
		return parseHeadOids(out)
	}
}

// tipOf resolves refs/heads/<branch> to its commit.
func tipOf(ctx context.Context) tipFn {
	return func(branch string) (string, error) {
		return git.Run(ctx, "rev-parse", "--verify", "-q", "refs/heads/"+branch+"^{commit}")
	}
}
