// Package font owns the checked-in PICO-8 1-bit glyph atlas and its metrics.
package font

import (
	"iter"
	"strings"
	"unicode/utf8"

	"github.com/asimmons91/pico-lsp/pkg/p8scii"
)

//go:generate go run ../../tools/fontgen -data glyphs.txt -out atlas_gen.go -png atlas.png

// Glyph metrics are locked by the V1 canvas decision: a 4x6 cell, with wide
// P8SCII glyphs occupying two cells. There is no baseline offset in V1.
const (
	NarrowWidth = 4
	WideWidth   = 8
	CellHeight  = 6
	LineAdvance = 6
)

// Glyph is one 1-bit glyph tile. Bit 0 of each row is the leftmost pixel.
type Glyph struct {
	Width uint8
	Rows  [6]uint8
}

// Replacement is drawn for an unknown rune or cluster.
var Replacement = Glyph{
	Width: NarrowWidth,
	Rows:  [6]uint8{0x0f, 0x09, 0x09, 0x09, 0x0f, 0x00},
}

// Cluster is one display unit of canonical Unicode text.
type Cluster struct {
	// Text is the exact source text for the cluster, including a trailing
	// U+FE0F when present.
	Text string
	// Meta is the P8SCII metadata for the cluster. Byte, Width, and Printable
	// are meaningful only when Known is true.
	Meta p8scii.Glyph
	// Shape is the local glyph bitmap. Unknown clusters use Replacement.
	Shape Glyph
	// Known reports whether p8scii recognized the cluster.
	Known bool
	// Width is the pixel advance at 1x: 0 for controls, 4 for narrow glyphs,
	// 8 for wide glyphs, and 4 for unknown clusters.
	Width int
}

// LookupByte returns the glyph for a P8SCII byte. Every byte has an entry.
func LookupByte(b byte) Glyph {
	return glyphs[b]
}

// Lookup returns the glyph for canonical Unicode text.
func Lookup(text string) (Glyph, bool) {
	meta, ok := p8scii.GlyphFor(text)
	if !ok {
		return Glyph{}, false
	}
	return glyphs[meta.Byte], true
}

// Clusters iterates display clusters in text. A base rune and one trailing
// U+FE0F form one cluster; invalid UTF-8 bytes each become one unknown cluster.
func Clusters(text string) iter.Seq[Cluster] {
	return func(yield func(Cluster) bool) {
		for i := 0; i < len(text); {
			r, size := utf8.DecodeRuneInString(text[i:])
			if r == utf8.RuneError && size == 1 {
				if !yield(Cluster{
					Text:  string(r),
					Shape: Replacement,
					Width: NarrowWidth,
				}) {
					return
				}
				i++
				continue
			}

			end := i + size
			if r != '\ufe0f' && strings.HasPrefix(text[end:], "\ufe0f") {
				end += len("\ufe0f")
			}
			cluster := text[i:end]
			meta, known := p8scii.GlyphFor(cluster)
			shape := Replacement
			width := NarrowWidth
			if known {
				shape = glyphs[meta.Byte]
				width = meta.Width
			}
			if !yield(Cluster{
				Text:  cluster,
				Meta:  meta,
				Shape: shape,
				Known: known,
				Width: width,
			}) {
				return
			}
			i = end
		}
	}
}

// CellWidth returns the advance of text in 4-pixel display cells. Controls
// advance zero cells; unknown clusters advance one.
func CellWidth(text string) int {
	cells := 0
	for cluster := range Clusters(text) {
		cells += cluster.Width / NarrowWidth
	}
	return cells
}
