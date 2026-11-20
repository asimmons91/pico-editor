package editor

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// insertGroupTimeout ends an insert undo group after this much idle time. M3
// also ends groups on Esc, mode changes, and motions.
const insertGroupTimeout = 500 * time.Millisecond

// Editor is the only mutable editing state: a buffer plus cursor, selection,
// viewport, undo history, and search configuration.
type Editor struct {
	Buffer    *Buffer
	Cursor    Cursor
	Selection Selection
	Viewport  Viewport
	Search    Search
	History   UndoStack

	now   func() time.Time
	group editGroup
}

type editGroup struct {
	active bool
	merge  bool
	last   time.Time
}

// NewEditor returns an editor for buffer. A nil buffer is replaced with an
// empty unnamed buffer.
func NewEditor(buffer *Buffer) *Editor {
	if buffer == nil {
		buffer = New("")
	}
	if buffer.TabWidth < 1 {
		buffer.TabWidth = 2
	}
	editor := &Editor{
		Buffer:   buffer,
		Viewport: Viewport{Width: 64, Height: 40, ScrollOff: 2},
		History:  UndoStack{Limit: DefaultUndoLimit},
		now:      time.Now,
	}
	editor.Cursor.Pos = buffer.NormalizePos(editor.Cursor.Pos)
	editor.Cursor.DesiredCell = buffer.CellColumn(editor.Cursor.Pos)
	editor.EnsureVisible()
	return editor
}

func (e *Editor) currentTime() time.Time {
	if e.now == nil {
		return time.Now()
	}
	return e.now()
}

// BeginGroup starts coalescing consecutive edits into one undo entry. Calling
// BeginGroup while a group is already open is a no-op.
func (e *Editor) BeginGroup() {
	if e.group.active {
		return
	}
	e.group.active = true
	e.group.merge = false
	e.group.last = e.currentTime()
}

// EndGroup closes the current undo group.
func (e *Editor) EndGroup() {
	e.group.active = false
	e.group.merge = false
}

// BreakGroup closes any open undo group. Motions, mode changes, and non-insert
// commands call this boundary.
func (e *Editor) BreakGroup() {
	e.EndGroup()
}

func (e *Editor) record(change Change) {
	if change.Empty() {
		return
	}
	now := e.currentTime()
	if e.group.active {
		if now.Sub(e.group.last) > insertGroupTimeout {
			e.group.merge = false
		}
		if e.group.merge && len(e.History.Undo) > 0 {
			if merged, ok := mergeChange(e.History.Undo[len(e.History.Undo)-1], change); ok {
				if merged.Empty() {
					// An insert followed by a backspace can net to nothing;
					// drop the group entry instead of keeping a no-op undo.
					e.History.Undo = e.History.Undo[:len(e.History.Undo)-1]
					e.group.merge = false
					e.group.last = now
					return
				}
				e.History.ReplaceLast(merged)
				e.group.last = now
				return
			}
		}
	}
	e.History.Push(change)
	e.group.merge = e.group.active
	e.group.last = now
}

func (e *Editor) setCursor(p Pos, updateDesired bool) {
	p = e.Buffer.NormalizePos(p)
	e.Cursor.Pos = p
	if updateDesired {
		e.Cursor.DesiredCell = e.Buffer.CellColumn(p)
	}
}

// SetCursor moves the cursor, snapping to a cluster boundary. A cursor motion
// ends the current insert undo group.
func (e *Editor) SetCursor(p Pos) {
	e.BreakGroup()
	e.setCursor(p, true)
	e.EnsureVisible()
}

// SetPos is an alias for SetCursor used by the position API described in the
// M2 spec.
func (e *Editor) SetPos(p Pos) {
	e.SetCursor(p)
}

// EnsureVisible scrolls the viewport so the cursor and its cluster are visible.
func (e *Editor) EnsureVisible() {
	pos := e.Buffer.NormalizePos(e.Cursor.Pos)
	info := e.Buffer.Info(pos)
	cells := e.Buffer.ClusterAt(pos).Cells
	if cells < 1 {
		cells = 1
	}
	e.Viewport.EnsureVisible(pos, info.Cell, cells, len(e.Buffer.Lines))
}

func (e *Editor) applyEdit(edit Edit) (Change, error) {
	return e.apply(edit, func(change Change) Pos {
		return posAfterText(change.Start, change.NewText)
	})
}

func (e *Editor) applyEditAt(edit Edit, after Pos) (Change, error) {
	return e.apply(edit, func(Change) Pos { return after })
}

func (e *Editor) apply(edit Edit, after func(Change) Pos) (Change, error) {
	before := e.Cursor.Pos
	change, err := e.Buffer.Apply(edit)
	if err != nil || change.Empty() {
		return change, err
	}
	change.Before = before
	e.setCursor(after(change), true)
	change.After = e.Cursor.Pos
	e.record(change)
	e.EnsureVisible()
	return change, nil
}

// Apply applies a generic edit at the given range and records one undo entry.
// The edit text must use canonical LF newlines; input-bearing callers normalize
// CRLF exactly once before calling. The cursor moves to the end of the inserted
// text.
func (e *Editor) Apply(edit Edit) (Change, error) {
	return e.applyEdit(edit)
}

// Insert inserts text at the cursor and moves the cursor after it. CRLF is
// normalized exactly once here; Buffer.Apply expects canonical text.
func (e *Editor) Insert(text string) (Change, error) {
	text = NormalizeNewlines(text)
	if text == "" {
		return Change{}, nil
	}
	return e.applyEdit(Edit{Start: e.Cursor.Pos, End: e.Cursor.Pos, Text: text})
}

// DeleteRange deletes text between start and end. The cursor moves to start.
func (e *Editor) DeleteRange(start, end Pos) (Change, error) {
	if end.Before(start) {
		return Change{}, ErrInvalidRange
	}
	return e.applyEditAt(Edit{Start: start, End: end}, start)
}

// DeleteSelection deletes the current selection. Linewise selections remove
// whole lines; characterwise selections remove the inclusive cluster range.
func (e *Editor) DeleteSelection() (Change, error) {
	if e.Selection.Empty() {
		return Change{}, nil
	}
	if e.Selection.Kind == SelectionLine {
		first, last := e.Selection.LineRange()
		return e.DeleteLines(first, last)
	}
	start, end, ok := e.characterwiseRange()
	if !ok {
		return Change{}, nil
	}
	return e.DeleteRange(start, end)
}

// DeleteLines deletes complete lines, including the adjoining newline. The
// cursor lands at the start of the affected position after the edit.
func (e *Editor) DeleteLines(first, last int) (Change, error) {
	first, last, err := e.lineRange(first, last)
	if err != nil {
		return Change{}, err
	}
	start, end := e.lineDeleteRange(first, last)
	return e.applyEditAt(Edit{Start: start, End: end}, start)
}

// LineRangeText returns a linewise range with a trailing newline, suitable for
// a linewise register.
func (e *Editor) LineRangeText(first, last int) (string, error) {
	first, last, err := e.lineRange(first, last)
	if err != nil {
		return "", err
	}
	return strings.Join(e.Buffer.Lines[first:last+1], "\n") + "\n", nil
}

func (e *Editor) lineRange(first, last int) (int, int, error) {
	if len(e.Buffer.Lines) == 0 {
		return 0, 0, ErrInvalidRange
	}
	if first > last {
		first, last = last, first
	}
	if first < 0 || last >= len(e.Buffer.Lines) {
		return 0, 0, ErrInvalidRange
	}
	return first, last, nil
}

func (e *Editor) lineDeleteRange(first, last int) (start, end Pos) {
	lastLine := len(e.Buffer.Lines) - 1
	switch {
	case last < lastLine:
		return Pos{Line: first}, Pos{Line: last + 1}
	case first > 0:
		return Pos{Line: first - 1, Rune: utf8.RuneCountInString(e.Buffer.Lines[first-1])},
			Pos{Line: last, Rune: utf8.RuneCountInString(e.Buffer.Lines[last])}
	default:
		return Pos{}, Pos{Line: last, Rune: utf8.RuneCountInString(e.Buffer.Lines[last])}
	}
}

// SplitLine splits the current line at the cursor (Enter).
func (e *Editor) SplitLine() (Change, error) {
	return e.Insert("\n")
}

// JoinLines joins the current line with the next one (J). One space is kept
// unless the current line ends in whitespace or the next starts with a closing
// bracket.
func (e *Editor) JoinLines() (Change, error) {
	line := e.Cursor.Pos.Line
	if line+1 >= len(e.Buffer.Lines) {
		return Change{}, nil
	}
	current := e.Buffer.Lines[line]
	next := e.Buffer.Lines[line+1]
	separator := " "
	if current != "" {
		last, _ := utf8.DecodeLastRuneInString(current)
		if unicode.IsSpace(last) {
			separator = ""
		}
	}
	if next != "" {
		first, _ := utf8.DecodeRuneInString(next)
		if first == ')' || first == ']' || first == '}' {
			separator = ""
		}
	}
	end := Pos{Line: line, Rune: utf8.RuneCountInString(current)}
	edit := Edit{
		Start: end,
		End:   Pos{Line: line + 1},
		Text:  separator,
	}
	return e.applyEditAt(edit, end)
}

// IndentLines inserts delta spaces at the start of each line when delta is
// positive. A negative delta removes up to -delta leading spaces, or one
// leading tab. The whole range is one undo entry.
func (e *Editor) IndentLines(first, last, delta int) (Change, error) {
	first, last, err := e.lineRange(first, last)
	if err != nil {
		return Change{}, err
	}
	if delta == 0 {
		return Change{}, nil
	}
	oldLines := append([]string(nil), e.Buffer.Lines[first:last+1]...)
	newLines := make([]string, len(oldLines))
	changed := false
	for i, line := range oldLines {
		if delta > 0 {
			newLines[i] = strings.Repeat(" ", delta) + line
			changed = true
			continue
		}
		newLines[i] = removeIndent(line, -delta)
		if newLines[i] != line {
			changed = true
		}
	}
	if !changed {
		return Change{}, nil
	}
	edit := Edit{
		Start: Pos{Line: first},
		End:   Pos{Line: last, Rune: utf8.RuneCountInString(e.Buffer.Lines[last])},
		Text:  strings.Join(newLines, "\n"),
	}
	after := Pos{Line: first, Rune: firstNonBlank(newLines[0])}
	return e.applyEditAt(edit, after)
}

func removeIndent(line string, spaces int) string {
	if strings.HasPrefix(line, "\t") {
		return line[1:]
	}
	remove := 0
	for remove < spaces && remove < len(line) && line[remove] == ' ' {
		remove++
	}
	return line[remove:]
}

func firstNonBlank(line string) int {
	index := 0
	for _, r := range line {
		if r != ' ' && r != '\t' {
			return index
		}
		index++
	}
	return index
}

// ReplaceCluster replaces the cluster under the cursor. The cursor stays on the
// replacement's first cluster.
func (e *Editor) ReplaceCluster(text string) (Change, error) {
	line := e.Buffer.Lines[e.Cursor.Pos.Line]
	if e.Cursor.Pos.Rune >= utf8.RuneCountInString(line) {
		return Change{}, ErrNoCluster
	}
	end := Pos{Line: e.Cursor.Pos.Line, Rune: nextClusterBoundary(line, e.Cursor.Pos.Rune)}
	edit := Edit{Start: e.Cursor.Pos, End: end, Text: NormalizeNewlines(text)}
	return e.applyEditAt(edit, e.Cursor.Pos)
}

// InsertTab inserts spaces up to the next tab stop.
func (e *Editor) InsertTab() (Change, error) {
	width := e.Buffer.TabWidth
	if width < 1 {
		width = 1
	}
	cell := e.Buffer.CellColumn(e.Cursor.Pos)
	count := width - cell%width
	return e.Insert(strings.Repeat(" ", count))
}

// DeleteClusterBefore deletes the cluster before the cursor (Backspace). At the
// start of a line it joins with the previous line.
func (e *Editor) DeleteClusterBefore() (Change, error) {
	position := e.Buffer.NormalizePos(e.Cursor.Pos)
	if position.Rune == 0 {
		if position.Line == 0 {
			return Change{}, nil
		}
		previous := Pos{
			Line: position.Line - 1,
			Rune: utf8.RuneCountInString(e.Buffer.Lines[position.Line-1]),
		}
		return e.applyEditAt(Edit{Start: previous, End: Pos{Line: position.Line}}, previous)
	}
	start := Pos{Line: position.Line, Rune: prevClusterBoundary(e.Buffer.Lines[position.Line], position.Rune)}
	return e.applyEditAt(Edit{Start: start, End: position}, start)
}

// DeleteCluster deletes the cluster under the cursor (Delete). At end of line it
// joins with the next line.
func (e *Editor) DeleteCluster() (Change, error) {
	position := e.Buffer.NormalizePos(e.Cursor.Pos)
	line := e.Buffer.Lines[position.Line]
	if position.Rune >= utf8.RuneCountInString(line) {
		if position.Line+1 >= len(e.Buffer.Lines) {
			return Change{}, nil
		}
		return e.applyEditAt(Edit{
			Start: Pos{Line: position.Line, Rune: utf8.RuneCountInString(line)},
			End:   Pos{Line: position.Line + 1},
		}, position)
	}
	end := Pos{Line: position.Line, Rune: nextClusterBoundary(line, position.Rune)}
	return e.applyEditAt(Edit{Start: position, End: end}, position)
}

// Undo reverts the newest change. It reports false when there is nothing to
// undo.
func (e *Editor) Undo() (Change, bool) {
	e.EndGroup()
	change, ok := e.History.PopUndo()
	if !ok {
		return Change{}, false
	}
	inverse := Edit{
		Start: change.Start,
		End:   posAfterText(change.Start, change.NewText),
		Text:  change.OldText,
	}
	if _, err := e.Buffer.applyRaw(inverse); err != nil {
		e.History.PushUndo(change)
		return Change{}, false
	}
	e.History.PushRedo(change)
	e.setCursor(change.Before, true)
	e.Selection = Selection{}
	e.EnsureVisible()
	return change, true
}

// Redo reapplies the newest undone change.
func (e *Editor) Redo() (Change, bool) {
	e.EndGroup()
	change, ok := e.History.PopRedo()
	if !ok {
		return Change{}, false
	}
	forward := Edit{
		Start: change.Start,
		End:   posAfterText(change.Start, change.OldText),
		Text:  change.NewText,
	}
	if _, err := e.Buffer.applyRaw(forward); err != nil {
		e.History.PushRedo(change)
		return Change{}, false
	}
	e.History.PushUndo(change)
	e.setCursor(change.After, true)
	e.Selection = Selection{}
	e.EnsureVisible()
	return change, true
}

// CanUndo reports whether Undo has an entry.
func (e *Editor) CanUndo() bool {
	return e.History.CanUndo()
}

// CanRedo reports whether Redo has an entry.
func (e *Editor) CanRedo() bool {
	return e.History.CanRedo()
}

// Save writes the buffer and ends any open insert group.
func (e *Editor) Save() error {
	e.EndGroup()
	return e.Buffer.Save()
}

// Reload re-reads the file, discards undo history and selection, and keeps the
// cursor within the reloaded text. Use this instead of Buffer.Reload when an
// Editor owns the buffer.
func (e *Editor) Reload() error {
	e.EndGroup()
	if err := e.Buffer.Reload(); err != nil {
		return err
	}
	e.History.Undo = nil
	e.History.Redo = nil
	e.Selection = Selection{}
	e.setCursor(e.Cursor.Pos, true)
	e.EnsureVisible()
	return nil
}
