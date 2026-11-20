// Package render owns the PICO-8 palette and the indexed software surface.
package render

import (
	"log/slog"

	"github.com/asimmons91/pico-editor/internal/font"
)

// Surface is a 256x256 indexed software image. All drawing is clipped; an
// out-of-bounds operation is a no-op.
type Surface struct {
	Pix    []uint8 // row-major palette indices
	Width  int
	Height int

	// Logger receives one debug record per unknown rune. Nil discards them.
	Logger *slog.Logger

	unknown map[rune]struct{}
}

// NewSurface returns a cleared surface of the given size. V1 always uses
// CanvasWidth x CanvasHeight from internal/app.
func NewSurface(width, height int) *Surface {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	return &Surface{
		Pix:     make([]uint8, width*height),
		Width:   width,
		Height:  height,
		unknown: make(map[rune]struct{}),
	}
}

// Clear fills the whole surface with a palette index.
func (s *Surface) Clear(index uint8) {
	for i := range s.Pix {
		s.Pix[i] = index & 0x0f
	}
}

// Set writes one pixel. Out-of-bounds writes are ignored.
func (s *Surface) Set(x, y int, index uint8) {
	if x < 0 || y < 0 || x >= s.Width || y >= s.Height {
		return
	}
	s.Pix[y*s.Width+x] = index & 0x0f
}

// At returns one pixel, or 0 outside the surface.
func (s *Surface) At(x, y int) uint8 {
	if x < 0 || y < 0 || x >= s.Width || y >= s.Height {
		return 0
	}
	return s.Pix[y*s.Width+x]
}

// FillRect fills a clipped rectangle with a palette index.
func (s *Surface) FillRect(x, y, w, h int, index uint8) {
	if w <= 0 || h <= 0 {
		return
	}
	x0, y0, x1, y1 := s.clipRect(x, y, w, h)
	index &= 0x0f
	for row := y0; row < y1; row++ {
		base := row * s.Width
		for col := x0; col < x1; col++ {
			s.Pix[base+col] = index
		}
	}
}

// InvertRect XORs the covered palette indices with 0x0f. This is the PICO-8
// block-cursor inversion and needs no alpha blending.
func (s *Surface) InvertRect(x, y, w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	x0, y0, x1, y1 := s.clipRect(x, y, w, h)
	for row := y0; row < y1; row++ {
		base := row * s.Width
		for col := x0; col < x1; col++ {
			s.Pix[base+col] ^= 0x0f
		}
	}
}

// DrawGlyph fills the glyph cell with bg, then draws its ink in fg. It returns
// the glyph's pixel width.
func (s *Surface) DrawGlyph(x, y int, g font.Glyph, fg, bg uint8) int {
	width := int(g.Width)
	if width <= 0 {
		return 0
	}
	s.FillRect(x, y, width, font.CellHeight, bg)
	for row := 0; row < font.CellHeight; row++ {
		bits := g.Rows[row]
		for col := 0; col < width; col++ {
			if bits&(1<<col) != 0 {
				s.Set(x+col, y+row, fg)
			}
		}
	}
	return width
}

func (s *Surface) clipRect(x, y, w, h int) (x0, y0, x1, y1 int) {
	x0 = max(0, x)
	y0 = max(0, y)
	x1 = min(s.Width, x+w)
	y1 = min(s.Height, y+h)
	if x1 < x0 {
		x1 = x0
	}
	if y1 < y0 {
		y1 = y0
	}
	return x0, y0, x1, y1
}
