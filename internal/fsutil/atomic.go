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
	target, err := resolveTarget(path)
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
			mode = info.Mode().Perm()
		}
	}
	if mode == 0 {
		mode = 0o644
	}

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
	if err := temp.Chmod(mode.Perm()); err != nil {
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
		return err
	}
	return nil
}

// resolveTarget returns the absolute path that should be replaced. A symlink is
// resolved once so that the link itself survives the rename.
func resolveTarget(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return absolute, nil
		}
		return "", fmt.Errorf("stat %s: %w", absolute, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return absolute, nil
	}
	target, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve symlink %s: %w", absolute, err)
	}
	return target, nil
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
