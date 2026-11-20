package editor

import (
	"errors"
	"testing"
)

func windowEditor(t *testing.T, lines ...string) *Editor {
	t.Helper()
	buffer := New("")
	buffer.Lines = append([]string(nil), lines...)
	buffer.MarkSaved()
	ed := NewEditor(buffer)
	ed.SetLineRange(1, len(lines)-1)
	return ed
}

func TestLineWindowClampsCursorAndMotions(t *testing.T) {
	ed := windowEditor(t, "marker", "one", "two", "marker")
	ed.SetCursor(Pos{})
	if got := ed.Cursor.Pos; got != (Pos{Line: 1}) {
		t.Fatalf("cursor = %+v, want line 1", got)
	}
	ed.MoveBufferEnd()
	if got := ed.Cursor.Pos.Line; got != 2 {
		t.Fatalf("buffer end line = %d, want 2", got)
	}
	ed.MoveDown(5)
	if got := ed.Cursor.Pos.Line; got != 2 {
		t.Fatalf("move down crossed window: %d", got)
	}
	ed.MoveBufferStart()
	if got := ed.Cursor.Pos.Line; got != 1 {
		t.Fatalf("buffer start line = %d, want 1", got)
	}
}

func TestLineWindowRejectsCrossingEdits(t *testing.T) {
	ed := windowEditor(t, "marker", "one", "two", "marker")
	_, err := ed.Apply(Edit{Start: Pos{Line: 1, Rune: 3}, End: Pos{Line: 3, Rune: 1}, Text: ""})
	if !errors.Is(err, ErrOutsideRange) {
		t.Fatalf("crossing edit err = %v, want ErrOutsideRange", err)
	}
	text := ed.Buffer.Text()
	if text != "marker\none\ntwo\nmarker" {
		t.Fatalf("buffer = %q", text)
	}
}

func TestLineWindowJoinAndDeletesStopAtBoundaries(t *testing.T) {
	ed := windowEditor(t, "marker", "one", "two", "marker")
	ed.SetCursor(Pos{Line: 2, Rune: 3})
	if _, err := ed.JoinLines(); err != nil {
		t.Fatal(err)
	}
	if got := ed.Buffer.Text(); got != "marker\none\ntwo\nmarker" {
		t.Fatalf("J changed the buffer: %q", got)
	}
	if _, err := ed.DeleteCluster(); err != nil {
		t.Fatal(err)
	}
	if got := ed.Buffer.Text(); got != "marker\none\ntwo\nmarker" {
		t.Fatalf("x changed the buffer: %q", got)
	}
	ed.SetCursor(Pos{Line: 1})
	if _, err := ed.DeleteClusterBefore(); err != nil {
		t.Fatal(err)
	}
	if got := ed.Buffer.Text(); got != "marker\none\ntwo\nmarker" {
		t.Fatalf("backspace changed the buffer: %q", got)
	}
}

func TestLineWindowDeleteLastLineKeepsSeparator(t *testing.T) {
	ed := windowEditor(t, "marker", "one", "marker")
	ed.SetCursor(Pos{Line: 1})
	if _, err := ed.DeleteLines(1, 1); err != nil {
		t.Fatal(err)
	}
	first, end, active := ed.LineRange()
	if !active || first != 1 || end != 1 {
		t.Fatalf("range = %d..%d active %t, want empty 1..1", first, end, active)
	}
	if got := ed.Buffer.Text(); got != "marker\nmarker" {
		t.Fatalf("buffer = %q, want the separator preserved", got)
	}
	if !ed.WindowEmpty() {
		t.Fatal("window is not empty after deleting its last line")
	}
}

func TestEmptyWindowMaterializesOnInsert(t *testing.T) {
	ed := windowEditor(t, "a", "marker")
	ed.SetLineRange(1, 1)
	if !ed.WindowEmpty() {
		t.Fatal("window should be empty")
	}
	if _, err := ed.Insert("hello"); err != nil {
		t.Fatal(err)
	}
	if got := ed.Buffer.Text(); got != "a\nhello\nmarker" {
		t.Fatalf("buffer = %q, want hello before marker", got)
	}
	if got := ed.Cursor.Pos; got != (Pos{Line: 1, Rune: 5}) {
		t.Fatalf("cursor = %+v", got)
	}
	first, end, _ := ed.LineRange()
	if first != 1 || end != 2 {
		t.Fatalf("range = %d..%d, want 1..2", first, end)
	}
}

func TestEmptyWindowNewlineMaterializesOneLine(t *testing.T) {
	ed := windowEditor(t, "a", "marker")
	ed.SetLineRange(1, 1)
	if _, err := ed.Insert("\n"); err != nil {
		t.Fatal(err)
	}
	if got := ed.Buffer.Text(); got != "a\n\nmarker" {
		t.Fatalf("buffer = %q, want exactly one blank line", got)
	}
}

func TestLineWindowDeleteLastBufferLine(t *testing.T) {
	ed := windowEditor(t, "marker", "hello")
	ed.SetLineRange(1, 2)
	ed.SetCursor(Pos{Line: 1})
	if _, err := ed.DeleteLines(1, 1); err != nil {
		t.Fatal(err)
	}
	if got := ed.Buffer.Text(); got != "marker" {
		t.Fatalf("buffer = %q, want marker without the deleted last line", got)
	}
}

func TestLineWindowUndoRedo(t *testing.T) {
	ed := windowEditor(t, "marker", "one", "marker")
	ed.SetCursor(Pos{Line: 1, Rune: 3})
	if _, err := ed.Insert("!"); err != nil {
		t.Fatal(err)
	}
	if _, ok := ed.Undo(); !ok {
		t.Fatal("undo failed")
	}
	if got := ed.Buffer.Text(); got != "marker\none\nmarker" {
		t.Fatalf("after undo = %q", got)
	}
	if _, ok := ed.Redo(); !ok {
		t.Fatal("redo failed")
	}
	if got := ed.Buffer.Text(); got != "marker\none!\nmarker" {
		t.Fatalf("after redo = %q", got)
	}
}
