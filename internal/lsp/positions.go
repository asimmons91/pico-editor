package lsp

import (
	"path/filepath"
	"strings"

	"go.lsp.dev/uri"

	"github.com/asimmons91/pico-editor/internal/editor"
)

// URIForPath returns the file URI used on the wire.
func URIForPath(path string) string {
	if path == "" {
		return ""
	}
	return uri.File(filepath.Clean(path)).String()
}

// PathForURI returns the local path for a file URI. It rejects non-file URIs
// and malformed values.
func PathForURI(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	u := uri.URI(raw)
	if !strings.EqualFold(u.Scheme(), "file") {
		return "", false
	}
	path := u.FsPath()
	if path == "" {
		return "", false
	}
	return filepath.Clean(path), true
}

// PositionFor converts an editor position to full-document LSP coordinates.
// baseLine is the cart's __lua__ base line (zero for standalone .lua files).
func PositionFor(buffer *editor.Buffer, baseLine int, pos editor.Pos) Position {
	if buffer == nil {
		return Position{}
	}
	pos = buffer.NormalizePos(pos)
	return Position{
		Line:      baseLine + pos.Line,
		Character: buffer.UTF16Column(pos),
	}
}

// EditorPos converts an LSP position to a code-buffer position. It returns
// false when the position lies in the cart header or a non-code section.
func EditorPos(buffer *editor.Buffer, baseLine int, pos Position) (editor.Pos, bool) {
	if buffer == nil {
		return editor.Pos{}, false
	}
	line := pos.Line - baseLine
	if line < 0 || line >= len(buffer.Lines) {
		return editor.Pos{}, false
	}
	if pos.Character < 0 {
		pos.Character = 0
	}
	return buffer.PosFromUTF16(line, pos.Character), true
}

// EditorRange converts an LSP range to cluster-boundary editor positions. The
// start expands backward and the end expands forward, so a token or diagnostic
// never splits a P8SCII cluster.
func EditorRange(buffer *editor.Buffer, baseLine int, r Range) (editor.Pos, editor.Pos, bool) {
	if buffer == nil {
		return editor.Pos{}, editor.Pos{}, false
	}
	startLine := r.Start.Line - baseLine
	endLine := r.End.Line - baseLine
	if startLine < 0 || startLine >= len(buffer.Lines) {
		return editor.Pos{}, editor.Pos{}, false
	}
	if endLine < 0 || endLine >= len(buffer.Lines) {
		return editor.Pos{}, editor.Pos{}, false
	}
	start := buffer.PosFromUTF16Start(startLine, max(0, r.Start.Character))
	end := buffer.PosFromUTF16End(endLine, max(0, r.End.Character))
	if end.Before(start) {
		return editor.Pos{}, editor.Pos{}, false
	}
	return start, end, true
}

// RangeFor returns a zero-width LSP range at pos.
func RangeFor(pos Position) Range {
	return Range{Start: pos, End: pos}
}

// CodeLineFor maps a full-document line to a code-buffer line.
func CodeLineFor(baseLine, fullLine int) int {
	return fullLine - baseLine
}

// FullLineFor maps a code-buffer line to a full-document line.
func FullLineFor(baseLine, codeLine int) int {
	return baseLine + codeLine
}
