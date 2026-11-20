// Package fsutil provides atomic file replacement and directory fsync helpers.
package fsutil

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type atomicOps struct {
	createTemp func(dir, pattern string) (*os.File, error)
	rename     func(oldPath, newPath string) error
	syncDir    func(dir string) error
}

// CommitError reports a failure after the new file was already renamed into
// place. The target contains the new bytes, but durability could not be
// confirmed.
type CommitError struct {
	Err error
}

func (e *CommitError) Error() string {
	return "file committed but not durably synced: " + e.Err.Error()
}

// Unwrap exposes the underlying sync error.
func (e *CommitError) Unwrap() error {
	return e.Err
}

const chmodBits = fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky

var defaultOps = atomicOps{
	createTemp: os.CreateTemp,
	rename:     os.Rename,
	syncDir:    syncDir,
}

// AtomicWrite replaces path with data. It writes a temporary file in the same
// directory, fsyncs it, applies mode, renames it over the target, and fsyncs the
// directory. A symlink target is resolved so the symlink itself is preserved.
// On any pre-rename failure the original file is left untouched and the
// temporary file is removed.
func AtomicWrite(path string, data []byte, mode fs.FileMode) error {
	return atomicWrite(path, data, mode, defaultOps)
}

func atomicWrite(path string, data []byte, mode fs.FileMode, ops atomicOps) error {
	target, err := ResolveTarget(path)
	if err != nil {
		return err
	}
	dir := filepath.Dir(target)

	info, statErr := os.Stat(target)
	exists := statErr == nil
	if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", target, statErr)
	}
	if exists {
		if info.IsDir() {
			return fmt.Errorf("%s is a directory", target)
		}
		if mode == 0 {
			mode = info.Mode() & chmodBits
		}
	}
	if mode == 0 {
		mode = 0o644
	}
	mode &= chmodBits

	temp, err := ops.createTemp(dir, "."+filepath.Base(target)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file in %s: %w", dir, err)
	}
	tempPath := temp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write temporary file %s: %w", tempPath, err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync temporary file %s: %w", tempPath, err)
	}
	if exists {
		if err := preserveOwner(tempPath, info); err != nil {
			_ = temp.Close()
			return err
		}
	}
	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return fmt.Errorf("chmod temporary file %s: %w", tempPath, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary file %s: %w", tempPath, err)
	}
	if err := ops.rename(tempPath, target); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tempPath, target, err)
	}
	renamed = true
	if err := ops.syncDir(dir); err != nil {
		return &CommitError{Err: err}
	}
	return nil
}

func syncDir(dir string) error {
	handle, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open directory %s: %w", dir, err)
	}
	defer func() {
		_ = handle.Close()
	}()
	if err := handle.Sync(); err != nil {
		return fmt.Errorf("sync directory %s: %w", dir, err)
	}
	return nil
}
