package editor

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/asimmons91/pico-lsp/pkg/p8scii"
)

func TestLineClustersWidths(t *testing.T) {
	t.Parallel()

	line := "a\t♥⬅️😐️\t"
	want := []struct {
		text  string
		rune  int
		cell  int
		cells int
	}{
		{"a", 0, 0, 1},
		{"\t", 1, 1, 1},
		{"♥", 2, 2, 2},
		{"⬅️", 3, 4, 2},
		{"😐️", 5, 6, 2},
		{"\t", 7, 8, 2},
	}

	var got []LineCluster
	for cluster := range LineClusters(line, 2) {
		got = append(got, cluster)
	}
	if len(got) != len(want) {
		t.Fatalf("LineClusters(%q) returned %d clusters, want %d: %+v", line, len(got), len(want), got)
	}
	for i, expected := range want {
		if got[i].Text != expected.text {
			t.Errorf("cluster %d text = %q, want %q", i, got[i].Text, expected.text)
		}
		if got[i].Rune != expected.rune {
			t.Errorf("cluster %d rune = %d, want %d", i, got[i].Rune, expected.rune)
		}
		if got[i].Cell != expected.cell {
			t.Errorf("cluster %d cell = %d, want %d", i, got[i].Cell, expected.cell)
		}
		if got[i].Cells != expected.cells {
			t.Errorf("cluster %d cells = %d, want %d", i, got[i].Cells, expected.cells)
		}
	}
}

func TestLineClustersTabStops(t *testing.T) {
	t.Parallel()

	cases := []struct {
		line  string
		cells []int
	}{
		{"\t", []int{2}},
		{"a\t", []int{1, 1}},
		{"ab\t", []int{1, 1, 2}},
		{"\t\t", []int{2, 2}},
	}
	for _, test := range cases {
		var cells []int
		for cluster := range LineClusters(test.line, 2) {
			cells = append(cells, cluster.Cells)
		}
		if len(cells) != len(test.cells) {
			t.Errorf("LineClusters(%q) = %v, want %v", test.line, cells, test.cells)
			continue
		}
		for i := range cells {
			if cells[i] != test.cells[i] {
				t.Errorf("LineClusters(%q)[%d] cells = %d, want %d", test.line, i, cells[i], test.cells[i])
			}
		}
	}
}

func TestPositionConversions(t *testing.T) {
	t.Parallel()

	buffer := New("")
	buffer.Lines = []string{"a\t♥⬅️😐️\tb", "second"}
	buffer.TrailingNewline = true

	boundaries := []Pos{
		{0, 0}, {0, 1}, {0, 2}, {0, 3}, {0, 5}, {0, 7}, {0, 8}, {0, 9},
		{1, 0}, {1, 6},
	}
	for _, position := range boundaries {
		info := buffer.Info(position)
		got, err := buffer.PosFromByte(info.Byte)
		if err != nil {
			t.Fatalf("PosFromByte(%d) for %v: %v", info.Byte, position, err)
		}
		if got != position {
			t.Errorf("byte round-trip %v -> %d -> %v", position, info.Byte, got)
		}
	}

	// A VS16 is part of the preceding glyph and is not a cursor position.
	if buffer.NormalizePos(Pos{Line: 0, Rune: 4}) == (Pos{Line: 0, Rune: 4}) {
		t.Error("NormalizePos kept a cursor inside a VS16 sequence")
	}
	// Byte offsets inside a cluster snap back to a boundary instead of
	// splitting the glyph. The VS16 bytes start at byte 3 of "⬅️".
	vs := New("")
	vs.Lines = []string{"⬅️"}
	snapped, err := vs.PosFromByte(len("⬅"))
	if err != nil {
		t.Fatalf("PosFromByte inside a VS16 cluster: %v", err)
	}
	if snapped != (Pos{Line: 0, Rune: 0}) {
		t.Errorf("PosFromByte inside a VS16 cluster = %v, want the cluster start", snapped)
	}
}

func TestPosFromByteTrailingNewline(t *testing.T) {
	t.Parallel()

	buffer := New("")
	buffer.Lines = []string{"a"}
	buffer.TrailingNewline = true
	if got := buffer.Text(); got != "a\n" {
		t.Fatalf("Text() = %q, want a\\n", got)
	}
	got, err := buffer.PosFromByte(len(buffer.Text()))
	if err != nil {
		t.Fatalf("PosFromByte(end): %v", err)
	}
	if got != (Pos{Line: 0, Rune: 1}) {
		t.Errorf("PosFromByte(end) = %v, want the last line's end", got)
	}

	empty := New("")
	empty.Lines = []string{""}
	empty.TrailingNewline = true
	got, err = empty.PosFromByte(len(empty.Text()))
	if err != nil {
		t.Fatalf("PosFromByte(end) on newline-only buffer: %v", err)
	}
	if got != (Pos{}) {
		t.Errorf("PosFromByte(end) = %v, want the origin", got)
	}
}

func TestPositionRoundTripAllP8SCIIGlyphs(t *testing.T) {
	t.Parallel()

	var line strings.Builder
	for i := 0; i < 256; i++ {
		text := p8scii.Lookup(byte(i)).Text
		if text == "\n" {
			continue // a newline is a line separator, never part of a Line
		}
		line.WriteString(text)
	}
	buffer := New("")
	buffer.Lines = []string{line.String()}

	for position := (Pos{}); ; {
		info := buffer.Info(position)
		got, err := buffer.PosFromByte(info.Byte)
		if err != nil {
			t.Fatalf("PosFromByte(%d) for %v: %v", info.Byte, position, err)
		}
		if got != position {
			t.Fatalf("byte round-trip %v -> %d -> %v", position, info.Byte, got)
		}
		next := buffer.NextCluster(position)
		if next == position {
			break
		}
		position = next
	}
}

func TestCellAndUTF16RoundTripAllP8SCIIGlyphs(t *testing.T) {
	t.Parallel()

	var line strings.Builder
	for i := 0; i < 256; i++ {
		text := p8scii.Lookup(byte(i)).Text
		if text == "\n" {
			continue
		}
		line.WriteString(text)
	}
	buffer := New("")
	buffer.Lines = []string{line.String()}

	for cluster := range LineClusters(buffer.Lines[0], buffer.TabWidth) {
		position := Pos{Line: 0, Rune: cluster.Rune}
		info := buffer.Info(position)
		if got := buffer.PosFromUTF16(0, info.UTF16); got != position {
			t.Fatalf("UTF-16 round-trip %v -> %d -> %v", position, info.UTF16, got)
		}
		if cluster.Cells == 0 {
			continue // a zero-width control shares its cell with the next cluster
		}
		if got := buffer.PosFromCell(0, info.Cell); got != position {
			t.Fatalf("cell round-trip %v -> %d -> %v", position, info.Cell, got)
		}
	}

	empty := New("")
	empty.Lines = []string{""}
	if got := empty.PosFromCell(0, 0); got != (Pos{}) {
		t.Errorf("empty line PosFromCell = %v, want origin", got)
	}
	if got := empty.PosFromUTF16(0, 0); got != (Pos{}) {
		t.Errorf("empty line PosFromUTF16 = %v, want origin", got)
	}
}

func TestCellAndUTF16Columns(t *testing.T) {
	t.Parallel()

	buffer := New("")
	buffer.Lines = []string{"a\t♥⬅️😐️\tb"}

	if got := buffer.CellColumn(Pos{Line: 0, Rune: 3}); got != 4 {
		t.Errorf("CellColumn before wide glyph = %d, want 4", got)
	}
	if got := buffer.CellColumn(Pos{Line: 0, Rune: 5}); got != 6 {
		t.Errorf("CellColumn after wide glyph = %d, want 6", got)
	}
	if got := buffer.PosFromCell(0, 4); got != (Pos{Line: 0, Rune: 3}) {
		t.Errorf("PosFromCell inside wide glyph = %v, want the glyph start", got)
	}
	if got := buffer.PosFromCell(0, 6); got != (Pos{Line: 0, Rune: 5}) {
		t.Errorf("PosFromCell after wide glyph = %v, want rune 5", got)
	}

	if got := buffer.UTF16Column(Pos{Line: 0, Rune: 3}); got != 3 {
		t.Errorf("UTF16Column before wide glyph = %d, want 3", got)
	}
	if got := buffer.UTF16Column(Pos{Line: 0, Rune: 5}); got != 5 {
		t.Errorf("UTF16Column after wide glyph = %d, want 5", got)
	}
	if got := buffer.PosFromUTF16(0, 4); got != (Pos{Line: 0, Rune: 3}) {
		t.Errorf("PosFromUTF16 inside surrogate pair = %v, want the glyph start", got)
	}
	if got := buffer.PosFromUTF16(0, 5); got != (Pos{Line: 0, Rune: 5}) {
		t.Errorf("PosFromUTF16 after wide glyph = %v, want rune 5", got)
	}
}

func TestControlGlyphWidth(t *testing.T) {
	t.Parallel()

	control := p8scii.Lookup(1)
	if control.Printable || control.Width != 0 {
		t.Fatalf("byte 1 = %+v, want a non-printable zero-width control", control)
	}
	buffer := New("")
	buffer.Lines = []string{control.Text + "a"}
	if got := buffer.CellColumn(Pos{Line: 0, Rune: 0}); got != 0 {
		t.Errorf("control start cell = %d, want 0", got)
	}
	if got := buffer.CellColumn(Pos{Line: 0, Rune: 1}); got != 0 {
		t.Errorf("cell after control = %d, want 0", got)
	}
	cell := buffer.ClusterAt(Pos{Line: 0, Rune: 0})
	if cell.Cells != 0 {
		t.Errorf("control cells = %d, want 0", cell.Cells)
	}
}

func TestSnapNearestClusterBoundary(t *testing.T) {
	t.Parallel()

	line := "a⬅️b"
	// Runes: a=0, arrow=1, VS16=2, b=3. Offset 2 is inside the arrow cluster.
	if got := snapNearest(line, 2); got != 1 && got != 3 {
		t.Errorf("snapNearest(%q, 2) = %d, want a cluster boundary", line, got)
	}
	if got := snapNearest(line, 1); got != 1 {
		t.Errorf("snapNearest(%q, 1) = %d, want 1", line, got)
	}
	if got := snapNearest(line, 3); got != 3 {
		t.Errorf("snapNearest(%q, 3) = %d, want 3", line, got)
	}
	if utf8.RuneCountInString(line) != 4 {
		t.Fatalf("test line has %d runes, want 4", utf8.RuneCountInString(line))
	}
}

func TestUTF16RangeExpansion(t *testing.T) {
	t.Parallel()

	buffer := New("")
	buffer.Lines = []string{"a⬅️b"}
	// ⬅ is one UTF-16 unit; the trailing VS16 is a second. A column in the
	// middle of the cluster must expand outward for LSP ranges.
	if got := buffer.PosFromUTF16Start(0, 2); got.Rune != 1 {
		t.Errorf("start column 2 = rune %d, want cluster start 1", got.Rune)
	}
	if got := buffer.PosFromUTF16End(0, 2); got.Rune != 3 {
		t.Errorf("end column 2 = rune %d, want cluster end 3", got.Rune)
	}
	if got := buffer.PosFromUTF16Start(0, 3); got.Rune != 3 {
		t.Errorf("start at boundary 3 = rune %d, want 3", got.Rune)
	}
	if got := buffer.PosFromUTF16End(0, 3); got.Rune != 3 {
		t.Errorf("end at boundary 3 = rune %d, want 3", got.Rune)
	}
}
