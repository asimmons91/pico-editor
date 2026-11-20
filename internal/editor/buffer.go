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
	hash    [sha256.Size]byte
}

// Open reads an existing text file. Cart files are rejected until the M4 cart
// adapter can preserve non-code sections.
func Open(path string) (*Buffer, error) {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".p8") || strings.HasSuffix(lower, ".p8.png") {
		return nil, pathError("open", path, ErrUnsupportedFile)
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, pathError("open", path, err)
	}
	if info.IsDir() {
		return nil, pathError("open", path, errors.New("is a directory"))
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
		Mode:            info.Mode().Perm(),
		TabWidth:        2,
		workspaceRoot:   filepath.Dir(absolute),
		disk: diskState{
			known:   true,
			size:    info.Size(),
			modTime: info.ModTime(),
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

// MarkSaved records the current text as the save point.
func (b *Buffer) MarkSaved() {
	b.savedHash = b.textHash()
	b.SavedVersion = b.Version
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
// changed since open/last save and refuses read-only files and symlink targets
// outside the M2 workspace root (the file's parent directory).
func (b *Buffer) Save() error {
	if b.Path == "" {
		return ErrNoPath
	}
	if b.disk.known {
		if err := b.checkExternal(); err != nil {
			return err
		}
	}
	if b.Mode.Perm()&0o222 == 0 {
		return pathError("save", b.Path, ErrReadOnly)
	}

	target, err := b.saveTarget()
	if err != nil {
		return err
	}
	data := b.encode()
	if err := fsutil.AtomicWrite(target, data, b.Mode); err != nil {
		return pathError("save", b.Path, err)
	}

	b.MarkSaved()
	if info, statErr := os.Stat(target); statErr == nil {
		b.disk = diskState{
			known:   true,
			size:    info.Size(),
			modTime: info.ModTime(),
			hash:    sha256.Sum256(data),
		}
		b.Mode = info.Mode().Perm()
	} else {
		b.disk.known = false
	}
	return nil
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
	if info.Size() != b.disk.size || !info.ModTime().Equal(b.disk.modTime) {
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

// saveTarget resolves symlinks and enforces the M2 workspace boundary. The
// workspace root is the directory containing the buffer path.
func (b *Buffer) saveTarget() (string, error) {
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

	root := b.workspaceRoot
	if root == "" {
		root = filepath.Dir(absolute)
	}
	if resolvedRoot, rootErr := filepath.EvalSymlinks(root); rootErr == nil {
		root = resolvedRoot
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", pathError("save", b.Path, ErrOutsideRoot)
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
