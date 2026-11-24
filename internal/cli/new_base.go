package cli

// Start-point resolution for `new`: fetch the remote the base lives on,
// then name the ref to branch from. Kept apart from new.go so the command
// stays a short RunE.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shhac/git-wt/internal/git"
)

// newBase is the start point `new` branches from.
type newBase struct {
	Ref string // "origin/main", a --from ref, or "HEAD"
	SHA string // short sha Ref resolved to
}

// resolveNewBase fetches the remote the base lives on (unless noFetch) and
// resolves the start point. Without any remote it is the local ref as given,
// or HEAD.
func resolveNewBase(ctx context.Context, from string, noFetch bool) (newBase, error) {
	remotes, err := listRemotes(ctx)
	if err != nil {
		return newBase{}, err
	}
	remote := pickRemote(from, remotes)
	fetched := remote != "" && !noFetch
	if fetched {
		if err := fetchRemote(ctx, remote); err != nil {
			return newBase{}, err
		}
	}
	ref, err := baseRef(ctx, from, remote, fetched)
	if err != nil {
		return newBase{}, err
	}
	sha, err := git.Run(ctx, "rev-parse", "--verify", "--short", ref+"^{commit}")
	if err != nil {
		return newBase{}, fmt.Errorf("base %q: %w", ref, err)
	}
	return newBase{Ref: ref, SHA: sha}, nil
}

// pickRemote names the remote to fetch before branching: the one that the
// `<remote>/` prefix of from names, else origin, else "" (no fetch).
func pickRemote(from string, remotes []string) string {
	for _, r := range remotes {
		if strings.HasPrefix(from, r+"/") {
			return r
		}
	}
	for _, r := range remotes {
		if r == "origin" {
			return r
		}
	}
	return ""
}

// baseRef is --from when given; otherwise origin's default branch when a
// remote is in play (pickRemote only yields origin for an empty from), else
// HEAD. fetched says the remote was just fetched, so origin/HEAD is refreshed.
func baseRef(ctx context.Context, from, remote string, fetched bool) (string, error) {
	switch {
	case from != "":
		return from, nil
	case remote == "":
		return "HEAD", nil
	default:
		return originDefaultRef(ctx, fetched)
	}
}

// describeBase is the one stderr line that says what new branched from.
func describeBase(b newBase) string {
	return fmt.Sprintf("branched from %s at %s", b.Ref, b.SHA)
}

func listRemotes(ctx context.Context) ([]string, error) {
	out, err := git.Run(ctx, "remote")
	if err != nil {
		return nil, err
	}
	return strings.Fields(out), nil
}

func fetchRemote(ctx context.Context, remote string) error {
	if _, err := git.Run(ctx, "fetch", "--quiet", remote); err != nil {
		return fmt.Errorf("fetch %s failed: %w (to branch from the refs as they are, pass --no-fetch)", remote, err)
	}
	return nil
}

// originDefaultRef returns origin's default branch as "origin/<name>". After
// a fetch (refresh) origin/HEAD is re-read from the remote, so a renamed
// default is followed; a failed refresh is an error. Without a fetch the
// stored origin/HEAD is used, and unset is an error that never contacts origin.
func originDefaultRef(ctx context.Context, refresh bool) (string, error) {
	if refresh {
		if _, err := git.Run(ctx, "remote", "set-head", "origin", "--auto"); err != nil {
			return "", fmt.Errorf("refresh origin/HEAD failed: %w (or pass --from <ref>)", err)
		}
	}
	ref, err := symbolicOriginHead(ctx)
	if err != nil {
		return "", errors.New("origin/HEAD is not set (pass --from <ref>, or drop --no-fetch)")
	}
	return ref, nil
}

func symbolicOriginHead(ctx context.Context) (string, error) {
	return git.Run(ctx, "symbolic-ref", "-q", "--short", "refs/remotes/origin/HEAD")
}
