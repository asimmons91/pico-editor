package editor

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/asimmons91/pico-editor/internal/fsutil"
)

// maxFileSize is the M2 open limit. Cart-sized Lua files are far smaller.
const maxFileSize = 2 * 1024 * 1024

// modeBits are the mode bits os.Chmod preserves on Unix.
const modeBits = os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky

var utf8BOM = []byte{0xef, 0xbb, 0xbf}

// Buffer is a line-based text buffer with no line terminators stored in Lines.
// Newline is the file's detected terminator ("\n" or "\r\n"). TrailingNewline
// records whether the file ended with a terminator.
type Buffer struct {
	Path            string
	Lines           []string
	Newline         string
	TrailingNewline bool
	HadBOM          bool
	Version         int
	SavedVersion    int
	Mode            os.FileMode
	TabWidth        int

	savedHash [sha256.Size]byte
	hash      [sha256.Size]byte
	hashValid bool

	lineStarts []int
	indexValid bool

	workspaceRoot string
	disk          diskState
}

type diskState struct {
	known   bool
	size    int64
	modTime time.Time
	mode    os.FileMode
	hash    [sha256.Size]byte
}

func isCartPath(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".p8") || strings.HasSuffix(lower, ".p8.png")
}

// Open reads an existing text file. Cart files are rejected until the M4 cart
// adapter can preserve non-code sections.
func Open(path string) (*Buffer, error) {
	if isCartPath(path) {
		return nil, pathError("open", path, ErrUnsupportedFile)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(path); resolveErr == nil && isCartPath(resolved) {
		return nil, pathError("open", path, ErrUnsupportedFile)
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, pathError("open", path, err)
	}
	if info.IsDir() {
		return nil, pathError("open", path, errors.New("is a directory"))
	}
	if !info.Mode().IsRegular() {
		return nil, pathError("open", path, ErrNotRegularFile)
	}
	if info.Size() > maxFileSize {
		return nil, pathError("open", path, ErrTooLarge)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, pathError("open", path, err)
	}
	if len(data) > maxFileSize {
		return nil, pathError("open", path, ErrTooLarge)
	}

	lines, newline, trailing, hadBOM, err := parseFileData(data)
	if err != nil {
		return nil, pathError("open", path, err)
	}

	absolute, absErr := filepath.Abs(path)
	if absErr != nil {
		return nil, pathError("open", path, absErr)
	}
	buffer := &Buffer{
		Path:            path,
		Lines:           lines,
		Newline:         newline,
		TrailingNewline: trailing,
		HadBOM:          hadBOM,
		Version:         1,
		SavedVersion:    1,
		Mode:            info.Mode() & modeBits,
		TabWidth:        2,
		workspaceRoot:   filepath.Dir(absolute),
		disk: diskState{
			known:   true,
			size:    info.Size(),
			modTime: info.ModTime(),
			mode:    info.Mode() & modeBits,
			hash:    sha256.Sum256(data),
		},
	}
	buffer.savedHash = buffer.textHash()
	return buffer, nil
}

// New returns an empty buffer associated with path. The file is not read or
// created until Save.
func New(path string) *Buffer {
	buffer := &Buffer{
		Path:         path,
		Lines:        []string{""},
		Newline:      "\n",
		Version:      1,
		SavedVersion: 1,
		Mode:         0o644,
		TabWidth:     2,
	}
	if path != "" {
		if absolute, err := filepath.Abs(path); err == nil {
			buffer.workspaceRoot = filepath.Dir(absolute)
		}
	}
	buffer.savedHash = buffer.textHash()
	return buffer
}

// parseFileData validates and splits file bytes without normalizing anything
// that the M2 buffer model can round-trip.
func parseFileData(data []byte) (lines []string, newline string, trailing, hadBOM bool, err error) {
	if bytes.HasPrefix(data, utf8BOM) {
		hadBOM = true
		data = data[len(utf8BOM):]
	}
	if !utf8.Valid(data) {
		return nil, "", false, hadBOM, ErrNotUTF8
	}

	sawCRLF := false
	sawLF := false
	for i := 0; i < len(data); i++ {
		if data[i] != '\n' {
			continue
		}
		if i > 0 && data[i-1] == '\r' {
			sawCRLF = true
		} else {
			sawLF = true
		}
	}
	if sawCRLF && sawLF {
		return nil, "", false, hadBOM, ErrMixedNewlines
	}
	newline = "\n"
	if sawCRLF {
		newline = "\r\n"
	}

	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] != '\n' {
			continue
		}
		end := i
		if newline == "\r\n" && end > start && data[end-1] == '\r' {
			end--
		}
		lines = append(lines, string(data[start:end]))
		start = i + 1
	}
	if start < len(data) || len(data) == 0 {
		lines = append(lines, string(data[start:]))
	}
	if len(data) > 0 && data[len(data)-1] == '\n' {
		trailing = true
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	return lines, newline, trailing, hadBOM, nil
}

// Text returns the canonical in-memory text, using LF newlines and no BOM.
func (b *Buffer) Text() string {
	if len(b.Lines) == 0 {
		return ""
	}
	size := 0
	for _, line := range b.Lines {
		size += len(line) + 1
	}
	var out strings.Builder
	out.Grow(size)
	for i, line := range b.Lines {
		if i > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(line)
	}
	if b.TrailingNewline {
		out.WriteByte('\n')
	}
	return out.String()
}

// Dirty reports whether the current text differs from the last saved text. It
// compares a cached content hash, so undoing back to the save point clears the
// flag without writing the file.
func (b *Buffer) Dirty() bool {
	return b.textHash() != b.savedHash
}

// MarkSaved records the current text as the save point. It invalidates cached
// text data first, so callers that load Lines directly (for example the M4 cart
// adapter) get a correct hash rather than a stale one.
func (b *Buffer) MarkSaved() {
	b.MarkSavedText(b.Text())
}

// MarkSavedText records text as the save point without requiring the buffer's
// current text to match it. The cart adapter uses it when a background save
// commits a snapshot while newer edits are already in the buffer.
func (b *Buffer) MarkSavedText(text string) {
	b.hashValid = false
	b.indexValid = false
	b.savedHash = sha256.Sum256([]byte(text))
	b.SavedVersion = b.Version
}

// SetWorkspaceRoot narrows or widens the directory used for the save-time
// symlink boundary check. It does not affect file contents.
func (b *Buffer) SetWorkspaceRoot(root string) {
	if root == "" {
		return
	}
	b.workspaceRoot = root
}

func (b *Buffer) textHash() [sha256.Size]byte {
	if !b.hashValid {
		b.hash = sha256.Sum256([]byte(b.Text()))
		b.hashValid = true
	}
	return b.hash
}

func (b *Buffer) invalidate() {
	b.hashValid = false
	b.indexValid = false
	b.Version++
}

// Save writes the buffer atomically. It refuses to overwrite a file that
// changed since open/last save and refuses read-only files, cart paths, and
// symlink targets outside the M2 workspace root (the file's parent directory).
// An LF file whose line content ends in CR cannot be written unambiguously; it
// returns ErrAmbiguousCR instead of creating bytes that would reopen as CRLF.
func (b *Buffer) Save() error {
	snapshot, err := b.PrepareSave()
	if err != nil {
		return err
	}
	return b.CommitSave(snapshot, snapshot.Write())
}

// SaveSnapshot captures a validated write without touching the buffer. Prepare
// it on the Update goroutine, call Write from a background goroutine, then call
// CommitSave on the Update goroutine. The snapshot's hash becomes the save
// point even if the buffer changed while the write was in flight.
type SaveSnapshot struct {
	Path    string
	Target  string
	Data    []byte
	Mode    os.FileMode
	Version int

	hash     [sha256.Size]byte // canonical text save point
	diskHash [sha256.Size]byte // encoded bytes on disk
}

// SaveOptions controls checks that the M4 conflict UI can explicitly override.
type SaveOptions struct {
	// AllowExternalChange permits writing even though the file changed or was
	// deleted after open/last save. The caller is responsible for backing up
	// conflicting bytes first.
	AllowExternalChange bool
	// AllowOutsideRoot permits a resolved target outside WorkspaceRoot.
	AllowOutsideRoot bool
}

// PrepareSave performs the same checks as Save and captures the bytes to write.
// It returns typed errors without modifying the buffer or the file.
func (b *Buffer) PrepareSave() (SaveSnapshot, error) {
	return b.PrepareSaveWithOptions(SaveOptions{})
}

// PrepareSaveWithOptions is PrepareSave with explicit conflict overrides.
func (b *Buffer) PrepareSaveWithOptions(options SaveOptions) (SaveSnapshot, error) {
	if b.Path == "" {
		return SaveSnapshot{}, ErrNoPath
	}
	if isCartPath(b.Path) {
		return SaveSnapshot{}, pathError("save", b.Path, ErrUnsupportedFile)
	}
	if b.hasAmbiguousCR() {
		return SaveSnapshot{}, pathError("save", b.Path, ErrAmbiguousCR)
	}
	if b.disk.known && !options.AllowExternalChange {
		if err := b.checkExternal(); err != nil {
			return SaveSnapshot{}, err
		}
	}
	if b.Mode.Perm()&0o222 == 0 {
		return SaveSnapshot{}, pathError("save", b.Path, ErrReadOnly)
	}

	target, err := b.saveTarget(options.AllowOutsideRoot)
	if err != nil {
		return SaveSnapshot{}, err
	}
	data := b.encode()
	return SaveSnapshot{
		Path:     b.Path,
		Target:   target,
		Data:     data,
		Mode:     b.Mode,
		Version:  b.Version,
		hash:     b.textHash(),
		diskHash: sha256.Sum256(data),
	}, nil
}

// Write performs the atomic write captured by PrepareSave. It does not touch
// the buffer and is safe to run off the Update goroutine.
func (s SaveSnapshot) Write() error {
	return fsutil.AtomicWrite(s.Target, s.Data, s.Mode)
}

// CommitSave records the result of a snapshot write. A CommitError means the
// target was replaced but a post-rename step failed; the save point is still
// updated so a retry is not blocked by a false external-change error.
func (b *Buffer) CommitSave(snapshot SaveSnapshot, writeErr error) error {
	var committed *fsutil.CommitError
	if writeErr != nil && !errors.As(writeErr, &committed) {
		return pathError("save", b.Path, writeErr)
	}

	b.savedHash = snapshot.hash
	b.SavedVersion = snapshot.Version
	if info, statErr := os.Stat(snapshot.Target); statErr == nil {
		b.Mode = info.Mode() & modeBits
		b.disk = diskState{
			known:   true,
			size:    info.Size(),
			modTime: info.ModTime(),
			mode:    b.Mode,
			hash:    snapshot.diskHash,
		}
	} else {
		b.disk.known = false
	}
	if writeErr != nil {
		return pathError("save", b.Path, writeErr)
	}
	return nil
}

func (b *Buffer) hasAmbiguousCR() bool {
	if b.Newline != "\n" {
		return false
	}
	for i, line := range b.Lines {
		if !strings.HasSuffix(line, "\r") {
			continue
		}
		if i < len(b.Lines)-1 || b.TrailingNewline {
			return true
		}
	}
	return false
}

// finishWrite is the synchronous compatibility wrapper around CommitSave.
func (b *Buffer) finishWrite(target string, data []byte, writeErr error) error {
	return b.CommitSave(SaveSnapshot{
		Path:     b.Path,
		Target:   target,
		Data:     data,
		Mode:     b.Mode,
		Version:  b.Version,
		hash:     b.textHash(),
		diskHash: sha256.Sum256(data),
	}, writeErr)
}

func (b *Buffer) encode() []byte {
	size := 0
	if b.HadBOM {
		size += len(utf8BOM)
	}
	for i, line := range b.Lines {
		if i > 0 {
			size += len(b.Newline)
		}
		size += len(line)
	}
	if b.TrailingNewline {
		size += len(b.Newline)
	}
	var out bytes.Buffer
	out.Grow(size)
	if b.HadBOM {
		out.Write(utf8BOM)
	}
	for i, line := range b.Lines {
		if i > 0 {
			out.WriteString(b.Newline)
		}
		out.WriteString(line)
	}
	if b.TrailingNewline {
		out.WriteString(b.Newline)
	}
	return out.Bytes()
}

func (b *Buffer) checkExternal() error {
	info, err := os.Stat(b.Path)
	if err != nil {
		return pathError("save", b.Path, errors.Join(ErrExternalChange, err))
	}
	if info.Size() != b.disk.size || !info.ModTime().Equal(b.disk.modTime) ||
		info.Mode()&modeBits != b.disk.mode {
		return pathError("save", b.Path, ErrExternalChange)
	}
	data, err := os.ReadFile(b.Path)
	if err != nil {
		return pathError("save", b.Path, errors.Join(ErrExternalChange, err))
	}
	if sha256.Sum256(data) != b.disk.hash {
		return pathError("save", b.Path, ErrExternalChange)
	}
	return nil
}

// CheckExternal reports whether the file changed since open or the last save.
// It returns nil when no disk state is known or the file still matches.
func (b *Buffer) CheckExternal() error {
	if !b.disk.known {
		return nil
	}
	return b.checkExternal()
}

// saveTarget resolves symlinks and enforces the M2 workspace boundary. The
// workspace root is the directory containing the buffer path.
func (b *Buffer) saveTarget(allowOutsideRoot bool) (string, error) {
	absolute, err := filepath.Abs(b.Path)
	if err != nil {
		return "", pathError("save", b.Path, err)
	}
	info, err := os.Lstat(absolute)
	if errors.Is(err, fs.ErrNotExist) {
		return absolute, nil
	}
	if err != nil {
		return "", pathError("save", b.Path, err)
	}

	target := absolute
	if info.Mode()&os.ModeSymlink != 0 {
		target, err = filepath.EvalSymlinks(absolute)
		if err != nil {
			return "", pathError("save", b.Path, err)
		}
	} else if directory, dirErr := filepath.EvalSymlinks(filepath.Dir(absolute)); dirErr == nil {
		// Resolve symlinked parent directories even when the final component
		// is a regular file, so the boundary check compares real paths.
		target = filepath.Join(directory, filepath.Base(absolute))
	}
	if isCartPath(target) {
		return "", pathError("save", b.Path, ErrUnsupportedFile)
	}

	root := b.workspaceRoot
	if root == "" {
		root = filepath.Dir(absolute)
	}
	if resolvedRoot, rootErr := filepath.EvalSymlinks(root); rootErr == nil {
		root = resolvedRoot
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		if !allowOutsideRoot {
			return "", pathError("save", b.Path, ErrOutsideRoot)
		}
	}
	return target, nil
}

// Reload replaces the buffer contents with a fresh read of Path, discarding
// unsaved changes.
func (b *Buffer) Reload() error {
	if b.Path == "" {
		return ErrNoPath
	}
	fresh, err := Open(b.Path)
	if err != nil {
		return err
	}
	b.Lines = fresh.Lines
	b.Newline = fresh.Newline
	b.TrailingNewline = fresh.TrailingNewline
	b.HadBOM = fresh.HadBOM
	b.Mode = fresh.Mode
	b.TabWidth = fresh.TabWidth
	b.disk = fresh.disk
	b.workspaceRoot = fresh.workspaceRoot
	b.Version++
	b.SavedVersion = b.Version
	b.hashValid = false
	b.indexValid = false
	b.savedHash = b.textHash()
	return nil
}
