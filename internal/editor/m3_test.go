package editor

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func newBlockEditor(t *testing.T, text string) *Editor {
	t.Helper()
	buffer := New("block.lua")
	buffer.Lines = splitTestLines(text)
	buffer.TrailingNewline = false
	buffer.MarkSaved()
	ed := NewEditor(buffer)
	return ed
}

func splitTestLines(text string) []string {
	if text == "" {
		return []string{""}
	}
	var lines []string
	start := 0
	for index := 0; index < len(text); index++ {
		if text[index] == '\n' {
			lines = append(lines, text[start:index])
			start = index + 1
		}
	}
	lines = append(lines, text[start:])
	return lines
}

func TestBlockLineRangeIncludesWideClusterWholly(t *testing.T) {
	ed := newBlockEditor(t, "a\u2588b")
	ed.Selection = Selection{
		Anchor: Pos{Line: 0, Rune: 1},
		Cursor: Pos{Line: 0, Rune: 1},
		Kind:   SelectionBlock,
	}
	start, end, ok := ed.Selection.BlockLineRange(ed.Buffer, 0)
	if !ok || start.Rune != 1 || end.Rune != 2 {
		t.Fatalf("block range = %+v..%+v ok=%t, want rune 1..2", start, end, ok)
	}

	// A rectangle that starts on the wide glyph's second cell still includes
	// the whole cluster.
	ed.Selection.Anchor.Rune = 2
	ed.Selection.Cursor.Rune = 2
	start, end, ok = ed.Selection.BlockLineRange(ed.Buffer, 0)
	if !ok || start.Rune != 2 || end.Rune != 3 {
		t.Fatalf("edge block = %+v..%+v ok=%t, want rune 2..3", start, end, ok)
	}
}

func TestBlockSelectedTextPadsShortLines(t *testing.T) {
	ed := newBlockEditor(t, "abcd\nef")
	ed.Selection = Selection{
		Anchor: Pos{Line: 0, Rune: 0},
		Cursor: Pos{Line: 1, Rune: 1},
		Kind:   SelectionBlock,
	}
	text, ok := ed.SelectedText()
	if !ok {
		t.Fatal("SelectedText() did not report a block")
	}
	if text != "ab\nef" {
		t.Fatalf("block text = %q, want %q", text, "ab\nef")
	}

	ed.Selection.Anchor = Pos{Line: 0, Rune: 2}
	ed.Selection.Cursor = Pos{Line: 1, Rune: 3}
	text, ok = ed.SelectedText()
	if !ok || text != "c\n " {
		t.Fatalf("padded block text = %q ok=%t, want %q", text, ok, "c\n ")
	}
}

func TestMoveWordEnd(t *testing.T) {
	ed := newBlockEditor(t, "one two")
	ed.MoveWordEnd(1)
	if ed.Cursor.Pos != (Pos{Line: 0, Rune: 2}) {
		t.Fatalf("word end = %+v, want 0:2", ed.Cursor.Pos)
	}
	ed.MoveWordEnd(1)
	if ed.Cursor.Pos != (Pos{Line: 0, Rune: 6}) {
		t.Fatalf("second word end = %+v, want 0:6", ed.Cursor.Pos)
	}
	ed.MoveWordEnd(1)
	if ed.Cursor.Pos != (Pos{Line: 0, Rune: 6}) {
		t.Fatalf("word end at EOF = %+v, want 0:6", ed.Cursor.Pos)
	}
}

func TestSearchFindAllExpandsClusters(t *testing.T) {
	search := Search{Pattern: "a", Regex: false}
	matches, err := search.FindAll("a a")
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("matches = %d, want 2", len(matches))
	}
	if matches[1].Start != (Pos{Line: 0, Rune: 2}) {
		t.Errorf("second match = %+v", matches[1].Start)
	}
}

func TestPrepareAndCommitSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.lua")
	if err := os.WriteFile(path, []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	buffer, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buffer.Apply(Edit{
		Start: Pos{Line: 0, Rune: 3},
		End:   Pos{Line: 0, Rune: 3},
		Text:  " two",
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := buffer.PrepareSave()
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshot.Write(); err != nil {
		t.Fatal(err)
	}
	if err := buffer.CommitSave(snapshot, nil); err != nil {
		t.Fatal(err)
	}
	if buffer.Dirty() {
		t.Error("buffer is dirty after committing its snapshot")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "one two\n" {
		t.Fatalf("file = %q, want %q", data, "one two\n")
	}

	// An edit made while the write is in flight remains dirty.
	previous := snapshot
	if _, err := buffer.Apply(Edit{
		Start: Pos{Line: 0, Rune: 7},
		End:   Pos{Line: 0, Rune: 7},
		Text:  "!",
	}); err != nil {
		t.Fatal(err)
	}
	if err := buffer.CommitSave(previous, nil); err != nil {
		t.Fatal(err)
	}
	if !buffer.Dirty() {
		t.Error("buffer lost its newer edit during save commit")
	}
}

func TestCommitSaveFailureKeepsSavePoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fail.lua")
	if err := os.WriteFile(path, []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	buffer, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buffer.Apply(Edit{
		Start: Pos{Line: 0, Rune: 0},
		End:   Pos{Line: 0, Rune: 0},
		Text:  "X",
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := buffer.PrepareSave()
	if err != nil {
		t.Fatal(err)
	}
	if err := buffer.CommitSave(snapshot, errors.New("disk full")); err == nil {
		t.Fatal("CommitSave accepted a write failure")
	}
	if !buffer.Dirty() {
		t.Error("buffer became clean after a failed write")
	}
}
