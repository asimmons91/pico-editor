package fsutil

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const maxSymlinkDepth = 40

// ResolveTarget returns the absolute path that should be replaced by an atomic
// write. Symlinks, including parent directories and broken chains, are resolved
// component by component so the link itself survives and a missing target can
// still be checked against its real directory.
func ResolveTarget(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}
	return resolveComponents(absolute, 0)
}

func resolveComponents(path string, depth int) (string, error) {
	if depth > maxSymlinkDepth {
		return "", fmt.Errorf("resolve %s: too many symlink levels", path)
	}
	volume := filepath.VolumeName(path)
	rest := strings.TrimPrefix(path, volume)
	root := volume + string(filepath.Separator)
	current := root
	for _, component := range strings.Split(strings.Trim(rest, string(filepath.Separator)), string(filepath.Separator)) {
		switch component {
		case "", ".":
			continue
		case "..":
			current = filepath.Dir(current)
			continue
		}
		candidate := filepath.Join(current, component)
		info, err := os.Lstat(candidate)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				current = candidate
				continue
			}
			return "", fmt.Errorf("stat %s: %w", candidate, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			current = candidate
			continue
		}
		link, err := os.Readlink(candidate)
		if err != nil {
			return "", fmt.Errorf("readlink %s: %w", candidate, err)
		}
		if !filepath.IsAbs(link) {
			link = current + string(filepath.Separator) + link
		}
		resolved, err := resolveComponents(link, depth+1)
		if err != nil {
			return "", err
		}
		current = resolved
	}
	return filepath.Clean(current), nil
}
