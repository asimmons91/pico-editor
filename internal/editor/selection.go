package editor

import "unicode/utf8"

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
	last := len(e.Buffer.Lines) - 1
	end := Pos{Line: last, Rune: utf8.RuneCountInString(e.Buffer.Lines[last])}
	e.Selection = Selection{
		Anchor: Pos{},
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
