package wt

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestUnderSymlink: a path whose parent chain under root holds a symlink is
// reported, so copying into it would write through the link.
func TestUnderSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".agents", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../.agents/skills", filepath.Join(root, ".claude", "skills")); err != nil {
		t.Fatal(err)
	}
	if !UnderSymlink(root, filepath.Join(root, ".claude", "skills", "new.md")) {
		t.Errorf("path under symlinked dir: UnderSymlink = false, want true")
	}
	if UnderSymlink(root, filepath.Join(root, ".claude", "settings.json")) {
		t.Errorf("path under real dir: UnderSymlink = true, want false")
	}
}
