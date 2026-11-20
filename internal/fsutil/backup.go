package fsutil

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ErrNoBackupName reports that every numbered backup name is occupied.
var ErrNoBackupName = errors.New("no free backup name")

// Backup copies path to the first free sibling name: <name>.bak, then
// <name>.bak.1 through <name>.bak.9. It returns the backup path. The copy is
// written exclusively, fsynced, and removed on failure, so a failed backup
// never becomes the source of an overwrite.
func Backup(path string) (string, error) {
	target, err := ResolveTarget(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", fmt.Errorf("stat backup source %s: %w", target, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("backup source %s: %w", target, errors.New("not a regular file"))
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return "", fmt.Errorf("read backup source %s: %w", target, err)
	}

	for _, candidate := range backupCandidates(target) {
		backup, err := writeBackup(candidate, data, info.Mode()&chmodBits)
		if err == nil {
			return backup, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", err
		}
	}
	return "", fmt.Errorf("%w for %s", ErrNoBackupName, target)
}

func backupCandidates(path string) []string {
	candidates := []string{path + ".bak"}
	for index := 1; index <= 9; index++ {
		candidates = append(candidates, fmt.Sprintf("%s.bak.%d", path, index))
	}
	return candidates
}

func writeBackup(path string, data []byte, mode fs.FileMode) (string, error) {
	if mode == 0 {
		mode = 0o644
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode&chmodBits)
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("write backup %s: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("sync backup %s: %w", path, err)
	}
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("chmod backup %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close backup %s: %w", path, err)
	}
	if err := syncDir(filepath.Dir(path)); err != nil {
		return "", fmt.Errorf("sync backup directory %s: %w", filepath.Dir(path), err)
	}
	ok = true
	return path, nil
}
