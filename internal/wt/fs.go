package wt

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// pathExists reports whether path exists. Errors other than not-existing are reported as exists=true.
func pathExists(path string) bool {
	_, err := os.Lstat(path)
	if err == nil {
		return true
	}
	return !errors.Is(err, os.ErrNotExist)
}

// CopyTree adds src to dst without overwriting. A dst that is absent is
// copied as today (symlinks stay symlinks). A dst that exists as a file or
// symlink is left alone, and nothing is written to or through it. A dst that
// is a real directory, with src a directory too, is descended into and the
// same rule applies per entry.
func CopyTree(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if pathExists(dst) {
		return mergeExisting(src, dst, info)
	}
	return copyNew(src, dst, info)
}

// mergeExisting handles a dst that already exists: only a real directory
// matched by a source directory is descended into.
func mergeExisting(src, dst string, info fs.FileInfo) error {
	if !info.IsDir() || !isRealDir(dst) {
		return nil
	}
	return copyEntries(src, dst)
}

// copyNew copies src to a dst that does not exist yet.
func copyNew(src, dst string, info fs.FileInfo) error {
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.Symlink(target, dst)
	case info.IsDir():
		if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
			return err
		}
		return copyEntries(src, dst)
	default:
		return copyFile(src, dst, info.Mode().Perm())
	}
}

// copyEntries applies CopyTree to every entry of the src directory.
func copyEntries(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := CopyTree(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// isRealDir reports whether path is a directory itself, not a symlink to one.
func isRealDir(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir()
}

// UnderSymlink reports whether a directory between root and path (path's
// parent chain, root excluded) is a symlink. Such a path must not be copied
// into, since the write would land in the symlink's target.
func UnderSymlink(root, path string) bool {
	rel, err := filepath.Rel(root, filepath.Dir(path))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return true
	}
	cur := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil {
			return false
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return true
		}
	}
	return false
}

func copyFile(src, dst string, perm fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// PathExists is the public version of pathExists, for callers in cli/.
func PathExists(path string) bool { return pathExists(path) }

// canonicalPath resolves symlinks (macOS reaches temp dirs via /var ->
// /private/var) so paths from porcelain and from gitdir files compare
// equal. A worktree whose directory is gone can't be resolved whole, so its
// deepest existing ancestor is resolved and the missing tail re-appended.
func canonicalPath(p string) string {
	p = filepath.Clean(p)
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	parent := filepath.Dir(p)
	if parent == p {
		return p
	}
	return filepath.Join(canonicalPath(parent), filepath.Base(p))
}
