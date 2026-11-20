package editor

import "strings"

// An active line range restricts cursor motion and edits to one cart code tab.
// The range is half-open: First is included and End is excluded. An empty
// range represents a cart tab with no content lines; the editor keeps a
// virtual cursor at First and materializes a blank line on the first edit.
type lineWindow struct {
	active bool
	first  int
	end    int
}

// SetLineRange restricts the editor to lines [first, end). Callers pass the
// active cart tab's content range. An empty range (first == end) is valid and
// represents an empty tab.
func (e *Editor) SetLineRange(first, end int) {
	e.lineWindow.active = true
	e.lineWindow.first = first
	e.lineWindow.end = end
	e.normalizeLineRange()
	e.setCursor(e.Cursor.Pos, true)
	e.EnsureVisible()
}

// ClearLineRange removes the restriction and leaves the cursor in place.
func (e *Editor) ClearLineRange() {
	e.lineWindow = lineWindow{}
	e.setCursor(e.Cursor.Pos, true)
	e.EnsureVisible()
}

// LineRange returns the active half-open line range.
func (e *Editor) LineRange() (first, end int, active bool) {
	if !e.lineWindow.active {
		return 0, 0, false
	}
	return e.lineWindow.first, e.lineWindow.end, true
}

// WindowEmpty reports whether the active range has no content lines.
func (e *Editor) WindowEmpty() bool {
	return e.lineWindow.active && e.lineWindow.end <= e.lineWindow.first
}

// WindowFirst returns the first line available to the cursor.
func (e *Editor) WindowFirst() int {
	if e.lineWindow.active {
		return e.lineWindow.first
	}
	return 0
}

// WindowLast returns the last line available to the cursor, inclusive. For an
// empty range it returns the virtual line at the range start.
func (e *Editor) WindowLast() int {
	if !e.lineWindow.active {
		return len(e.Buffer.Lines) - 1
	}
	if e.WindowEmpty() {
		return e.lineWindow.first
	}
	return e.lineWindow.end - 1
}

// HasLine reports whether line is inside the active range. Empty ranges have no
// content lines.
func (e *Editor) HasLine(line int) bool {
	if !e.lineWindow.active {
		return line >= 0 && line < len(e.Buffer.Lines)
	}
	if e.WindowEmpty() {
		return false
	}
	return line >= e.lineWindow.first && line < e.lineWindow.end
}

func (e *Editor) normalizeLineRange() {
	if !e.lineWindow.active {
		return
	}
	if len(e.Buffer.Lines) == 0 {
		e.lineWindow.first = 0
		e.lineWindow.end = 0
		return
	}
	if e.lineWindow.first < 0 {
		e.lineWindow.first = 0
	}
	if e.lineWindow.first > len(e.Buffer.Lines)-1 {
		e.lineWindow.first = len(e.Buffer.Lines) - 1
	}
	if e.lineWindow.end < e.lineWindow.first {
		e.lineWindow.end = e.lineWindow.first
	}
	if e.lineWindow.end > len(e.Buffer.Lines) {
		e.lineWindow.end = len(e.Buffer.Lines)
	}
}

func (e *Editor) clampToWindow(p Pos) Pos {
	p = e.Buffer.NormalizePos(p)
	if !e.lineWindow.active {
		return p
	}
	if e.WindowEmpty() {
		return Pos{Line: e.lineWindow.first}
	}
	if p.Line < e.lineWindow.first {
		return Pos{Line: e.lineWindow.first}
	}
	if p.Line >= e.lineWindow.end {
		line := e.lineWindow.end - 1
		return Pos{Line: line, Rune: runeCount(e.Buffer.Lines[line])}
	}
	return p
}

// clampLine limits a line number to the active tab. With no active range it
// limits only to the buffer.
func (e *Editor) clampLine(line int) int {
	if e.lineWindow.active {
		return min(max(line, e.lineWindow.first), e.WindowLast())
	}
	return min(max(line, 0), len(e.Buffer.Lines)-1)
}

// editInWindow reports whether an edit stays inside the active tab. An edit
// may end exactly at the start of the separator line.
func (e *Editor) editInWindow(edit Edit) bool {
	if !e.lineWindow.active {
		return true
	}
	if e.WindowEmpty() {
		return false
	}
	if edit.Start.Line < e.lineWindow.first || edit.Start.Line >= e.lineWindow.end {
		return false
	}
	if edit.End.Line < edit.Start.Line || edit.End.Line > e.lineWindow.end {
		return false
	}
	if edit.End.Line == e.lineWindow.end && edit.End.Rune != 0 {
		return false
	}
	return true
}

func (e *Editor) shiftLineRange(delta int) {
	if !e.lineWindow.active || delta == 0 {
		return
	}
	e.lineWindow.end += delta
	e.normalizeLineRange()
}

func runeCount(text string) int {
	return len([]rune(text))
}

// materializeEmptyWindow turns the virtual cursor line of an empty tab into a
// real blank line. It returns the adjusted edit and whether the edit itself
// should be skipped because the blank line already satisfies it.
func (e *Editor) materializeEmptyWindow(edit Edit) (Edit, bool, Change, error) {
	position := Pos{Line: e.lineWindow.first}
	change, err := e.Buffer.Apply(Edit{Start: position, End: position, Text: "\n"})
	if err != nil {
		return edit, false, Change{}, err
	}
	change.Before = e.Cursor.Pos
	change.After = position
	e.record(change)
	e.lineWindow.end = e.lineWindow.first + 1
	e.normalizeLineRange()
	e.setCursor(position, true)

	// Positions built from the hidden marker line refer to the virtual blank
	// line that now occupies the range start.
	if edit.Start.Line == e.lineWindow.first {
		edit.Start.Rune = 0
	}
	if edit.End.Line == e.lineWindow.first {
		edit.End.Rune = 0
	}
	skip := edit.Start == position && edit.End == position && edit.Text == "\n"
	return edit, skip, change, nil
}

func editLineDelta(edit Edit) int {
	return strings.Count(edit.Text, "\n") - (edit.End.Line - edit.Start.Line)
}
