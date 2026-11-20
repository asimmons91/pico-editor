package editor

import (
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func editorFromText(text string) *Editor {
	buffer := New("")
	if text == "" {
		buffer.Lines = []string{""}
		buffer.TrailingNewline = false
	} else {
		buffer.TrailingNewline = strings.HasSuffix(text, "\n")
		body := text
		if buffer.TrailingNewline {
			body = body[:len(body)-1]
		}
		buffer.Lines = strings.Split(body, "\n")
	}
	buffer.hashValid = false
	buffer.indexValid = false
	buffer.MarkSaved()
	editor := NewEditor(buffer)
	editor.History = UndoStack{Limit: DefaultUndoLimit}
	return editor
}

func TestApplyInsertAndDelete(t *testing.T) {
	t.Parallel()

	editor := editorFromText("hello world\n")
	if _, err := editor.Insert("brave "); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if got := editor.Buffer.Lines[0]; got != "brave hello world" {
		t.Errorf("line = %q, want brave hello world", got)
	}
	if got := editor.Buffer.Text(); got != "brave hello world\n" {
		t.Errorf("text = %q", got)
	}

	if _, err := editor.DeleteRange(Pos{}, Pos{Line: 0, Rune: 6}); err != nil {
		t.Fatalf("DeleteRange: %v", err)
	}
	if got := editor.Buffer.Text(); got != "hello world\n" {
		t.Errorf("after delete text = %q", got)
	}
	if editor.Cursor.Pos != (Pos{}) {
		t.Errorf("cursor after delete = %v, want origin", editor.Cursor.Pos)
	}
}

func TestApplyRejectsClusterSplits(t *testing.T) {
	t.Parallel()

	editor := editorFromText("⬅️x\n")
	_, err := editor.Buffer.Apply(Edit{
		Start: Pos{Line: 0, Rune: 1},
		End:   Pos{Line: 0, Rune: 1},
		Text:  "!",
	})
	if err != ErrInvalidPosition {
		t.Fatalf("Apply inside VS16 error = %v, want ErrInvalidPosition", err)
	}
	if got := editor.Buffer.Text(); got != "⬅️x\n" {
		t.Errorf("rejected edit changed text to %q", got)
	}
}

func TestApplyRejectsInvalidUTF8Text(t *testing.T) {
	t.Parallel()

	editor := editorFromText("abc\n")
	_, err := editor.Buffer.Apply(Edit{
		Start: Pos{Line: 0, Rune: 1},
		End:   Pos{Line: 0, Rune: 1},
		Text:  string([]byte{0xff}),
	})
	if err != ErrInvalidText {
		t.Fatalf("Apply invalid UTF-8 error = %v, want ErrInvalidText", err)
	}
	if got := editor.Buffer.Text(); got != "abc\n" {
		t.Errorf("rejected edit changed text to %q", got)
	}
}

func TestUndoPreservesLoneCRAtLineEnd(t *testing.T) {
	t.Parallel()

	editor := editorFromText("abc\n")
	editor.SetCursor(Pos{Line: 0, Rune: 3})
	editor.BeginGroup()
	if _, err := editor.Insert("\r"); err != nil {
		t.Fatalf("Insert CR: %v", err)
	}
	if _, err := editor.SplitLine(); err != nil {
		t.Fatalf("SplitLine: %v", err)
	}
	editor.EndGroup()
	if got := editor.Buffer.Text(); got != "abc\r\n\n" {
		t.Fatalf("text = %q, want abc\\r\\n\\n", got)
	}
	editor.Undo()
	if got := editor.Buffer.Text(); got != "abc\n" {
		t.Errorf("undo text = %q, want abc\\n", got)
	}
	editor.Redo()
	if got := editor.Buffer.Text(); got != "abc\r\n\n" {
		t.Errorf("redo text = %q, want abc\\r\\n\\n", got)
	}
}

func TestUndoRestoresCRLineDeletedAcrossBreak(t *testing.T) {
	t.Parallel()

	editor := editorFromText("abc\r\ndef\n")
	if _, err := editor.DeleteRange(Pos{Line: 0}, Pos{Line: 1}); err != nil {
		t.Fatalf("DeleteRange: %v", err)
	}
	if got := editor.Buffer.Text(); got != "def\n" {
		t.Fatalf("deleted text = %q, want def\\n", got)
	}
	editor.Undo()
	if got := editor.Buffer.Text(); got != "abc\r\ndef\n" {
		t.Errorf("undo text = %q, want the CR line restored", got)
	}
}

func TestIndentPreservesLoneCRAtLineEnd(t *testing.T) {
	t.Parallel()

	editor := editorFromText("a\r\nb\n")
	if _, err := editor.IndentLines(0, 1, 1); err != nil {
		t.Fatalf("IndentLines: %v", err)
	}
	if got := editor.Buffer.Lines[0]; got != " a\r" {
		t.Errorf("indented first line = %q, want a leading space and CR", got)
	}
	editor.Undo()
	if got := editor.Buffer.Text(); got != "a\r\nb\n" {
		t.Errorf("undo indent text = %q, want the original", got)
	}
}

func TestUndoInsertBeforeStandaloneVS16(t *testing.T) {
	t.Parallel()

	editor := editorFromText("\ufe0f\n")
	if _, err := editor.Insert("⬅"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if got := editor.Buffer.Lines[0]; got != "⬅\ufe0f" {
		t.Fatalf("line = %q, want base plus VS16", got)
	}
	if _, ok := editor.Undo(); !ok {
		t.Fatal("Undo failed for an insertion that merged with a following VS16")
	}
	if got := editor.Buffer.Text(); got != "\ufe0f\n" {
		t.Errorf("undo text = %q, want the standalone VS16", got)
	}
	if _, ok := editor.Redo(); !ok {
		t.Fatal("Redo failed after VS16 undo")
	}
	if got := editor.Buffer.Text(); got != "⬅\ufe0f\n" {
		t.Errorf("redo text = %q, want the merged glyph", got)
	}
}

func TestDeleteThenRetypeSameTextLeavesNoUndoEntry(t *testing.T) {
	t.Parallel()

	editor := editorFromText("axb\n")
	editor.SetCursor(Pos{Line: 0, Rune: 2})
	editor.BeginGroup()
	if _, err := editor.DeleteClusterBefore(); err != nil {
		t.Fatalf("DeleteClusterBefore: %v", err)
	}
	if _, err := editor.Insert("x"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	editor.EndGroup()
	if got := editor.Buffer.Text(); got != "axb\n" {
		t.Fatalf("text = %q, want axb\\n", got)
	}
	if len(editor.History.Undo) != 0 {
		t.Errorf("undo entries = %d, want 0 for a net-zero replacement", len(editor.History.Undo))
	}
}

func TestUndoInsertAfterBaseWithStandaloneVS16(t *testing.T) {
	t.Parallel()

	editor := editorFromText("ab\n")
	editor.SetCursor(Pos{Line: 0, Rune: 1})
	if _, err := editor.Insert("\ufe0f"); err != nil {
		t.Fatalf("Insert VS16: %v", err)
	}
	if got := editor.Buffer.Lines[0]; got != "a\ufe0fb" {
		t.Fatalf("line = %q, want base plus VS16", got)
	}
	if _, ok := editor.Undo(); !ok {
		t.Fatal("Undo failed for an insertion that merged with the previous base")
	}
	if got := editor.Buffer.Text(); got != "ab\n" {
		t.Errorf("undo text = %q, want ab\\n", got)
	}
	if _, ok := editor.Redo(); !ok {
		t.Fatal("Redo failed after VS16 undo")
	}
	if got := editor.Buffer.Text(); got != "a\ufe0fb\n" {
		t.Errorf("redo text = %q, want the merged glyph", got)
	}
}

func TestUndoInsertThenBackspaceDoesNotMergeAcrossGroups(t *testing.T) {
	t.Parallel()

	editor := editorFromText("\n")
	if _, err := editor.Insert("Z"); err != nil {
		t.Fatalf("Insert Z: %v", err)
	}
	editor.BeginGroup()
	if _, err := editor.Insert("a"); err != nil {
		t.Fatalf("Insert a: %v", err)
	}
	if _, err := editor.DeleteClusterBefore(); err != nil {
		t.Fatalf("DeleteClusterBefore: %v", err)
	}
	if _, err := editor.Insert("b"); err != nil {
		t.Fatalf("Insert b: %v", err)
	}
	editor.EndGroup()
	if got := editor.Buffer.Text(); got != "Zb\n" {
		t.Fatalf("text = %q, want Zb\\n", got)
	}
	if len(editor.History.Undo) != 2 {
		t.Fatalf("undo entries = %d, want 2; a net-zero group must not merge with older history", len(editor.History.Undo))
	}
}

func TestInsertThenBackspaceLeavesNoUndoEntry(t *testing.T) {
	t.Parallel()

	editor := editorFromText("ab\n")
	editor.SetCursor(Pos{Line: 0, Rune: 1})
	editor.BeginGroup()
	if _, err := editor.Insert("x"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if _, err := editor.DeleteClusterBefore(); err != nil {
		t.Fatalf("DeleteClusterBefore: %v", err)
	}
	editor.EndGroup()
	if got := editor.Buffer.Text(); got != "ab\n" {
		t.Fatalf("text = %q, want ab\\n", got)
	}
	if len(editor.History.Undo) != 0 {
		t.Errorf("undo entries = %d, want 0 for a net-zero group", len(editor.History.Undo))
	}
	if _, ok := editor.Undo(); ok {
		t.Error("Undo reported a net-zero group")
	}
}

func TestPosAfterDoesNotClampToPreEditBuffer(t *testing.T) {
	t.Parallel()

	buffer := New("")
	if got := buffer.PosAfter(Pos{}, "a\nb"); got != (Pos{Line: 1, Rune: 1}) {
		t.Errorf("PosAfter multiline = %v, want 1,1", got)
	}
	buffer.Lines = []string{"abc"}
	if got := buffer.PosAfter(Pos{Line: 0, Rune: 3}, "d"); got != (Pos{Line: 0, Rune: 4}) {
		t.Errorf("PosAfter EOL = %v, want 0,4", got)
	}
	buffer.Lines = []string{"x"}
	if got := buffer.PosAfter(Pos{Line: 0, Rune: 1}, "\ny"); got != (Pos{Line: 1, Rune: 1}) {
		t.Errorf("PosAfter newline = %v, want 1,1", got)
	}
}

func TestInsertPreservesLoneCRBeforeCRLF(t *testing.T) {
	t.Parallel()

	editor := editorFromText("\n")
	if _, err := editor.Insert("a\r\r\nb"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if got := editor.Buffer.Lines[0]; got != "a\r" {
		t.Errorf("first line = %q, want the lone CR preserved", got)
	}
	if got := editor.Buffer.Lines[1]; got != "b" {
		t.Errorf("second line = %q, want b", got)
	}
	if got := NormalizeNewlines("a\r\r\nb"); got != "a\r\nb" {
		t.Errorf("NormalizeNewlines single pass = %q, want a\\r\\nb", got)
	}
}

func TestInsertAndDeleteAcrossLines(t *testing.T) {
	t.Parallel()

	editor := editorFromText("one\ntwo\nthree\n")
	editor.SetCursor(Pos{Line: 0, Rune: 1})
	if _, err := editor.Insert(",\n"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if got := editor.Buffer.Text(); got != "o,\nne\ntwo\nthree\n" {
		t.Errorf("split text = %q", got)
	}
	if _, err := editor.DeleteRange(Pos{Line: 0, Rune: 2}, Pos{Line: 1, Rune: 0}); err != nil {
		t.Fatalf("DeleteRange: %v", err)
	}
	if got := editor.Buffer.Text(); got != "o,ne\ntwo\nthree\n" {
		t.Errorf("joined text = %q", got)
	}
}

func TestSplitAndJoinLines(t *testing.T) {
	t.Parallel()

	editor := editorFromText("one two\nthree\n")
	editor.SetCursor(Pos{Line: 0, Rune: 4})
	if _, err := editor.SplitLine(); err != nil {
		t.Fatalf("SplitLine: %v", err)
	}
	if got := editor.Buffer.Text(); got != "one \ntwo\nthree\n" {
		t.Errorf("split text = %q", got)
	}
	editor.SetCursor(Pos{Line: 0, Rune: 4})
	if _, err := editor.JoinLines(); err != nil {
		t.Fatalf("JoinLines: %v", err)
	}
	if got := editor.Buffer.Text(); got != "one two\nthree\n" {
		t.Errorf("joined text = %q", got)
	}

	editor = editorFromText("one \ntwo\n")
	if _, err := editor.JoinLines(); err != nil {
		t.Fatalf("JoinLines: %v", err)
	}
	if got := editor.Buffer.Lines[0]; got != "one two" {
		t.Errorf("whitespace join = %q, want one two", got)
	}

	editor = editorFromText("one\n)\n")
	if _, err := editor.JoinLines(); err != nil {
		t.Fatalf("JoinLines: %v", err)
	}
	if got := editor.Buffer.Lines[0]; got != "one)" {
		t.Errorf("bracket join = %q, want one)", got)
	}
}

func TestIndentAndOutdentLines(t *testing.T) {
	t.Parallel()

	editor := editorFromText("a\nb\nc\n")
	if _, err := editor.IndentLines(0, 1, 2); err != nil {
		t.Fatalf("IndentLines: %v", err)
	}
	if got := editor.Buffer.Text(); got != "  a\n  b\nc\n" {
		t.Errorf("indented text = %q", got)
	}
	if editor.Cursor.Pos != (Pos{Line: 0, Rune: 2}) {
		t.Errorf("cursor after indent = %v, want 0,2", editor.Cursor.Pos)
	}
	if _, err := editor.IndentLines(0, 1, -1); err != nil {
		t.Fatalf("outdent: %v", err)
	}
	if got := editor.Buffer.Text(); got != " a\n b\nc\n" {
		t.Errorf("outdented text = %q", got)
	}
}

func TestReplaceClusterWideGlyph(t *testing.T) {
	t.Parallel()

	editor := editorFromText("a⬅️b\n")
	editor.SetCursor(Pos{Line: 0, Rune: 1})
	if _, err := editor.ReplaceCluster("x"); err != nil {
		t.Fatalf("ReplaceCluster: %v", err)
	}
	if got := editor.Buffer.Text(); got != "axb\n" {
		t.Errorf("replaced text = %q", got)
	}
	if editor.Cursor.Pos != (Pos{Line: 0, Rune: 1}) {
		t.Errorf("cursor = %v, want 0,1", editor.Cursor.Pos)
	}
	if _, err := editor.ReplaceCluster("x"); err == nil {
		// The cursor is on "b", so this is a valid replacement; the error
		// case needs end of line.
		editor.SetCursor(Pos{Line: 0, Rune: 3})
		if _, err := editor.ReplaceCluster("x"); err != ErrNoCluster {
			t.Errorf("ReplaceCluster at EOL error = %v, want ErrNoCluster", err)
		}
	}
}

func TestDeleteClusterBeforeAndAfter(t *testing.T) {
	t.Parallel()

	editor := editorFromText("ab\ncd\n")
	editor.SetCursor(Pos{Line: 1})
	if _, err := editor.DeleteClusterBefore(); err != nil {
		t.Fatalf("DeleteClusterBefore: %v", err)
	}
	if got := editor.Buffer.Text(); got != "abcd\n" {
		t.Errorf("join text = %q, want abcd\\n", got)
	}
	if editor.Cursor.Pos != (Pos{Line: 0, Rune: 2}) {
		t.Errorf("cursor = %v, want 0,2", editor.Cursor.Pos)
	}

	editor = editorFromText("abc\n")
	editor.SetCursor(Pos{Line: 0, Rune: 1})
	if _, err := editor.DeleteCluster(); err != nil {
		t.Fatalf("DeleteCluster: %v", err)
	}
	if got := editor.Buffer.Text(); got != "ac\n" {
		t.Errorf("delete cluster = %q, want ac\\n", got)
	}

	editor.SetCursor(Pos{Line: 0, Rune: 2})
	if _, err := editor.DeleteCluster(); err != nil {
		t.Fatalf("DeleteCluster at EOL: %v", err)
	}
	if got := editor.Buffer.Text(); got != "ac\n" {
		t.Errorf("delete at EOF = %q, want ac\\n", got)
	}
}

func TestInsertTabUsesTabStops(t *testing.T) {
	t.Parallel()

	editor := editorFromText("\n")
	if _, err := editor.InsertTab(); err != nil {
		t.Fatalf("InsertTab: %v", err)
	}
	if got := editor.Buffer.Text(); got != "  \n" {
		t.Errorf("tab at cell 0 = %q, want two spaces", got)
	}

	editor = editorFromText("a\n")
	editor.SetCursor(Pos{Line: 0, Rune: 1})
	if _, err := editor.InsertTab(); err != nil {
		t.Fatalf("InsertTab: %v", err)
	}
	if got := editor.Buffer.Text(); got != "a \n" {
		t.Errorf("tab at cell 1 = %q, want one space", got)
	}
}

func TestUndoGroupCoalescesInsertSession(t *testing.T) {
	t.Parallel()

	editor := editorFromText("\n")
	editor.BeginGroup()
	for _, text := range []string{"a", "b", "c"} {
		if _, err := editor.Insert(text); err != nil {
			t.Fatalf("Insert(%q): %v", text, err)
		}
	}
	if _, err := editor.DeleteClusterBefore(); err != nil {
		t.Fatalf("DeleteClusterBefore: %v", err)
	}
	if _, err := editor.Insert("d"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	editor.EndGroup()

	if got := editor.Buffer.Text(); got != "abd\n" {
		t.Fatalf("text = %q, want abd\\n", got)
	}
	if len(editor.History.Undo) != 1 {
		t.Fatalf("undo entries = %d, want 1", len(editor.History.Undo))
	}
	if _, ok := editor.Undo(); !ok {
		t.Fatal("Undo returned false")
	}
	if got := editor.Buffer.Text(); got != "\n" {
		t.Errorf("after undo text = %q, want newline", got)
	}
	if _, ok := editor.Redo(); !ok {
		t.Fatal("Redo returned false")
	}
	if got := editor.Buffer.Text(); got != "abd\n" {
		t.Errorf("after redo text = %q, want abd\\n", got)
	}
}

func TestUndoGroupIdleTimeout(t *testing.T) {
	t.Parallel()

	now := time.Unix(0, 0)
	editor := editorFromText("\n")
	editor.now = func() time.Time { return now }
	editor.BeginGroup()
	if _, err := editor.Insert("a"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	now = now.Add(insertGroupTimeout + time.Millisecond)
	if _, err := editor.Insert("b"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	editor.EndGroup()

	if len(editor.History.Undo) != 2 {
		t.Fatalf("undo entries = %d, want 2 after idle timeout", len(editor.History.Undo))
	}
	editor.Undo()
	if got := editor.Buffer.Text(); got != "a\n" {
		t.Errorf("after first undo text = %q, want a\\n", got)
	}
	editor.Undo()
	if got := editor.Buffer.Text(); got != "\n" {
		t.Errorf("after second undo text = %q, want newline", got)
	}
}

func TestNewEditClearsRedo(t *testing.T) {
	t.Parallel()

	editor := editorFromText("\n")
	if _, err := editor.Insert("a"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	editor.Undo()
	if !editor.CanRedo() {
		t.Fatal("redo stack is empty after undo")
	}
	if _, err := editor.Insert("b"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if editor.CanRedo() {
		t.Error("new edit did not clear the redo stack")
	}
}

func TestSavePointSurvivesUndoRedo(t *testing.T) {
	t.Parallel()

	editor := editorFromText("abc\n")
	if editor.Buffer.Dirty() {
		t.Fatal("initial buffer is dirty")
	}
	editor.MoveBufferEnd()
	if _, err := editor.Insert("d"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if !editor.Buffer.Dirty() {
		t.Fatal("buffer is not dirty after edit")
	}
	editor.Undo()
	if editor.Buffer.Dirty() {
		t.Error("undo to the save point left the buffer dirty")
	}
	editor.Redo()
	if !editor.Buffer.Dirty() {
		t.Error("redo away from the save point left the buffer clean")
	}
}

func TestWideGlyphCursorMotions(t *testing.T) {
	t.Parallel()

	editor := editorFromText("a⬅️b\n")
	editor.SetCursor(Pos{Line: 0, Rune: 1})
	editor.MoveRight(1)
	if editor.Cursor.Pos != (Pos{Line: 0, Rune: 3}) {
		t.Errorf("right over wide glyph = %v, want 0,3", editor.Cursor.Pos)
	}
	editor.MoveLeft(1)
	if editor.Cursor.Pos != (Pos{Line: 0, Rune: 1}) {
		t.Errorf("left over wide glyph = %v, want 0,1", editor.Cursor.Pos)
	}
	editor.MoveLeft(1)
	if editor.Cursor.Pos != (Pos{}) {
		t.Errorf("left to start = %v, want origin", editor.Cursor.Pos)
	}
}

func TestDesiredCellAcrossLines(t *testing.T) {
	t.Parallel()

	editor := editorFromText("abcdef\nx\nabcdef\n")
	editor.SetCursor(Pos{Line: 0, Rune: 5})
	editor.MoveDown(1)
	if editor.Cursor.Pos != (Pos{Line: 1, Rune: 1}) {
		t.Errorf("down to short line = %v, want 1,1", editor.Cursor.Pos)
	}
	if editor.Cursor.DesiredCell != 5 {
		t.Errorf("desired cell = %d, want 5", editor.Cursor.DesiredCell)
	}
	editor.MoveDown(1)
	if editor.Cursor.Pos != (Pos{Line: 2, Rune: 5}) {
		t.Errorf("down to long line = %v, want 2,5", editor.Cursor.Pos)
	}
	editor.MoveUp(1)
	if editor.Cursor.Pos != (Pos{Line: 1, Rune: 1}) {
		t.Errorf("up to short line = %v, want 1,1", editor.Cursor.Pos)
	}
	editor.MoveUp(1)
	if editor.Cursor.Pos != (Pos{Line: 0, Rune: 5}) {
		t.Errorf("up to original line = %v, want 0,5", editor.Cursor.Pos)
	}
}

func TestLineBoundaryMotionsWithWideGlyphs(t *testing.T) {
	t.Parallel()

	editor := editorFromText("a⬅️\nb⬅️c\n")
	editor.SetCursor(Pos{Line: 0, Rune: 3})
	editor.MoveRight(1)
	if editor.Cursor.Pos != (Pos{Line: 1, Rune: 0}) {
		t.Errorf("right at EOL = %v, want 1,0", editor.Cursor.Pos)
	}
	editor.MoveLeft(1)
	if editor.Cursor.Pos != (Pos{Line: 0, Rune: 3}) {
		t.Errorf("left at line start = %v, want 0,3", editor.Cursor.Pos)
	}
	editor.MoveDown(1)
	if editor.Cursor.Pos != (Pos{Line: 1, Rune: 3}) {
		t.Errorf("down with wide glyph = %v, want 1,3", editor.Cursor.Pos)
	}
	editor.MoveUp(1)
	if editor.Cursor.Pos != (Pos{Line: 0, Rune: 3}) {
		t.Errorf("up with wide glyph = %v, want 0,3", editor.Cursor.Pos)
	}
}

func TestCursorPrimitives(t *testing.T) {
	t.Parallel()

	editor := editorFromText("  abc\nx\n")
	editor.MoveLineEnd()
	if editor.Cursor.Pos != (Pos{Line: 0, Rune: 5}) {
		t.Errorf("MoveLineEnd = %v, want 0,5", editor.Cursor.Pos)
	}
	editor.MoveFirstNonBlank()
	if editor.Cursor.Pos != (Pos{Line: 0, Rune: 2}) {
		t.Errorf("MoveFirstNonBlank = %v, want 0,2", editor.Cursor.Pos)
	}
	editor.MoveLineStart()
	if editor.Cursor.Pos != (Pos{Line: 0}) {
		t.Errorf("MoveLineStart = %v, want origin", editor.Cursor.Pos)
	}
	editor.MoveToCell(1)
	if editor.Cursor.Pos != (Pos{Line: 0, Rune: 1}) {
		t.Errorf("MoveToCell(1) = %v, want 0,1", editor.Cursor.Pos)
	}
	editor.MoveTo(1, 1)
	if editor.Cursor.Pos != (Pos{Line: 1, Rune: 1}) {
		t.Errorf("MoveTo(1,1) = %v, want 1,1", editor.Cursor.Pos)
	}
	editor.MovePageUp()
	if editor.Cursor.Pos != (Pos{Line: 0, Rune: 1}) {
		t.Errorf("MovePageUp = %v, want 0,1", editor.Cursor.Pos)
	}
	editor.MovePageDown()
	if editor.Cursor.Pos != (Pos{Line: 1, Rune: 1}) {
		t.Errorf("MovePageDown = %v, want 1,1", editor.Cursor.Pos)
	}
	editor.SetPos(Pos{Line: 0, Rune: 1})
	if editor.Cursor.Pos != (Pos{Line: 0, Rune: 1}) {
		t.Errorf("SetPos = %v, want 0,1", editor.Cursor.Pos)
	}
}

func TestWordMotions(t *testing.T) {
	t.Parallel()

	editor := editorFromText("one two_three (four)\nfive\n")
	editor.MoveWordForward(1)
	if editor.Cursor.Pos != (Pos{Line: 0, Rune: 4}) {
		t.Errorf("w = %v, want 0,4", editor.Cursor.Pos)
	}
	editor.MoveWordForward(2)
	if editor.Cursor.Pos != (Pos{Line: 0, Rune: 15}) {
		t.Errorf("w over punctuation = %v, want 0,15", editor.Cursor.Pos)
	}
	editor.MoveWordForward(2)
	if editor.Cursor.Pos != (Pos{Line: 1, Rune: 0}) {
		t.Errorf("w across line = %v, want 1,0", editor.Cursor.Pos)
	}
	editor.MoveWordBackward(1)
	if editor.Cursor.Pos != (Pos{Line: 0, Rune: 19}) {
		t.Errorf("b from next line = %v, want 0,19", editor.Cursor.Pos)
	}

	editor = editorFromText("a.b c\n")
	editor.MoveBigWordForward(1)
	if editor.Cursor.Pos != (Pos{Line: 0, Rune: 4}) {
		t.Errorf("W = %v, want 0,4", editor.Cursor.Pos)
	}
	editor.MoveBigWordBackward(1)
	if editor.Cursor.Pos != (Pos{}) {
		t.Errorf("B = %v, want origin", editor.Cursor.Pos)
	}
}

func TestDeleteLinesEdges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		text  string
		first int
		last  int
		want  string
	}{
		{name: "middle", text: "a\nb\nc\n", first: 1, last: 1, want: "a\nc\n"},
		{name: "last", text: "a\nb\nc\n", first: 2, last: 2, want: "a\nb\n"},
		{name: "only", text: "a\n", first: 0, last: 0, want: "\n"},
		{name: "all", text: "a\nb\nc\n", first: 0, last: 2, want: "\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			editor := editorFromText(test.text)
			if _, err := editor.DeleteLines(test.first, test.last); err != nil {
				t.Fatalf("DeleteLines: %v", err)
			}
			if got := editor.Buffer.Text(); got != test.want {
				t.Errorf("text = %q, want %q", got, test.want)
			}
		})
	}
}

func TestLinewiseSelectedText(t *testing.T) {
	t.Parallel()

	editor := editorFromText("a\nb\nc\n")
	editor.Selection = Selection{
		Anchor: Pos{Line: 1},
		Cursor: Pos{Line: 1},
		Kind:   SelectionLine,
	}
	text, ok := editor.SelectedText()
	if !ok {
		t.Fatal("SelectedText returned false for a linewise selection")
	}
	if text != "b\n" {
		t.Errorf("SelectedText = %q, want b\\n", text)
	}
}

func TestSelectionDelete(t *testing.T) {
	t.Parallel()

	editor := editorFromText("abc\ndef\n")
	editor.Selection = Selection{
		Anchor: Pos{Line: 0},
		Cursor: Pos{Line: 0, Rune: 1},
		Kind:   SelectionChar,
	}
	text, ok := editor.SelectedText()
	if !ok || text != "ab" {
		t.Fatalf("SelectedText = %q, %t, want ab", text, ok)
	}
	if _, err := editor.DeleteSelection(); err != nil {
		t.Fatalf("DeleteSelection: %v", err)
	}
	if got := editor.Buffer.Text(); got != "c\ndef\n" {
		t.Errorf("char selection delete = %q, want c\\ndef\\n", got)
	}

	editor = editorFromText("abc\ndef\nghi\n")
	editor.Selection = Selection{
		Anchor: Pos{Line: 1},
		Cursor: Pos{Line: 1},
		Kind:   SelectionLine,
	}
	if _, err := editor.DeleteSelection(); err != nil {
		t.Fatalf("DeleteSelection line: %v", err)
	}
	if got := editor.Buffer.Text(); got != "abc\nghi\n" {
		t.Errorf("line selection delete = %q, want abc\\nghi\\n", got)
	}
}

func TestApplyNoopDoesNotVersion(t *testing.T) {
	t.Parallel()

	editor := editorFromText("abc\n")
	version := editor.Buffer.Version
	change, err := editor.Buffer.Apply(Edit{
		Start: Pos{Line: 0, Rune: 1},
		End:   Pos{Line: 0, Rune: 2},
		Text:  "b",
	})
	if err != nil {
		t.Fatalf("no-op Apply: %v", err)
	}
	if !change.Empty() {
		t.Errorf("no-op change = %+v, want empty", change)
	}
	if editor.Buffer.Version != version {
		t.Errorf("version = %d, want unchanged %d", editor.Buffer.Version, version)
	}
}

func TestUndoGroupBreaksOnMotion(t *testing.T) {
	t.Parallel()

	editor := editorFromText("xy\n")
	editor.BeginGroup()
	if _, err := editor.Insert("a"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	editor.MoveRight(1)
	if _, err := editor.Insert("b"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	editor.EndGroup()
	if got := editor.Buffer.Text(); got != "axby\n" {
		t.Fatalf("text = %q, want axby\\n", got)
	}
	if len(editor.History.Undo) != 2 {
		t.Fatalf("undo entries = %d, want 2 after a motion boundary", len(editor.History.Undo))
	}
	editor.Undo()
	if got := editor.Buffer.Text(); got != "axy\n" {
		t.Errorf("first undo text = %q, want axy\\n", got)
	}
	editor.Undo()
	if got := editor.Buffer.Text(); got != "xy\n" {
		t.Errorf("second undo text = %q, want xy\\n", got)
	}
}

func TestUndoGroupCoalescesMultilineInsert(t *testing.T) {
	t.Parallel()

	editor := editorFromText("\n")
	editor.BeginGroup()
	if _, err := editor.Insert("a\nb"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if _, err := editor.Insert("c"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	editor.EndGroup()
	if got := editor.Buffer.Text(); got != "a\nbc\n" {
		t.Fatalf("text = %q, want a\\nbc\\n", got)
	}
	if len(editor.History.Undo) != 1 {
		t.Fatalf("undo entries = %d, want 1", len(editor.History.Undo))
	}
	editor.Undo()
	if got := editor.Buffer.Text(); got != "\n" {
		t.Errorf("undo text = %q, want a newline", got)
	}
	editor.Redo()
	if got := editor.Buffer.Text(); got != "a\nbc\n" {
		t.Errorf("redo text = %q, want a\\nbc\\n", got)
	}
}

func TestUndoRedoRestoreCursor(t *testing.T) {
	t.Parallel()

	editor := editorFromText("abc\n")
	editor.SetCursor(Pos{Line: 0, Rune: 3})
	if _, err := editor.Insert("d"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if editor.Cursor.Pos != (Pos{Line: 0, Rune: 4}) {
		t.Fatalf("cursor after insert = %v, want 0,4", editor.Cursor.Pos)
	}
	editor.Undo()
	if editor.Cursor.Pos != (Pos{Line: 0, Rune: 3}) {
		t.Errorf("cursor after undo = %v, want Change.Before 0,3", editor.Cursor.Pos)
	}
	editor.Redo()
	if editor.Cursor.Pos != (Pos{Line: 0, Rune: 4}) {
		t.Errorf("cursor after redo = %v, want Change.After 0,4", editor.Cursor.Pos)
	}
}

func TestSelectAllMovesCursor(t *testing.T) {
	t.Parallel()

	editor := editorFromText("abc\ndef\n")
	editor.SelectAll()
	if editor.Cursor.Pos != (Pos{Line: 1, Rune: 3}) {
		t.Errorf("cursor after SelectAll = %v, want the buffer end", editor.Cursor.Pos)
	}
	if editor.Selection.Cursor != editor.Cursor.Pos {
		t.Errorf("selection cursor %v diverges from editor cursor %v", editor.Selection.Cursor, editor.Cursor.Pos)
	}
	text, ok := editor.SelectedText()
	if !ok || text != "abc\ndef" {
		t.Errorf("SelectedText = %q, %t, want abc\\ndef", text, ok)
	}
}

func TestUndoHistoryCap(t *testing.T) {
	t.Parallel()

	editor := editorFromText("\n")
	editor.History.Limit = 3
	for i := 0; i < 6; i++ {
		editor.MoveBufferEnd()
		if _, err := editor.Insert(string(rune('a' + i))); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}
	if got := len(editor.History.Undo); got != 3 {
		t.Fatalf("undo entries = %d, want 3", got)
	}
	for editor.CanUndo() {
		editor.Undo()
	}
	if got := editor.Buffer.Text(); got == "\n" {
		t.Error("capped history undid more than the retained entries")
	}
	if !utf8.ValidString(editor.Buffer.Text()) {
		t.Error("undo produced invalid UTF-8")
	}
}

func TestFuzzEditUndoRestoresText(t *testing.T) {
	seeds := []string{"", "\n", "abc", "a\nb\n", "⬅️😐️\n", "one two\nthree"}
	for _, seed := range seeds {
		seed := seed
		t.Run("seed", func(t *testing.T) {
			editor := editorFromText(seed)
			initial := editor.Buffer.Text()
			check := func(_ Change, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("edit: %v", err)
				}
			}
			for i := 0; i < 20; i++ {
				editor.SetCursor(Pos{Line: i % len(editor.Buffer.Lines), Rune: i})
				switch i % 5 {
				case 0:
					check(editor.Insert("x"))
				case 1:
					check(editor.DeleteClusterBefore())
				case 2:
					check(editor.DeleteCluster())
				case 3:
					check(editor.SplitLine())
				case 4:
					check(editor.JoinLines())
				}
			}
			for editor.CanUndo() {
				editor.Undo()
			}
			if got := editor.Buffer.Text(); got != initial {
				t.Errorf("undo-all text = %q, want %q", got, initial)
			}
		})
	}
}

func FuzzEditUndo(f *testing.F) {
	f.Add("", []byte{0, 1, 2, 3})
	f.Add("abc", []byte{3, 1, 4, 1, 5})
	f.Add("a\nb\n", []byte{2, 2, 0})
	f.Add("⬅️😐️\n", []byte{4, 1, 3})
	f.Fuzz(func(t *testing.T, text string, operations []byte) {
		if !utf8.ValidString(text) {
			t.Skip()
		}
		if len(operations) > DefaultUndoLimit {
			operations = operations[:DefaultUndoLimit]
		}
		editor := editorFromText(text)
		initial := editor.Buffer.Text()
		check := func(_ Change, err error) {
			t.Helper()
			if err != nil {
				t.Fatalf("edit: %v", err)
			}
		}
		for _, operation := range operations {
			line := int(operation) % len(editor.Buffer.Lines)
			runeOffset := int(operation) % 8
			editor.SetCursor(Pos{Line: line, Rune: runeOffset})
			switch operation % 5 {
			case 0:
				check(editor.Insert("ab"))
			case 1:
				check(editor.DeleteClusterBefore())
			case 2:
				check(editor.DeleteCluster())
			case 3:
				check(editor.SplitLine())
			case 4:
				check(editor.JoinLines())
			}
			if !utf8.ValidString(editor.Buffer.Text()) {
				t.Fatalf("edit sequence produced invalid UTF-8: %q", editor.Buffer.Text())
			}
			if editor.Cursor.Pos.Line < 0 || editor.Cursor.Pos.Line >= len(editor.Buffer.Lines) {
				t.Fatalf("cursor line out of range: %v", editor.Cursor.Pos)
			}
		}
		for editor.CanUndo() {
			editor.Undo()
		}
		if got := editor.Buffer.Text(); got != initial {
			t.Fatalf("undo-all text = %q, want %q", got, initial)
		}
	})
}

func BenchmarkApplySingleCharacter(b *testing.B) {
	editor := editorFromText(strings.Repeat("print(1)\n", 8000))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		editor.SetCursor(Pos{})
		if _, err := editor.Insert("x"); err != nil {
			b.Fatal(err)
		}
		if _, err := editor.DeleteCluster(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOpen64K(b *testing.B) {
	data := []byte(strings.Repeat("print(1)\n", 8000))
	path := writeBenchmarkFile(b, data)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buffer, err := Open(path)
		if err != nil {
			b.Fatal(err)
		}
		_ = buffer.Text()
	}
}

func writeBenchmarkFile(b *testing.B, data []byte) string {
	b.Helper()
	path := b.TempDir() + "/bench.lua"
	if err := os.WriteFile(path, data, 0o644); err != nil {
		b.Fatal(err)
	}
	return path
}
