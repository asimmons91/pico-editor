package cart

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	p8cart "github.com/asimmons91/pico-lsp/pkg/cart"

	"github.com/asimmons91/pico-editor/internal/fsutil"
)

// SaveOptions controls the checks the conflict UI can override.
type SaveOptions struct {
	AllowExternalChange bool
	AllowOutsideRoot    bool
}

// SaveSnapshot captures a validated cart write. Prepare it on the Update
// goroutine, write it in the background, then commit it on the Update goroutine.
type SaveSnapshot struct {
	Path     string
	Target   string
	Data     []byte
	Mode     os.FileMode
	CodeText string
	Version  int

	diskHash [sha256.Size]byte
	parsed   *p8cart.Document
}

// PrepareSave validates and captures the current cart bytes.
func (d *Document) PrepareSave(options SaveOptions) (SaveSnapshot, error) {
	if d.Parsed == nil || !d.Parsed.HasCode() {
		return SaveSnapshot{}, fmt.Errorf("%w: %s", ErrNoCode, d.Path)
	}
	if d.disk.known && !options.AllowExternalChange {
		if err := d.CheckExternal(); err != nil {
			return SaveSnapshot{}, err
		}
	}
	mode := d.Code.Mode
	if options.AllowExternalChange {
		info, statErr := os.Stat(d.Path)
		switch {
		case statErr == nil:
			mode = info.Mode() & modeBits
		case errors.Is(statErr, fs.ErrNotExist):
			if mode.Perm()&0o222 == 0 {
				mode = 0o644
			}
		default:
			return SaveSnapshot{}, fmt.Errorf("stat %s: %w", d.Path, statErr)
		}
	}
	if mode.Perm()&0o222 == 0 {
		return SaveSnapshot{}, fmt.Errorf("%w: %s", ErrReadOnly, d.Path)
	}
	data, err := d.FullBytes()
	if err != nil {
		return SaveSnapshot{}, err
	}
	parsed, err := p8cart.Parse(d.Path, data)
	if err != nil {
		return SaveSnapshot{}, err
	}
	target, err := saveTarget(d.Path, d.workspaceRoot, options.AllowOutsideRoot)
	if err != nil {
		return SaveSnapshot{}, err
	}
	return SaveSnapshot{
		Path:     d.Path,
		Target:   target,
		Data:     data,
		Mode:     mode,
		CodeText: d.Code.Text(),
		Version:  d.Code.Version,
		diskHash: sha256.Sum256(data),
		parsed:   parsed,
	}, nil
}

// Write performs the atomic write captured by PrepareSave.
func (s SaveSnapshot) Write() error {
	return fsutil.AtomicWrite(s.Target, s.Data, s.Mode)
}

// Backup copies the current on-disk cart to the next free .bak name.
func (s SaveSnapshot) Backup() (string, error) {
	return fsutil.Backup(s.Path)
}

// CommitSave applies the result of a snapshot write. A CommitError means the
// file was replaced but durability could not be confirmed; the save point is
// still advanced so retries are not blocked by a false conflict.
func (d *Document) CommitSave(snapshot SaveSnapshot, writeErr error) error {
	var committed *fsutil.CommitError
	if writeErr != nil && !errors.As(writeErr, &committed) {
		return writeErr
	}
	if snapshot.parsed == nil {
		return errors.New("cart save snapshot has no parsed document")
	}
	d.Raw = append([]byte(nil), snapshot.Data...)
	d.Parsed = snapshot.parsed
	d.BaseLine = d.Parsed.CodeStartLine()
	d.Code.MarkSavedTextVersion(snapshot.CodeText, snapshot.Version)
	if info, err := os.Stat(snapshot.Target); err == nil {
		d.Code.Mode = info.Mode() & modeBits
		d.disk = diskState{
			known:   true,
			size:    info.Size(),
			modTime: info.ModTime(),
			mode:    info.Mode() & modeBits,
			hash:    snapshot.diskHash,
		}
	} else {
		d.disk.known = false
	}
	if writeErr != nil {
		return writeErr
	}
	return nil
}

// Save writes the cart synchronously. The app uses PrepareSave/Write/CommitSave
// for asynchronous saves; tests and simple callers can use Save.
func (d *Document) Save() error {
	snapshot, err := d.PrepareSave(SaveOptions{})
	if err != nil {
		return err
	}
	return d.CommitSave(snapshot, snapshot.Write())
}

func saveTarget(path, root string, allowOutsideRoot bool) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}
	target, err := fsutil.ResolveTarget(absolute)
	if err != nil {
		return "", err
	}
	if root == "" {
		root = filepath.Dir(absolute)
	}
	if resolvedRoot, rootErr := filepath.EvalSymlinks(root); rootErr == nil {
		root = resolvedRoot
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		if !allowOutsideRoot {
			return "", fmt.Errorf("%w: %s", ErrOutsideRoot, target)
		}
	}
	return target, nil
}
