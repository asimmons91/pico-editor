package font

import (
	"bytes"
	"os"
	"testing"

	"github.com/asimmons91/pico-lsp/pkg/p8scii"

	"github.com/asimmons91/pico-editor/internal/fontgen"
)

func TestGeneratedFilesAreCurrent(t *testing.T) {
	records, err := fontgen.ParseFile("glyphs.txt")
	if err != nil {
		t.Fatalf("ParseFile(glyphs.txt): %v", err)
	}

	wantGo := fontgen.GenerateGo(records)
	gotGo, err := os.ReadFile("atlas_gen.go")
	if err != nil {
		t.Fatalf("read atlas_gen.go: %v", err)
	}
	if !bytes.Equal(gotGo, wantGo) {
		t.Error("atlas_gen.go is stale; run go generate ./internal/font")
	}

	wantPNG, err := fontgen.GeneratePNG(records)
	if err != nil {
		t.Fatalf("GeneratePNG: %v", err)
	}
	gotPNG, err := os.ReadFile("atlas.png")
	if err != nil {
		t.Fatalf("read atlas.png: %v", err)
	}
	if !bytes.Equal(gotPNG, wantPNG) {
		t.Error("atlas.png is stale; run go generate ./internal/font")
	}
}

func TestEveryByteMatchesP8SCII(t *testing.T) {
	for b := 0; b < 256; b++ {
		meta := p8scii.Lookup(byte(b))
		glyph := LookupByte(byte(b))
		if meta.Width == 8 {
			if glyph.Width != WideWidth {
				t.Errorf("byte %#02x: local width = %d, want %d", b, glyph.Width, WideWidth)
			}
		} else if glyph.Width != NarrowWidth {
			t.Errorf("byte %#02x: local width = %d, want %d", b, glyph.Width, NarrowWidth)
		}
		if got, ok := Lookup(meta.Text); !ok || got != glyph {
			t.Errorf("Lookup(%q) = (%+v, %t), want byte %#02x glyph", meta.Text, got, ok, b)
		}
		if got := p8scii.Encode(meta.Text); !bytes.Equal(got, []byte{byte(b)}) {
			t.Errorf("p8scii.Encode(%q) = %v, want [%#02x]", meta.Text, got, b)
		}
		for row, bits := range glyph.Rows {
			limit := uint8(0x0f)
			if glyph.Width == WideWidth {
				limit = 0xff
			}
			if bits&^limit != 0 {
				t.Errorf("byte %#02x row %d = %#02x, ink outside width %d", b, row, bits, glyph.Width)
			}
		}
	}
}

func TestLookupUnknown(t *testing.T) {
	if glyph, ok := Lookup("not a glyph"); ok {
		t.Errorf("Lookup(unknown) = (%+v, true), want false", glyph)
	}
	if got, ok := Lookup("😐️"); !ok || got != LookupByte(0x8c) {
		t.Errorf("Lookup(😐 + VS16) = (%+v, %t), want byte 0x8c", got, ok)
	}
}

func TestCellWidth(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"", 0},
		{"A", 1},
		{"ABC", 3},
		{"🐱", 2},
		{"⬅️", 2},
		{"😐", 2},
		{"😐️", 2},
		{"A🐱B", 4},
		{"\x00", 0},
		{"\t", 0},
		{"\x00A", 1},
		{"é", 1},
		{string([]byte{0xff}), 1},
		{string([]byte{0xff, 0xfe}), 2},
	}
	for _, tc := range cases {
		if got := CellWidth(tc.text); got != tc.want {
			t.Errorf("CellWidth(%q) = %d, want %d", tc.text, got, tc.want)
		}
	}
}

func TestClustersGroupVS16(t *testing.T) {
	var clusters []Cluster
	for cluster := range Clusters("⬅️A😐️") {
		clusters = append(clusters, cluster)
	}
	if len(clusters) != 3 {
		t.Fatalf("got %d clusters, want 3: %+v", len(clusters), clusters)
	}
	if clusters[0].Meta.Byte != 0x8b || clusters[0].Width != 8 || !clusters[0].Known {
		t.Errorf("first cluster = %+v, want byte 0x8b and width 8", clusters[0])
	}
	if clusters[1].Meta.Byte != 'A' || clusters[1].Width != 4 {
		t.Errorf("second cluster = %+v, want byte 'A' and width 4", clusters[1])
	}
	if clusters[2].Meta.Byte != 0x8c || clusters[2].Width != 8 {
		t.Errorf("third cluster = %+v, want byte 0x8c and width 8", clusters[2])
	}
}

func TestClustersInvalidUTF8(t *testing.T) {
	var clusters []Cluster
	for cluster := range Clusters(string([]byte{0xff, 'A'})) {
		clusters = append(clusters, cluster)
	}
	if len(clusters) != 2 {
		t.Fatalf("got %d clusters, want 2", len(clusters))
	}
	if clusters[0].Known || clusters[0].Width != NarrowWidth || clusters[0].Text != "\ufffd" {
		t.Errorf("invalid cluster = %+v, want unknown 4px replacement", clusters[0])
	}
	if !clusters[1].Known || clusters[1].Meta.Byte != 'A' {
		t.Errorf("valid cluster = %+v, want 'A'", clusters[1])
	}
}

func TestControlClustersAreNotPrintable(t *testing.T) {
	for cluster := range Clusters("\x00\t\n") {
		if !cluster.Known || cluster.Meta.Printable || cluster.Width != 0 {
			t.Errorf("control cluster = %+v, want known, non-printable, width 0", cluster)
		}
	}
}
