package fsutil

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAtomicWriteCreatesAndReplaces(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "file.lua")
	if err := AtomicWrite(path, []byte("one"), 0o640); err != nil {
		t.Fatalf("AtomicWrite create: %v", err)
	}
	if err := AtomicWrite(path, []byte("two"), 0o640); err != nil {
		t.Fatalf("AtomicWrite replace: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "two" {
		t.Errorf("contents = %q, want two", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Errorf("mode = %o, want 640", got)
	}
}

func TestAtomicWritePreservesSymlink(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "target.lua")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(dir, "link.lua")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if err := AtomicWrite(link, []byte("new"), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("atomic write replaced the symlink")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != "new" {
		t.Errorf("target contents = %q, want new", data)
	}
}

func TestAtomicWriteRenameFailureLeavesOriginal(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "file.lua")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatalf("write original: %v", err)
	}

	operations := defaultOps
	operations.rename = func(string, string) error {
		return errors.New("injected rename failure")
	}
	err := atomicWrite(path, []byte("replacement"), 0o600, operations)
	if err == nil {
		t.Fatal("atomicWrite succeeded with an injected rename failure")
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read original: %v", readErr)
	}
	if string(data) != "original" {
		t.Errorf("original changed to %q", data)
	}

	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatalf("read dir: %v", readErr)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Errorf("temporary file %q was not removed", entry.Name())
		}
	}
}

func TestAtomicWriteDirectorySyncFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "file.lua")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatalf("write original: %v", err)
	}

	operations := defaultOps
	operations.syncDir = func(string) error {
		return errors.New("injected sync failure")
	}
	if err := atomicWrite(path, []byte("replacement"), 0o600, operations); err == nil {
		t.Fatal("atomicWrite succeeded with an injected directory sync failure")
	}
	// The rename already happened; the data is complete even though durability
	// could not be confirmed.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !bytes.Equal(data, []byte("replacement")) {
		t.Errorf("contents = %q, want replacement", data)
	}
}
