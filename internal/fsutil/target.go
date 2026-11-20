package fsutil

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ResolveTarget returns the absolute path that should be replaced by an atomic
// write. A final symlink and symlinked parent directories are resolved so the
// link itself survives. A broken final symlink is followed lexically so a
// deleted target can still be checked against its real directory.
func ResolveTarget(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}
	info, err := os.Lstat(absolute)
	if errors.Is(err, fs.ErrNotExist) {
		return resolveParent(absolute)
	}
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", absolute, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return resolveParent(absolute)
	}
	if resolved, evalErr := filepath.EvalSymlinks(absolute); evalErr == nil {
		return resolved, nil
	}
	link, linkErr := os.Readlink(absolute)
	if linkErr != nil {
		return "", fmt.Errorf("resolve symlink %s: %w", absolute, linkErr)
	}
	if !filepath.IsAbs(link) {
		link = filepath.Join(filepath.Dir(absolute), link)
	}
	return resolveParent(filepath.Clean(link))
}

func resolveParent(path string) (string, error) {
	if directory, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
		return filepath.Join(directory, filepath.Base(path)), nil
	}
	return path, nil
}
