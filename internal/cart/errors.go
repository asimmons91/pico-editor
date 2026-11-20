// Package cart adapts pico-lsp's byte-preserving cart parser to the editor. It
// keeps the original cart bytes immutable and exposes only the __lua__ body to
// the editor buffer.
package cart

import (
	"errors"
)

var (
	// ErrNoCode reports a cart with no __lua__ section.
	ErrNoCode = errors.New("cart has no __lua__ section")
	// ErrExternalChange reports that the file changed since open/last save.
	ErrExternalChange = errors.New("cart changed on disk")
	// ErrDeleted reports that the file was removed since open/last save.
	ErrDeleted = errors.New("cart was deleted on disk")
	// ErrReadOnly reports a target without write permission.
	ErrReadOnly = errors.New("cart is read-only")
	// ErrOutsideRoot reports a resolved target outside the workspace root.
	ErrOutsideRoot = errors.New("symlink target is outside the workspace root")
	// ErrInvalidPath reports an unusable cart path.
	ErrInvalidPath = errors.New("invalid cart path")
	// ErrCodeSection reports code text that would introduce a cart section
	// marker and truncate the __lua__ body on the next load.
	ErrCodeSection = errors.New("code contains a cart section header")
)
