package app

import (
	"fmt"

	"github.com/asimmons91/pico-lsp/pkg/p8scii"

	"github.com/asimmons91/pico-editor/internal/font"
	"github.com/asimmons91/pico-editor/internal/render"
)

const (
	charsetTiles = 16
	charsetTile  = 16
)

// Charset is the M1 full-charset debug screen. It is scaffolding kept behind
// --debug-charset after M1.
type Charset struct {
	Selected      byte
	Overlay       bool
	Background    uint8
	CursorVisible bool
	Log           []string
}

// NewCharset returns the debug screen with a useful initial selection.
func NewCharset() *Charset {
	return &Charset{Selected: 0x41}
}

// Move changes the selected byte by dx/dy tiles and clamps to the charset.
func (c *Charset) Move(dx, dy int) {
	x := int(c.Selected)%charsetTiles + dx
	y := int(c.Selected)/charsetTiles + dy
	x = min(max(x, 0), charsetTiles-1)
	y = min(max(y, 0), charsetTiles-1)
	c.Selected = byte(y*charsetTiles + x)
}

// CycleBackground advances through all 16 palette colors.
func (c *Charset) CycleBackground() {
	c.Background = (c.Background + 1) & 0x0f
}

// AddLog records one input event for the debug pane.
func (c *Charset) AddLog(line string) {
	c.Log = append(c.Log, line)
	if len(c.Log) > 3 {
		c.Log = c.Log[len(c.Log)-3:]
	}
}

// Draw composes the screen into the software surface.
func (c *Charset) Draw(s *render.Surface) {
	bg := c.Background & 0x0f
	fg := contrast(7, bg)
	grid := contrast(5, bg)
	accent := contrast(10, bg)
	s.Clear(bg)

	for b := 0; b < 256; b++ {
		x := (b % charsetTiles) * charsetTile
		y := (b / charsetTiles) * charsetTile
		meta := p8scii.Lookup(byte(b))
		if meta.Printable {
			s.DrawGlyph(x+4, y+2, font.LookupByte(byte(b)), fg, bg)
		}
		s.DrawText(x+2, y+9, fmt.Sprintf("%02X", b), fg, bg)
		if c.Overlay {
			pixels, cells := widthLabels(meta.Width)
			s.DrawText(x+12, y+2, pixels, accent, bg)
			s.DrawText(x+12, y+9, cells, accent, bg)
			// Draw borders after the labels so their background cells do not
			// erase the right and bottom edges.
			s.FillRect(x, y, charsetTile, 1, grid)
			s.FillRect(x, y+charsetTile-1, charsetTile, 1, grid)
			s.FillRect(x, y, 1, charsetTile, grid)
			s.FillRect(x+charsetTile-1, y, 1, charsetTile, grid)
		}
	}

	x := int(c.Selected) % charsetTiles * charsetTile
	y := int(c.Selected) / charsetTiles * charsetTile
	if c.CursorVisible {
		width := p8scii.Lookup(c.Selected).Width
		if width == 0 {
			width = 2
		}
		s.DrawCursor(x+4, y+2, width, render.CursorBlock)
	}
	s.FillRect(x, y, charsetTile, 1, accent)
	s.FillRect(x, y+charsetTile-1, charsetTile, 1, accent)
	s.FillRect(x, y, 1, charsetTile, accent)
	s.FillRect(x+charsetTile-1, y, 1, charsetTile, accent)

	if !c.showLog() {
		return
	}
	meta := p8scii.Lookup(c.Selected)
	s.FillRect(0, 232, s.Width, 24, 0)
	s.DrawText(2, 232, fmt.Sprintf("byte %02X  text %q  width %d", c.Selected, meta.Text, meta.Width), 7, 0)
	for i, line := range c.Log {
		s.DrawText(2, 238+i*6, line, 6, 0)
	}
}

func (c *Charset) showLog() bool {
	return len(c.Log) > 0 && c.Selected < 0xe0
}

// widthLabels returns the overlay's pixel and cell counts. Control bytes are
// zero pixels and zero cells.
func widthLabels(width int) (pixels, cells string) {
	return fmt.Sprintf("%d", width), fmt.Sprintf("%d", width/font.NarrowWidth)
}

func contrast(index, bg uint8) uint8 {
	if index == bg {
		return 0
	}
	return index
}
