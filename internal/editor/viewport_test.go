package editor

import "testing"

func TestViewportEnsureVisibleVertical(t *testing.T) {
	t.Parallel()

	viewport := Viewport{Width: 64, Height: 10, ScrollOff: 2}
	viewport.EnsureVisible(Pos{Line: 0}, 0, 1, 100)
	if viewport.TopLine != 0 {
		t.Errorf("top line at buffer start = %d, want 0", viewport.TopLine)
	}
	viewport.EnsureVisible(Pos{Line: 5}, 0, 1, 100)
	if viewport.TopLine != 0 {
		t.Errorf("top line inside window = %d, want 0", viewport.TopLine)
	}
	viewport.EnsureVisible(Pos{Line: 9}, 0, 1, 100)
	if viewport.TopLine != 2 {
		t.Errorf("top line after scrolloff = %d, want 2", viewport.TopLine)
	}
	viewport.EnsureVisible(Pos{Line: 99}, 0, 1, 100)
	if viewport.TopLine != 90 {
		t.Errorf("top line at buffer end = %d, want 90", viewport.TopLine)
	}
}

func TestViewportEnsureVisibleHorizontal(t *testing.T) {
	t.Parallel()

	viewport := Viewport{Width: 10, Height: 10, ScrollOff: 2}
	viewport.EnsureVisible(Pos{Line: 0}, 0, 1, 100)
	if viewport.LeftCell != 0 {
		t.Errorf("left cell = %d, want 0", viewport.LeftCell)
	}
	viewport.EnsureVisible(Pos{Line: 0}, 9, 1, 100)
	if viewport.LeftCell != 2 {
		t.Errorf("left cell after scrolloff = %d, want 2", viewport.LeftCell)
	}
	viewport.EnsureVisible(Pos{Line: 0}, 0, 1, 100)
	if viewport.LeftCell != 0 {
		t.Errorf("left cell after scrolling back = %d, want 0", viewport.LeftCell)
	}
}

func TestViewportWideGlyphVisibility(t *testing.T) {
	t.Parallel()

	viewport := Viewport{Width: 4, Height: 2, ScrollOff: 0}
	viewport.EnsureVisible(Pos{Line: 0}, 3, 2, 10)
	if viewport.LeftCell != 1 {
		t.Errorf("left cell for wide glyph = %d, want 1", viewport.LeftCell)
	}
}

func TestViewportVisibleLines(t *testing.T) {
	t.Parallel()

	buffer := editorFromText("a\nb\nc\nd\n").Buffer
	viewport := Viewport{TopLine: 1, Width: 10, Height: 2}
	lines := viewport.VisibleLines(buffer)
	if len(lines) != 2 {
		t.Fatalf("VisibleLines returned %d lines, want 2", len(lines))
	}
	if lines[0] != (VisibleLine{Line: 1, Row: 0}) || lines[1] != (VisibleLine{Line: 2, Row: 1}) {
		t.Errorf("VisibleLines = %+v", lines)
	}
}

func TestHorizontalMovePreservesCursorCell(t *testing.T) {
	t.Parallel()

	editor := editorFromText("0123456789\n")
	editor.SetCursor(Pos{Line: 0, Rune: 5})
	editor.Viewport.Width = 4
	editor.Viewport.ScrollOff = 0
	editor.EnsureVisible()
	if editor.Viewport.LeftCell != 2 {
		t.Errorf("left cell = %d, want 2", editor.Viewport.LeftCell)
	}
	editor.MoveDown(1)
	if editor.Cursor.Pos != (Pos{Line: 0, Rune: 5}) {
		t.Errorf("vertical move at last line changed cursor to %v", editor.Cursor.Pos)
	}
	if editor.Viewport.LeftCell != 2 {
		t.Errorf("vertical move changed horizontal scroll to %d, want 2", editor.Viewport.LeftCell)
	}
}
