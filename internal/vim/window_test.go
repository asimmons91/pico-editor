package vim

import (
	"testing"

	"github.com/asimmons91/pico-editor/internal/editor"
)

func vimWindowEditor(t *testing.T, lines ...string) (*Vim, *editor.Editor) {
	t.Helper()
	buffer := editor.New("")
	buffer.Lines = append([]string(nil), lines...)
	buffer.MarkSaved()
	ed := editor.NewEditor(buffer)
	return New(), ed
}

func TestBracketMotionStaysInActiveTab(t *testing.T) {
	_, ed := vimWindowEditor(t, "f(1", "-->8", "g)")
	ed.SetLineRange(0, 1)
	ed.SetCursor(editor.Pos{Line: 0, Rune: 1})
	if _, ok := matchBracket(ed, ed.Cursor.Pos); ok {
		t.Fatal("matchBracket paired a bracket in another tab")
	}
}

func TestScrollCursorClampsToActiveTab(t *testing.T) {
	v, ed := vimWindowEditor(t, "one", "two", "three", "four", "five")
	ed.SetLineRange(2, 5)
	ed.SetCursor(editor.Pos{Line: 2})
	v.scrollCursor(ed, 'b')
	if ed.Viewport.TopLine < 2 {
		t.Fatalf("TopLine = %d, want at least the tab start", ed.Viewport.TopLine)
	}
}
