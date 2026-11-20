package editor

import "strings"

// Change records one accepted edit. Start and the two text values are enough to
// apply the change in either direction; Before and After are the cursor
// positions recorded by the Editor.
type Change struct {
	Start   Pos
	OldText string
	NewText string
	Before  Pos
	After   Pos
}

// Empty reports whether the change contains no textual edit, including a
// replacement whose old and new text are identical.
func (c Change) Empty() bool {
	return c.OldText == c.NewText
}

// UndoStack is a linear undo/redo history with a bounded size.
type UndoStack struct {
	Undo  []Change
	Redo  []Change
	Limit int
}

// DefaultUndoLimit is the V1 history cap.
const DefaultUndoLimit = 1000

// Push appends a change and clears the redo branch.
func (u *UndoStack) Push(change Change) {
	if change.Empty() {
		return
	}
	u.Redo = nil
	u.Undo = append(u.Undo, change)
	u.trim()
}

// ReplaceLast replaces the newest undo entry, preserving the redo branch.
func (u *UndoStack) ReplaceLast(change Change) {
	if len(u.Undo) == 0 {
		u.Push(change)
		return
	}
	u.Undo[len(u.Undo)-1] = change
}

// PopUndo removes and returns the newest undo entry.
func (u *UndoStack) PopUndo() (Change, bool) {
	if len(u.Undo) == 0 {
		return Change{}, false
	}
	change := u.Undo[len(u.Undo)-1]
	u.Undo = u.Undo[:len(u.Undo)-1]
	return change, true
}

// PopRedo removes and returns the newest redo entry.
func (u *UndoStack) PopRedo() (Change, bool) {
	if len(u.Redo) == 0 {
		return Change{}, false
	}
	change := u.Redo[len(u.Redo)-1]
	u.Redo = u.Redo[:len(u.Redo)-1]
	return change, true
}

// PushRedo appends a change to the redo branch.
func (u *UndoStack) PushRedo(change Change) {
	if change.Empty() {
		return
	}
	u.Redo = append(u.Redo, change)
}

// PushUndo appends a change to the undo stack without clearing the redo branch.
// It is used when undoing a redo or restoring an entry after a failed apply.
func (u *UndoStack) PushUndo(change Change) {
	if change.Empty() {
		return
	}
	u.Undo = append(u.Undo, change)
	u.trim()
}

// CanUndo reports whether an undo entry is available.
func (u *UndoStack) CanUndo() bool {
	return len(u.Undo) > 0
}

// CanRedo reports whether a redo entry is available.
func (u *UndoStack) CanRedo() bool {
	return len(u.Redo) > 0
}

func (u *UndoStack) trim() {
	limit := u.Limit
	if limit <= 0 {
		limit = DefaultUndoLimit
	}
	if len(u.Undo) <= limit {
		return
	}
	discard := len(u.Undo) - limit
	copy(u.Undo, u.Undo[discard:])
	u.Undo = u.Undo[:limit]
}

// mergeChange folds next into previous when the two edits touch the same point.
// This is what makes an insert session a single undo entry, including
// backspaces inside the session.
func mergeChange(previous, next Change) (Change, bool) {
	previousEnd := posAfterText(previous.Start, previous.NewText)

	switch {
	case previousEnd == next.Start:
		merged := previous
		merged.NewText += next.NewText
		merged.OldText += next.OldText
		merged.After = next.After
		return merged, true
	case next.NewText == "" && posAfterText(next.Start, next.OldText) == previousEnd:
		if len(next.OldText) > len(previous.NewText) || !strings.HasSuffix(previous.NewText, next.OldText) {
			return Change{}, false
		}
		merged := previous
		merged.NewText = previous.NewText[:len(previous.NewText)-len(next.OldText)]
		merged.After = next.After
		return merged, true
	default:
		return Change{}, false
	}
}

// posAfterText is the pure position calculation used while merging changes; it
// does not depend on the buffer because the two changes are already adjacent.
func posAfterText(start Pos, text string) Pos {
	line := start.Line
	column := start.Rune
	for _, r := range text {
		if r == '\n' {
			line++
			column = 0
		} else {
			column++
		}
	}
	return Pos{Line: line, Rune: column}
}
