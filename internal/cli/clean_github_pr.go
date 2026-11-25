package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ghTimeout bounds every gh call; a timeout is an error, hence no proof.
const ghTimeout = 30 * time.Second

// mergedPR is a merged pull request: the commit its branch ended at and the
// branch it merged into.
type mergedPR struct {
	HeadOid string `json:"headRefOid"`
	Base    string `json:"baseRefName"`
}

// mergedPRsFn is the GitHub seam of the merged-PR scan: the merged pull
// requests whose head branch is branch.
type mergedPRsFn func(branch string) ([]mergedPR, error)

// isBaseFn reports whether any pull request, in any state, targets branch.
type isBaseFn func(branch string) (bool, error)

// ghRun runs gh with a timeout and returns stdout. Any failure is an error
// that carries gh's own message.
func ghRun(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, ghTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "gh", args...).Output()
	if err == nil {
		return out, nil
	}
	var ee *exec.ExitError
	if ctx.Err() != nil {
		return nil, fmt.Errorf("gh pr list: %w", ctx.Err())
	}
	if errors.As(err, &ee) {
		return nil, fmt.Errorf("gh pr list: %s", strings.TrimSpace(string(ee.Stderr)))
	}
	return nil, fmt.Errorf("gh pr list: %w", err)
}

// parseMergedPRs reads `gh pr list --json headRefOid,baseRefName`. Pure.
func parseMergedPRs(data []byte) ([]mergedPR, error) {
	var prs []mergedPR
	if err := json.Unmarshal(data, &prs); err != nil {
		return nil, fmt.Errorf("gh pr list: unreadable output: %w", err)
	}
	return prs, nil
}

// ghMergedPRs is the real mergedPRsFn.
func ghMergedPRs(ctx context.Context) mergedPRsFn {
	return func(branch string) ([]mergedPR, error) {
		out, err := ghRun(ctx, "pr", "list", "--head", branch, "--state", "merged",
			"--json", "headRefOid,baseRefName", "--limit", "100")
		if err != nil {
			return nil, err
		}
		return parseMergedPRs(out)
	}
}

// ghIsBase is the real isBaseFn: one PR in any state targeting branch is
// enough, so the call asks for one.
func ghIsBase(ctx context.Context) isBaseFn {
	return func(branch string) (bool, error) {
		out, err := ghRun(ctx, "pr", "list", "--base", branch, "--state", "all",
			"--json", "number", "--limit", "1")
		if err != nil {
			return false, err
		}
		var prs []struct{}
		if err := json.Unmarshal(out, &prs); err != nil {
			return false, fmt.Errorf("gh pr list: unreadable output: %w", err)
		}
		return len(prs) > 0, nil
	}
}
