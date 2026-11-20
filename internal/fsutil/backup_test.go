package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupCreatesNumberedCopies(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "game.p8")
	if err := os.WriteFile(path, []byte("one"), 0o640); err != nil {
		t.Fatal(err)
	}
	first, err := Backup(path)
	if err != nil {
		t.Fatal(err)
	}
	if first != path+".bak" {
		t.Fatalf("first backup = %q", first)
	}
	if data, err := os.ReadFile(first); err != nil || string(data) != "one" {
		t.Fatalf("backup data = %q err %v", data, err)
	}
	if info, err := os.Stat(first); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("backup mode = %v err %v", info.Mode(), err)
	}

	if err := os.WriteFile(path, []byte("two"), 0o640); err != nil {
		t.Fatal(err)
	}
	second, err := Backup(path)
	if err != nil {
		t.Fatal(err)
	}
	if second != path+".bak.1" {
		t.Fatalf("second backup = %q", second)
	}
	if data, err := os.ReadFile(second); err != nil || string(data) != "two" {
		t.Fatalf("second backup data = %q err %v", data, err)
	}
}

func TestBackupExhaustion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "game.p8")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range backupCandidates(path) {
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Backup(path); !errors.Is(err, ErrNoBackupName) {
		t.Fatalf("Backup error = %v, want ErrNoBackupName", err)
	}
}

func TestBackupDirectorySyncFailureIsNotACommit(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "game.p8")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o300); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	_, err := Backup(path)
	if err == nil {
		t.Fatal("Backup succeeded without a readable directory")
	}
	var committed *CommitError
	if errors.As(err, &committed) {
		t.Fatalf("backup sync error reported as a committed target: %v", err)
	}
}

func TestBackupMissingSource(t *testing.T) {
	if _, err := Backup(filepath.Join(t.TempDir(), "missing.p8")); err == nil {
		t.Fatal("Backup(missing) succeeded")
	}
}

func TestBackupSymlinkPreservesLink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.p8")
	link := filepath.Join(dir, "link.p8")
	if err := os.WriteFile(target, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink: %v", err)
	}
	backup, err := Backup(link)
	if err != nil {
		t.Fatal(err)
	}
	if backup != target+".bak" {
		t.Fatalf("backup = %q, want %q", backup, target+".bak")
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink was replaced: %v %v", info, err)
	}
}
