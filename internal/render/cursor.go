package render

import "github.com/asimmons91/pico-editor/internal/font"

// CursorStyle is the requested cursor presentation.
type CursorStyle uint8

const (
	// CursorBlock inverts the glyph cells under the cursor.
	CursorBlock CursorStyle = iota
	// CursorBar is a two-pixel bar at the cursor's left edge.
	CursorBar
)

const cursorBarWidth = 2

// DrawCursor draws a cursor without owning blink state. width is the pixel
// width under the cursor (4 for narrow, 8 for wide, or 2 for end-of-line).
func (s *Surface) DrawCursor(x, y, width int, style CursorStyle) {
	switch style {
	case CursorBar:
		s.FillRect(x, y, cursorBarWidth, font.CellHeight, DefaultForeground)
	default:
		if width <= 0 {
			width = font.NarrowWidth
		}
		s.InvertRect(x, y, width, font.CellHeight)
	}
}
