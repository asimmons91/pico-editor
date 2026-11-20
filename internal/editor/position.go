package editor

import (
	"iter"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/asimmons91/pico-lsp/pkg/p8scii"
)

// Pos is a zero-based line and rune offset within that line. A valid cursor
// position always sits on a P8SCII cluster boundary.
type Pos struct {
	Line int
	Rune int
}

// Compare returns -1, 0, or 1 when p is before, equal to, or after other.
func (p Pos) Compare(other Pos) int {
	switch {
	case p.Line < other.Line:
		return -1
	case p.Line > other.Line:
		return 1
	case p.Rune < other.Rune:
		return -1
	case p.Rune > other.Rune:
		return 1
	default:
		return 0
	}
}

// Before reports whether p sorts before other.
func (p Pos) Before(other Pos) bool {
	return p.Compare(other) < 0
}

// PosInfo is a position expressed in every coordinate system the editor uses.
type PosInfo struct {
	Pos   Pos
	Byte  int // byte offset in Buffer.Text()
	Cell  int // display column in 4px cells
	UTF16 int // UTF-16 code units from the start of the line
}

// LineCluster is one display unit of a line. Text is the exact source text,
// including a trailing U+FE0F when present.
type LineCluster struct {
	Text   string
	Rune   int // rune offset of the cluster start
	Cells  int // display cells advanced
	Pixels int // display pixels advanced at 1x
	Cell   int // starting display cell
	Glyph  p8scii.Glyph
	Known  bool
	Tab    bool
}

// LineClusters iterates the display clusters in one line. Tabs advance to the
// next multiple of tabWidth; P8SCII control glyphs advance zero cells; unknown
// clusters advance one 4px cell.
func LineClusters(line string, tabWidth int) iter.Seq[LineCluster] {
	if tabWidth < 1 {
		tabWidth = 1
	}
	return func(yield func(LineCluster) bool) {
		cell := 0
		runeIndex := 0
		for i := 0; i < len(line); {
			r, size := utf8.DecodeRuneInString(line[i:])
			end := i + size
			runes := 1
			if r != '\ufe0f' && (r != utf8.RuneError || size != 1) {
				if next, nextSize := utf8.DecodeRuneInString(line[end:]); next == '\ufe0f' {
					end += nextSize
					runes++
				}
			}
			text := line[i:end]

			cluster := LineCluster{
				Text: text,
				Rune: runeIndex,
				Cell: cell,
			}
			switch {
			case r == '\t':
				cluster.Tab = true
				cluster.Cells = tabWidth - cell%tabWidth
				cluster.Pixels = cluster.Cells * narrowWidth
			case r == utf8.RuneError && size == 1:
				cluster.Cells = 1
				cluster.Pixels = narrowWidth
			default:
				meta, known := p8scii.GlyphFor(text)
				cluster.Glyph = meta
				cluster.Known = known
				if known {
					cluster.Pixels = meta.Width
					cluster.Cells = meta.Width / narrowWidth
				} else {
					cluster.Pixels = narrowWidth
					cluster.Cells = 1
				}
			}

			if !yield(cluster) {
				return
			}
			cell += cluster.Cells
			runeIndex += runes
			i = end
		}
	}
}

const narrowWidth = 4

// clusterEnd returns the rune offset just after the cluster starting at start.
func clusterEnd(line string, start int) int {
	if start < 0 || start >= len(line) {
		return start
	}
	byteStart := byteIndexForRune(line, start)
	if byteStart >= len(line) {
		return start
	}
	r, size := utf8.DecodeRuneInString(line[byteStart:])
	end := byteStart + size
	if r != '\ufe0f' && (r != utf8.RuneError || size != 1) {
		if next, nextSize := utf8.DecodeRuneInString(line[end:]); next == '\ufe0f' {
			end += nextSize
		}
	}
	return start + utf8.RuneCountInString(line[byteStart:end])
}

// clusterStart returns the rune offset of the cluster containing runeOffset.
func clusterStart(line string, runeOffset int) int {
	if runeOffset <= 0 {
		return 0
	}
	if runeOffset >= utf8.RuneCountInString(line) {
		return utf8.RuneCountInString(line)
	}
	for start := 0; start < utf8.RuneCountInString(line); start = clusterEnd(line, start) {
		end := clusterEnd(line, start)
		if runeOffset < end {
			return start
		}
	}
	return utf8.RuneCountInString(line)
}

func snapNearest(line string, runeOffset int) int {
	length := utf8.RuneCountInString(line)
	if runeOffset <= 0 {
		return 0
	}
	if runeOffset >= length {
		return length
	}
	for start := 0; start < length; start = clusterEnd(line, start) {
		end := clusterEnd(line, start)
		if runeOffset == start || runeOffset == end {
			return runeOffset
		}
		if runeOffset < end {
			if runeOffset-start <= end-runeOffset {
				return start
			}
			return end
		}
	}
	return length
}

func isClusterBoundary(line string, runeOffset int) bool {
	length := utf8.RuneCountInString(line)
	if runeOffset < 0 || runeOffset > length {
		return false
	}
	if runeOffset == 0 || runeOffset == length {
		return true
	}
	return clusterStart(line, runeOffset) == runeOffset
}

func nextClusterBoundary(line string, runeOffset int) int {
	if runeOffset >= utf8.RuneCountInString(line) {
		return utf8.RuneCountInString(line)
	}
	return clusterEnd(line, runeOffset)
}

func prevClusterBoundary(line string, runeOffset int) int {
	if runeOffset <= 0 {
		return 0
	}
	if runeOffset > utf8.RuneCountInString(line) {
		runeOffset = utf8.RuneCountInString(line)
	}
	return clusterStart(line, runeOffset-1)
}

func byteIndexForRune(line string, runeOffset int) int {
	if runeOffset <= 0 {
		return 0
	}
	index := 0
	seen := 0
	for index < len(line) && seen < runeOffset {
		_, size := utf8.DecodeRuneInString(line[index:])
		index += size
		seen++
	}
	return index
}

func cellColumn(line string, runeOffset, tabWidth int) int {
	cell := 0
	for cluster := range LineClusters(line, tabWidth) {
		if cluster.Rune >= runeOffset {
			return cluster.Cell
		}
		cell = cluster.Cell + cluster.Cells
	}
	return cell
}

func runeAtCell(line string, cell, tabWidth int) int {
	last := 0
	for cluster := range LineClusters(line, tabWidth) {
		if cluster.Cells > 0 && cell < cluster.Cell+cluster.Cells {
			return cluster.Rune
		}
		last = cluster.Rune + utf8.RuneCountInString(cluster.Text)
	}
	return last
}

func utf16Column(line string, runeOffset int) int {
	units := 0
	index := 0
	for _, r := range line {
		if index >= runeOffset {
			break
		}
		size := utf16.RuneLen(r)
		if size < 0 {
			size = 1
		}
		units += size
		index++
	}
	return units
}

func runeFromUTF16(line string, column int) int {
	if column <= 0 {
		return 0
	}
	units := 0
	index := 0
	for _, r := range line {
		size := utf16.RuneLen(r)
		if size < 0 {
			size = 1
		}
		if units+size > column {
			return index
		}
		units += size
		index++
		if units == column {
			return index
		}
	}
	return index
}

// textIndex maps line/rune positions to byte offsets for an arbitrary text
// value. It is used by search, which runs against a text snapshot.
type textIndex struct {
	text   string
	starts []int
}

func newTextIndex(text string) *textIndex {
	starts := []int{0}
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return &textIndex{text: text, starts: starts}
}

func (x *textIndex) lineText(line int) string {
	if line < 0 || line >= len(x.starts) {
		return ""
	}
	start := x.starts[line]
	end := len(x.text)
	if line+1 < len(x.starts) {
		end = x.starts[line+1] - 1 // omit the newline
	}
	return x.text[start:end]
}

func (x *textIndex) posToByte(p Pos) (int, error) {
	if p.Line < 0 || p.Line >= len(x.starts) {
		return 0, ErrInvalidPosition
	}
	line := x.lineText(p.Line)
	if p.Rune < 0 || p.Rune > utf8.RuneCountInString(line) || !isClusterBoundary(line, p.Rune) {
		return 0, ErrInvalidPosition
	}
	return x.starts[p.Line] + byteIndexForRune(line, p.Rune), nil
}

func (x *textIndex) byteToPos(offset int) (Pos, error) {
	if offset < 0 || offset > len(x.text) {
		return Pos{}, ErrInvalidPosition
	}
	if offset == len(x.text) {
		line := len(x.starts) - 1
		return Pos{Line: line, Rune: utf8.RuneCountInString(x.lineText(line))}, nil
	}
	line := 0
	for line+1 < len(x.starts) && x.starts[line+1] <= offset {
		line++
	}
	lineStart := x.starts[line]
	lineEnd := len(x.text)
	if line+1 < len(x.starts) {
		lineEnd = x.starts[line+1] - 1
	}
	if offset > lineEnd {
		return Pos{}, ErrInvalidPosition
	}
	lineText := x.text[lineStart:lineEnd]
	if !utf8.RuneStart(x.text[offset]) {
		// Offsets inside a rune are not valid positions.
		return Pos{}, ErrInvalidPosition
	}
	return Pos{Line: line, Rune: utf8.RuneCountInString(lineText[:offset-lineStart])}, nil
}

// normalizePos clamps p to the buffer and snaps it to the nearest cluster
// boundary.
func (b *Buffer) normalizePos(p Pos) Pos {
	if len(b.Lines) == 0 {
		return Pos{}
	}
	if p.Line < 0 {
		p.Line = 0
	}
	if p.Line >= len(b.Lines) {
		p.Line = len(b.Lines) - 1
	}
	line := b.Lines[p.Line]
	length := utf8.RuneCountInString(line)
	if p.Rune < 0 {
		p.Rune = 0
	}
	if p.Rune > length {
		p.Rune = length
	}
	p.Rune = snapNearest(line, p.Rune)
	return p
}

func (b *Buffer) validatePos(p Pos) error {
	if err := b.validateRunePos(p); err != nil {
		return err
	}
	line := b.Lines[p.Line]
	if !isClusterBoundary(line, p.Rune) {
		return ErrInvalidPosition
	}
	return nil
}

func (b *Buffer) validateRunePos(p Pos) error {
	if p.Line < 0 || p.Line >= len(b.Lines) {
		return ErrInvalidPosition
	}
	line := b.Lines[p.Line]
	if p.Rune < 0 || p.Rune > utf8.RuneCountInString(line) {
		return ErrInvalidPosition
	}
	return nil
}

// NormalizePos clamps p to the buffer and snaps it to the nearest cluster
// boundary.
func (b *Buffer) NormalizePos(p Pos) Pos {
	return b.normalizePos(p)
}

// Info converts p into byte, cell, and UTF-16 coordinates.
func (b *Buffer) Info(p Pos) PosInfo {
	p = b.normalizePos(p)
	start := b.lineStart(p.Line)
	return PosInfo{
		Pos:   p,
		Byte:  start + byteIndexForRune(b.Lines[p.Line], p.Rune),
		Cell:  cellColumn(b.Lines[p.Line], p.Rune, b.TabWidth),
		UTF16: utf16Column(b.Lines[p.Line], p.Rune),
	}
}

// ByteOffset returns the byte offset of p in Buffer.Text().
func (b *Buffer) ByteOffset(p Pos) int {
	return b.Info(p).Byte
}

// CellColumn returns the display column of p in 4px cells.
func (b *Buffer) CellColumn(p Pos) int {
	return b.Info(p).Cell
}

// UTF16Column returns the UTF-16 code unit offset of p from its line start.
func (b *Buffer) UTF16Column(p Pos) int {
	return b.Info(p).UTF16
}

// PosFromByte converts a byte offset in Buffer.Text() to a position. The offset
// must be on a rune boundary; the result snaps to the containing cluster. An
// offset at or after the implicit trailing newline maps to the last line's end,
// because the trailing terminator is represented by TrailingNewline rather than
// an extra line.
func (b *Buffer) PosFromByte(offset int) (Pos, error) {
	index := newTextIndex(b.Text())
	p, err := index.byteToPos(offset)
	if err != nil {
		return Pos{}, err
	}
	if p.Line >= len(b.Lines) {
		p.Line = len(b.Lines) - 1
		p.Rune = utf8.RuneCountInString(b.Lines[p.Line])
	}
	return b.normalizePos(p), nil
}

// PosFromUTF16 converts a UTF-16 column on line to a cluster-boundary
// position. Columns inside a surrogate pair or cluster snap to the boundary.
func (b *Buffer) PosFromUTF16(line, column int) Pos {
	if len(b.Lines) == 0 {
		return Pos{}
	}
	if line < 0 {
		line = 0
	}
	if line >= len(b.Lines) {
		line = len(b.Lines) - 1
	}
	runeOffset := runeFromUTF16(b.Lines[line], column)
	return b.normalizePos(Pos{Line: line, Rune: runeOffset})
}

// PosFromCell converts a display cell on line to a cluster-boundary position.
// Cells inside a wide glyph or tab snap to the start of that cluster.
func (b *Buffer) PosFromCell(line, cell int) Pos {
	if len(b.Lines) == 0 {
		return Pos{}
	}
	if line < 0 {
		line = 0
	}
	if line >= len(b.Lines) {
		line = len(b.Lines) - 1
	}
	if cell < 0 {
		cell = 0
	}
	return b.normalizePos(Pos{Line: line, Rune: runeAtCell(b.Lines[line], cell, b.TabWidth)})
}

// NextCluster returns the position immediately after p, crossing line
// boundaries. It returns p at the end of the buffer.
func (b *Buffer) NextCluster(p Pos) Pos {
	p = b.normalizePos(p)
	if p.Rune < utf8.RuneCountInString(b.Lines[p.Line]) {
		return Pos{Line: p.Line, Rune: nextClusterBoundary(b.Lines[p.Line], p.Rune)}
	}
	if p.Line+1 < len(b.Lines) {
		return Pos{Line: p.Line + 1}
	}
	return p
}

// PrevCluster returns the position immediately before p, crossing line
// boundaries. It returns p at the start of the buffer.
func (b *Buffer) PrevCluster(p Pos) Pos {
	p = b.normalizePos(p)
	if p.Rune > 0 {
		return Pos{Line: p.Line, Rune: prevClusterBoundary(b.Lines[p.Line], p.Rune)}
	}
	if p.Line > 0 {
		return Pos{Line: p.Line - 1, Rune: utf8.RuneCountInString(b.Lines[p.Line-1])}
	}
	return p
}

// ClusterAt returns the display cluster starting at p. It returns a zero
// cluster at end of line.
func (b *Buffer) ClusterAt(p Pos) LineCluster {
	p = b.normalizePos(p)
	for cluster := range LineClusters(b.Lines[p.Line], b.TabWidth) {
		if cluster.Rune == p.Rune {
			return cluster
		}
		if cluster.Rune > p.Rune {
			break
		}
	}
	return LineCluster{Rune: p.Rune}
}

// PosAfter returns the position after inserting canonical LF text at start. It
// uses rune counts, not cells, and does not clamp to the current buffer because
// the insertion may grow the line or add lines. Callers must normalize CRLF
// exactly once before calling; Buffer.Apply expects canonical text.
func (b *Buffer) PosAfter(start Pos, text string) Pos {
	lines := strings.Split(text, "\n")
	if len(lines) == 1 {
		return Pos{Line: start.Line, Rune: start.Rune + utf8.RuneCountInString(text)}
	}
	return Pos{
		Line: start.Line + len(lines) - 1,
		Rune: utf8.RuneCountInString(lines[len(lines)-1]),
	}
}

// lineStart returns the cached byte offset of the first character of line.
func (b *Buffer) lineStart(line int) int {
	if b.indexValid && line >= 0 && line < len(b.lineStarts) {
		return b.lineStarts[line]
	}
	starts := make([]int, len(b.Lines))
	offset := 0
	for i, text := range b.Lines {
		starts[i] = offset
		offset += len(text) + 1
	}
	b.lineStarts = starts
	b.indexValid = true
	if line < 0 || line >= len(starts) {
		return 0
	}
	return starts[line]
}

// TextInRange returns the canonical text between cluster-boundary positions.
func (b *Buffer) TextInRange(start, end Pos) (string, error) {
	return b.textInRange(start, end, true)
}

func (b *Buffer) textInRange(start, end Pos, requireBoundaries bool) (string, error) {
	if requireBoundaries {
		if err := b.validatePos(start); err != nil {
			return "", err
		}
		if err := b.validatePos(end); err != nil {
			return "", err
		}
	} else {
		if err := b.validateRunePos(start); err != nil {
			return "", err
		}
		if err := b.validateRunePos(end); err != nil {
			return "", err
		}
	}
	if end.Before(start) {
		return "", ErrInvalidRange
	}
	if start.Line == end.Line {
		line := b.Lines[start.Line]
		return line[byteIndexForRune(line, start.Rune):byteIndexForRune(line, end.Rune)], nil
	}
	var out strings.Builder
	first := b.Lines[start.Line]
	out.WriteString(first[byteIndexForRune(first, start.Rune):])
	out.WriteByte('\n')
	for line := start.Line + 1; line < end.Line; line++ {
		out.WriteString(b.Lines[line])
		out.WriteByte('\n')
	}
	last := b.Lines[end.Line]
	out.WriteString(last[:byteIndexForRune(last, end.Rune)])
	return out.String(), nil
}
