// Package editor owns the line-based text buffer, cursor, selection, viewport,
// undo history, search, and safe file IO. It knows nothing about Vim, carts, or
// LSP.
package editor

import (
	"errors"
	"fmt"
)

// Typed errors returned by the editor. File errors are wrapped in PathError so
// callers can both match the class with errors.Is and report the path.
var (
	ErrNotUTF8         = errors.New("file is not valid UTF-8")
	ErrTooLarge        = errors.New("file is larger than 2MB")
	ErrReadOnly        = errors.New("file is read-only")
	ErrExternalChange  = errors.New("file changed on disk")
	ErrMixedNewlines   = errors.New("file has mixed line endings")
	ErrAmbiguousCR     = errors.New("line ending in CR would be indistinguishable from CRLF")
	ErrNotRegularFile  = errors.New("not a regular file")
	ErrUnsupportedFile = errors.New("unsupported file type until M4")
	ErrNoPath          = errors.New("buffer has no path")
	ErrOutsideRoot     = errors.New("symlink target is outside the workspace root")

	ErrInvalidPosition = errors.New("position is not a cluster boundary")
	ErrInvalidRange    = errors.New("invalid edit range")
	ErrInvalidText     = errors.New("edit text is not valid UTF-8")
	ErrNoCluster       = errors.New("no cluster at the cursor")
	ErrInvalidPattern  = errors.New("invalid search pattern")
)

// PathError attaches the affected path to an editor error.
type PathError struct {
	Op   string
	Path string
	Err  error
}

func (e *PathError) Error() string {
	return fmt.Sprintf("%s %s: %v", e.Op, e.Path, e.Err)
}

// Unwrap exposes the typed error for errors.Is.
func (e *PathError) Unwrap() error {
	return e.Err
}

func pathError(op, path string, err error) error {
	return &PathError{Op: op, Path: path, Err: err}
}
