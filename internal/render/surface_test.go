package render

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/asimmons91/pico-editor/internal/font"
)

func TestPalette(t *testing.T) {
	want := [16][3]uint8{
		{0x00, 0x00, 0x00}, {0x1d, 0x2b, 0x53}, {0x7e, 0x25, 0x53}, {0x00, 0x87, 0x51},
		{0xab, 0x52, 0x36}, {0x5f, 0x57, 0x4f}, {0xc2, 0xc3, 0xc7}, {0xff, 0xf1, 0xe8},
		{0xff, 0x00, 0x4d}, {0xff, 0xa3, 0x00}, {0xff, 0xec, 0x27}, {0x00, 0xe4, 0x36},
		{0x29, 0xad, 0xff}, {0x83, 0x76, 0x9c}, {0xff, 0x77, 0xa8}, {0xff, 0xcc, 0xaa},
	}
	for i, c := range Palette {
		if c.A != 0xff {
			t.Errorf("Palette[%d].A = %#x, want 0xff", i, c.A)
		}
		if c.R != want[i][0] || c.G != want[i][1] || c.B != want[i][2] {
			t.Errorf("Palette[%d] = %#v, want %#v", i, c, want[i])
		}
	}
}

func TestSurfaceBoundsAreNoOps(t *testing.T) {
	s := NewSurface(8, 8)
	s.Set(-1, 0, 7)
	s.Set(8, 0, 7)
	s.Set(0, -1, 7)
	s.Set(0, 8, 7)
	s.FillRect(-100, -100, 2, 2, 7)
	s.InvertRect(100, 100, 4, 4)
	if got := s.At(0, 0); got != 0 {
		t.Errorf("At(0,0) = %d after out-of-bounds drawing, want 0", got)
	}

	s.FillRect(-2, -2, 4, 4, 3)
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			if got := s.At(x, y); got != 3 {
				t.Errorf("At(%d,%d) = %d, want 3", x, y, got)
			}
		}
	}
	if got := s.At(2, 2); got != 0 {
		t.Errorf("At(2,2) = %d after clipped fill, want 0", got)
	}
}

func TestSurfaceClearAndInvert(t *testing.T) {
	s := NewSurface(4, 2)
	s.Clear(7)
	s.InvertRect(1, 0, 2, 2)
	want := [][]uint8{
		{7, 8, 8, 7},
		{7, 8, 8, 7},
	}
	for y, row := range want {
		for x, v := range row {
			if got := s.At(x, y); got != v {
				t.Errorf("At(%d,%d) = %d, want %d", x, y, got, v)
			}
		}
	}
}

func TestDrawGlyphNarrowAndWide(t *testing.T) {
	s := NewSurface(16, 8)
	if got := s.DrawGlyph(0, 0, font.LookupByte('A'), 7, 0); got != 4 {
		t.Fatalf("DrawGlyph(A) = %d, want 4", got)
	}
	wantRows := []string{
		"....",
		".##.",
		"#.#.",
		"###.",
		"#.#.",
		"....",
	}
	for y, want := range wantRows {
		if got := rowString(s, 0, y, 4); got != want {
			t.Errorf("row %d = %q, want %q", y, got, want)
		}
	}

	if got := s.DrawGlyph(4, 0, font.LookupByte(0x80), 7, 0); got != 8 {
		t.Fatalf("DrawGlyph(0x80) = %d, want 8", got)
	}
	for y := 0; y < 5; y++ {
		if got := rowString(s, 4, y, 8); got != "#######." {
			t.Errorf("wide row %d = %q, want seven ink pixels and one gap", y, got)
		}
	}
	if got := rowString(s, 4, 5, 8); got != "........" {
		t.Errorf("wide row 5 = %q, want blank", got)
	}
}

func TestDrawTextAdvancesAndSkipsUnknownAndControls(t *testing.T) {
	s := NewSurface(32, 8)
	if got := s.DrawText(0, 0, "A\x00B", 7, 0); got != 2 {
		t.Fatalf("DrawText(A\\x00B) = %d cells, want 2", got)
	}
	if got := s.At(0, 1); got != 0 {
		t.Errorf("control byte advanced the cursor: At(0,1) = %d", got)
	}
	if got := s.At(4, 1); got != 7 {
		t.Errorf("B did not draw after the skipped control: At(4,1) = %d", got)
	}

	before := countInk(s)
	s.DrawText(12, 0, "\u2603", 7, 0)
	if countInk(s) == before {
		t.Fatal("unknown rune drew nothing")
	}
}

func TestDrawTextLogsUnknownOnce(t *testing.T) {
	var logs bytes.Buffer
	s := NewSurface(16, 8)
	s.Logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	s.DrawText(0, 0, "\u2603\u2603", 7, 0)
	if got := strings.Count(logs.String(), "unknown glyph"); got != 1 {
		t.Errorf("unknown glyph log count = %d, want 1\n%s", got, logs.String())
	}
}

func TestDrawTextWideAdvance(t *testing.T) {
	s := NewSurface(32, 8)
	if got := s.DrawText(0, 0, "A🐱B", 7, 0); got != 4 {
		t.Fatalf("DrawText(A🐱B) = %d cells, want 4", got)
	}
	if got := s.At(4, 0); got != 7 {
		t.Errorf("wide glyph did not start at x=4: At(4,0) = %d", got)
	}
	if got := s.At(12, 0); got != 0 {
		t.Errorf("B unexpectedly has ink at row 0: At(12,0) = %d", got)
	}
	if got := s.At(12, 1); got == 0 {
		t.Error("B did not draw at x=12")
	}
}

func TestDrawCursor(t *testing.T) {
	s := NewSurface(16, 8)
	s.DrawCursor(0, 0, 4, CursorBlock)
	for y := 0; y < font.CellHeight; y++ {
		for x := 0; x < 4; x++ {
			if got := s.At(x, y); got != 0x0f {
				t.Errorf("block At(%d,%d) = %d, want 0x0f", x, y, got)
			}
		}
	}
	if got := s.At(4, 0); got != 0 {
		t.Errorf("block cursor drew outside narrow width: At(4,0) = %d", got)
	}

	s.Clear(0)
	s.DrawCursor(0, 0, 8, CursorBlock)
	if got := s.At(7, 5); got != 0x0f {
		t.Errorf("wide block At(7,5) = %d, want 0x0f", got)
	}

	s.Clear(0)
	s.DrawCursor(0, 0, 4, CursorBar)
	for y := 0; y < font.CellHeight; y++ {
		for x := 0; x < 2; x++ {
			if got := s.At(x, y); got != DefaultForeground {
				t.Errorf("bar At(%d,%d) = %d, want %d", x, y, got, DefaultForeground)
			}
		}
	}
	if got := s.At(2, 0); got != 0 {
		t.Errorf("bar At(2,0) = %d, want 0", got)
	}

	s.Clear(0)
	s.DrawCursor(0, 0, 2, CursorBlock)
	if got := s.At(1, 0); got != 0x0f {
		t.Errorf("EOL block At(1,0) = %d, want 0x0f", got)
	}
	if got := s.At(2, 0); got != 0 {
		t.Errorf("EOL block At(2,0) = %d, want 0", got)
	}
}

func rowString(s *Surface, x, y, width int) string {
	var out strings.Builder
	out.Grow(width)
	for i := 0; i < width; i++ {
		if s.At(x+i, y) != 0 {
			out.WriteByte('#')
		} else {
			out.WriteByte('.')
		}
	}
	return out.String()
}

func countInk(s *Surface) int {
	n := 0
	for _, v := range s.Pix {
		if v != 0 {
			n++
		}
	}
	return n
}
