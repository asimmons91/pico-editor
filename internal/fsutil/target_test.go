package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTargetThroughSymlinkedParentAndRelativeLink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outside, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "dir"), filepath.Join(root, "linkdir")); err != nil {
		t.Skipf("symlink: %v", err)
	}
	if err := os.Symlink("../target.lua", filepath.Join(outside, "dir", "real.lua")); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveTarget(filepath.Join(root, "linkdir", "real.lua"))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(outside, "target.lua")
	if got != want {
		t.Fatalf("ResolveTarget = %q, want %q", got, want)
	}
}

func TestResolveTargetThroughBrokenSymlinkChain(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "real.lua")
	if err := os.Symlink(target, filepath.Join(root, "b.lua")); err != nil {
		t.Skipf("symlink: %v", err)
	}
	if err := os.Symlink("b.lua", filepath.Join(root, "a.lua")); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveTarget(filepath.Join(root, "a.lua"))
	if err != nil {
		t.Fatal(err)
	}
	if got != target {
		t.Fatalf("ResolveTarget = %q, want %q", got, target)
	}
}
