package cart

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	p8cart "github.com/asimmons91/pico-lsp/pkg/cart"

	"github.com/asimmons91/pico-editor/internal/editor"
)

const (
	modeBits    = os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	maxCartSize = 2 * 1024 * 1024
)

// Tab is one code tab expressed in code-buffer lines. StartLine is the first
// content line, EndLine is exclusive, and MarkerLine is the hidden -->8 line
// (or -1 for the first tab).
type Tab struct {
	Index      int
	Name       string
	StartLine  int
	EndLine    int
	MarkerLine int
}

// ViewState is the per-tab cursor and viewport that survives tab switches.
type ViewState struct {
	Cursor      editor.Pos
	DesiredCell int
	TopLine     int
	LeftCell    int
}

// Document holds one open .p8 cart. Raw is the immutable file snapshot from
// open or the last successful save; Code is the only editable text.
type Document struct {
	Path     string
	Raw      []byte
	Parsed   *p8cart.Document
	Code     *editor.Buffer
	BaseLine int
	Tabs     []Tab

	workspaceRoot string
	syntheticTail bool
	disk          diskState
}

type diskState struct {
	known   bool
	size    int64
	modTime time.Time
	mode    os.FileMode
	hash    [sha256.Size]byte
}

// Open reads and parses a text .p8 cart. A cart without __lua__ is rejected
// with ErrNoCode; the caller never sees an editable buffer for non-code data.
func Open(path string) (*Document, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidPath, path, err)
	}
	absolute = filepath.Clean(absolute)
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", absolute, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("open %s: %w", absolute, errors.New("is a directory"))
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("open %s: %w", absolute, errors.New("not a regular file"))
	}
	if info.Size() > maxCartSize {
		return nil, fmt.Errorf("open %s: %w", absolute, errors.New("cart is too large"))
	}
	data, err := os.ReadFile(absolute)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", absolute, err)
	}
	parsed, err := p8cart.Parse(absolute, data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", absolute, err)
	}
	if !parsed.HasCode() {
		return nil, fmt.Errorf("%w: %s", ErrNoCode, absolute)
	}

	lines, trailing := splitCode(parsed.Code())
	code := editor.New(absolute)
	code.Lines = lines
	code.TrailingNewline = trailing
	code.Newline = "\n"
	code.HadBOM = false
	code.Mode = info.Mode() & modeBits
	code.SetWorkspaceRoot(filepath.Dir(absolute))

	document := &Document{
		Path:          absolute,
		Raw:           append([]byte(nil), data...),
		Parsed:        parsed,
		Code:          code,
		BaseLine:      parsed.CodeStartLine(),
		Tabs:          localTabs(parsed),
		workspaceRoot: filepath.Dir(absolute),
		disk: diskState{
			known:   true,
			size:    info.Size(),
			modTime: info.ModTime(),
			mode:    info.Mode() & modeBits,
			hash:    sha256.Sum256(data),
		},
	}
	document.ensureSyntheticTail()
	code.MarkSavedText(code.Text())
	return document, nil
}

// splitCode converts the cart's raw code body to LF-normalized buffer lines.
// CRLF terminators are removed; a lone CR is P8SCII glyph data and remains in
// the line text.
func splitCode(code string) ([]string, bool) {
	if code == "" {
		return []string{""}, false
	}
	lines := make([]string, 0, strings.Count(code, "\n")+1)
	start := 0
	for index := 0; index < len(code); index++ {
		if code[index] != '\n' {
			continue
		}
		end := index
		if end > start && code[end-1] == '\r' {
			end--
		}
		lines = append(lines, code[start:end])
		start = index + 1
	}
	trailing := code[len(code)-1] == '\n'
	if start < len(code) {
		lines = append(lines, code[start:])
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	return lines, trailing
}

func localTabs(parsed *p8cart.Document) []Tab {
	base := parsed.CodeStartLine()
	tabs := parsed.Tabs()
	if len(tabs) == 0 {
		return []Tab{{Index: 0, Name: "Tab 1", StartLine: 0, EndLine: 0, MarkerLine: -1}}
	}
	out := make([]Tab, 0, len(tabs))
	for _, tab := range tabs {
		out = append(out, Tab{
			Index:      tab.Index,
			Name:       tab.Name,
			StartLine:  max(0, tab.StartLine-base),
			EndLine:    max(0, tab.EndLine-base),
			MarkerLine: markerLine(tab.MarkerLine, base),
		})
	}
	return out
}

func markerLine(full, base int) int {
	if full < 0 {
		return -1
	}
	return max(0, full-base)
}

// FullText splices the current code text back into the immutable cart bytes.
func (d *Document) FullText() (string, error) {
	if d.Parsed == nil || !d.Parsed.HasCode() {
		return "", fmt.Errorf("%w: %s", ErrNoCode, d.Path)
	}
	data, err := d.Parsed.ReplaceCode(protectTrailingCR(d.codeText()))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// codeText returns the buffer text to splice. A synthetic trailing blank line
// represents an empty code tab at EOF without adding a byte to the cart.
func (d *Document) codeText() string {
	text := d.Code.Text()
	if d.syntheticTail && len(d.Code.Lines) > 0 && d.Code.Lines[len(d.Code.Lines)-1] == "" {
		text = strings.TrimSuffix(text, "\n")
	}
	return text
}

// ensureSyntheticTail gives an empty final code tab a real blank line in the
// editor buffer. The extra newline is stripped by codeText until the line is
// edited, so untouched carts still round-trip byte-for-byte.
func (d *Document) ensureSyntheticTail() {
	if len(d.Tabs) == 0 || len(d.Code.Lines) == 0 {
		d.syntheticTail = false
		return
	}
	last := &d.Tabs[len(d.Tabs)-1]
	if last.StartLine != last.EndLine {
		d.syntheticTail = false
		return
	}
	if last.StartLine < len(d.Code.Lines) {
		if d.syntheticTail && last.StartLine == len(d.Code.Lines)-1 {
			last.EndLine = last.StartLine + 1
		}
		return
	}
	line := len(d.Code.Lines) - 1
	position := editor.Pos{
		Line: line,
		Rune: utf8.RuneCountInString(d.Code.Lines[line]),
	}
	if _, err := d.Code.Apply(editor.Edit{Start: position, End: position, Text: "\n"}); err != nil {
		d.syntheticTail = false
		return
	}
	d.syntheticTail = true
	last.EndLine = last.StartLine + 1
}

// FullBytes is FullText as bytes.
func (d *Document) FullBytes() ([]byte, error) {
	text, err := d.FullText()
	if err != nil {
		return nil, err
	}
	return []byte(text), nil
}

// protectTrailingCR keeps P8SCII glyph-13 data at the end of a line distinct
// from a CRLF terminator when pkg/cart restores newline style.
func protectTrailingCR(code string) string {
	if !strings.Contains(code, "\r\n") {
		return code
	}
	return strings.ReplaceAll(code, "\r\n", "\r\r\n")
}

// SyncTabs reparses the current code body through pkg/cart so separator edits
// and line insertions/deletions update tab ranges without a second parser.
func (d *Document) SyncTabs() error {
	data, err := d.FullBytes()
	if err != nil {
		return err
	}
	parsed, err := p8cart.Parse(d.Path, data)
	if err != nil {
		return err
	}
	d.Tabs = localTabs(parsed)
	d.ensureSyntheticTail()
	return nil
}

// Dirty reports whether the code buffer differs from the last saved code.
func (d *Document) Dirty() bool {
	return d.Code.Dirty()
}

// SetWorkspaceRoot widens the save boundary from the cart's directory to the
// project root.
func (d *Document) SetWorkspaceRoot(root string) {
	if root == "" {
		return
	}
	d.workspaceRoot = root
	d.Code.SetWorkspaceRoot(root)
}

// CheckExternal compares the file to the state recorded at open/last save.
func (d *Document) CheckExternal() error {
	info, err := os.Stat(d.Path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrDeleted, d.Path)
		}
		return fmt.Errorf("%w: %s: %v", ErrExternalChange, d.Path, err)
	}
	if info.Size() != d.disk.size || !info.ModTime().Equal(d.disk.modTime) ||
		info.Mode()&modeBits != d.disk.mode {
		return fmt.Errorf("%w: %s", ErrExternalChange, d.Path)
	}
	data, err := os.ReadFile(d.Path)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrExternalChange, d.Path, err)
	}
	if sha256.Sum256(data) != d.disk.hash {
		return fmt.Errorf("%w: %s", ErrExternalChange, d.Path)
	}
	return nil
}
