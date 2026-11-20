package editor

import (
	"strings"
	"unicode/utf8"
)

// SelectionKind is the shape of a selection. Blockwise selection is added in
// M3.
type SelectionKind uint8

const (
	// SelectionNone means no selection is active.
	SelectionNone SelectionKind = iota
	// SelectionChar is a characterwise range with inclusive endpoints.
	SelectionChar
	// SelectionLine is a linewise range.
	SelectionLine
	// SelectionBlock is a rectangular cell range. Its endpoints are inclusive
	// cells, and block edges never split a wide cluster.
	SelectionBlock
)

// Selection is an anchor and cursor pair. Normalized endpoints are inclusive
// for characterwise selections; linewise operations use LineRange.
type Selection struct {
	Anchor Pos
	Cursor Pos
	Kind   SelectionKind
}

// Empty reports whether no selection is active. A one-cluster character
// selection and a one-line linewise selection are not empty.
func (s Selection) Empty() bool {
	return s.Kind == SelectionNone
}

// Normalized returns the endpoints in buffer order. The returned endpoints are
// inclusive.
func (s Selection) Normalized() (start, end Pos) {
	if s.Cursor.Before(s.Anchor) {
		return s.Cursor, s.Anchor
	}
	return s.Anchor, s.Cursor
}

// LineRange returns the inclusive first and last lines of the selection.
func (s Selection) LineRange() (first, last int) {
	start, end := s.Normalized()
	return start.Line, end.Line
}

// BlockRange returns the inclusive line and cell bounds of a block selection.
// The cell bounds are normalized to display cells, not rune offsets.
func (s Selection) BlockRange(buffer *Buffer) (first, last, left, right int) {
	first, last = s.LineRange()
	anchorCell := buffer.CellColumn(s.Anchor)
	cursorCell := buffer.CellColumn(s.Cursor)
	anchorCells := buffer.ClusterAt(s.Anchor).Cells
	cursorCells := buffer.ClusterAt(s.Cursor).Cells
	if anchorCells < 1 {
		anchorCells = 1
	}
	if cursorCells < 1 {
		cursorCells = 1
	}
	left = min(anchorCell, cursorCell)
	right = max(anchorCell+anchorCells-1, cursorCell+cursorCells-1)
	return first, last, left, right
}

// BlockLineRange returns the cluster-aligned half-open range covered by the
// block rectangle on line. It reports false when the line has no cluster in the
// rectangle. Clusters that straddle either edge are included whole.
func (s Selection) BlockLineRange(buffer *Buffer, line int) (start, end Pos, ok bool) {
	if s.Kind != SelectionBlock {
		return Pos{}, Pos{}, false
	}
	first, last, left, right := s.BlockRange(buffer)
	if line < first || line > last || line < 0 || line >= len(buffer.Lines) {
		return Pos{}, Pos{}, false
	}
	text := buffer.Lines[line]
	for cluster := range LineClusters(text, buffer.TabWidth) {
		if cluster.Cells == 0 {
			continue
		}
		if cluster.Cell+cluster.Cells-1 < left || cluster.Cell > right {
			continue
		}
		if !ok {
			start = Pos{Line: line, Rune: cluster.Rune}
			ok = true
		}
		end = Pos{Line: line, Rune: cluster.Rune + utf8.RuneCountInString(cluster.Text)}
	}
	return start, end, ok
}

// StartSelection begins a selection at the cursor.
func (e *Editor) StartSelection(kind SelectionKind) {
	e.BreakGroup()
	e.Selection = Selection{
		Anchor: e.Cursor.Pos,
		Cursor: e.Cursor.Pos,
		Kind:   kind,
	}
}

// ExtendSelection moves the selection endpoint to the cursor after a motion.
func (e *Editor) ExtendSelection() {
	if e.Selection.Kind == SelectionNone {
		return
	}
	e.Selection.Cursor = e.Cursor.Pos
}

// ClearSelection removes the selection without moving the cursor.
func (e *Editor) ClearSelection() {
	e.Selection = Selection{}
}

// SelectAll selects the whole buffer characterwise and moves the cursor to the
// selection end, keeping Cursor and Selection.Cursor consistent.
func (e *Editor) SelectAll() {
	e.BreakGroup()
	first := e.WindowFirst()
	last := e.WindowLast()
	if e.lineWindow.active && e.WindowEmpty() {
		e.Selection = Selection{Anchor: Pos{Line: first}, Cursor: Pos{Line: first}, Kind: SelectionChar}
		e.setCursor(Pos{Line: first}, true)
		e.EnsureVisible()
		return
	}
	end := Pos{Line: last, Rune: utf8.RuneCountInString(e.Buffer.Lines[last])}
	e.Selection = Selection{
		Anchor: Pos{Line: first},
		Cursor: end,
		Kind:   SelectionChar,
	}
	e.setCursor(end, true)
	e.EnsureVisible()
}

// SelectedText returns the selected text. Linewise selections include a
// trailing newline, matching register semantics.
func (e *Editor) SelectedText() (string, bool) {
	if e.Selection.Empty() {
		return "", false
	}
	if e.Selection.Kind == SelectionLine {
		first, last := e.Selection.LineRange()
		text, err := e.LineRangeText(first, last)
		if err != nil {
			return "", false
		}
		return text, true
	}
	if e.Selection.Kind == SelectionBlock {
		first, last, left, right := e.Selection.BlockRange(e.Buffer)
		width := right - left + 1
		lines := make([]string, 0, last-first+1)
		for line := first; line <= last; line++ {
			start, end, ok := e.Selection.BlockLineRange(e.Buffer, line)
			if !ok {
				lines = append(lines, strings.Repeat(" ", width))
				continue
			}
			text, err := e.Buffer.TextInRange(start, end)
			if err != nil {
				return "", false
			}
			if cells := e.Buffer.Info(end).Cell - e.Buffer.Info(start).Cell; cells < width {
				text += strings.Repeat(" ", width-cells)
			}
			lines = append(lines, text)
		}
		return strings.Join(lines, "\n"), true
	}
	start, end, ok := e.characterwiseRange()
	if !ok {
		return "", false
	}
	text, err := e.Buffer.TextInRange(start, end)
	if err != nil {
		return "", false
	}
	return text, true
}

// characterwiseRange converts an inclusive selection to a half-open edit range.
func (e *Editor) characterwiseRange() (start, end Pos, ok bool) {
	start, inclusiveEnd := e.Selection.Normalized()
	if e.lineWindow.active {
		if e.WindowEmpty() {
			return Pos{}, Pos{}, false
		}
		start = e.clampToWindow(start)
		inclusiveEnd = e.clampToWindow(inclusiveEnd)
	}
	if start.Line < 0 || start.Line >= len(e.Buffer.Lines) {
		return Pos{}, Pos{}, false
	}
	if inclusiveEnd.Line < 0 || inclusiveEnd.Line >= len(e.Buffer.Lines) {
		return Pos{}, Pos{}, false
	}
	line := e.Buffer.Lines[inclusiveEnd.Line]
	if inclusiveEnd.Rune < utf8.RuneCountInString(line) {
		inclusiveEnd.Rune = nextClusterBoundary(line, inclusiveEnd.Rune)
	}
	return start, inclusiveEnd, true
}
