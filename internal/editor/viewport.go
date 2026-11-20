package editor

// Viewport is the visible cell window into the buffer. TopLine is the first
// visible line; LeftCell is the first visible display cell.
type Viewport struct {
	TopLine   int
	LeftCell  int
	Width     int // cells
	Height    int // rows
	ScrollOff int
}

// VisibleLine maps a buffer line to a screen row.
type VisibleLine struct {
	Line int
	Row  int
}

// EnsureVisible scrolls the viewport so pos and its cluster are visible,
// keeping ScrollOff lines and cells around the cursor when possible.
func (v *Viewport) EnsureVisible(pos Pos, cell, cells, lineCount int) {
	if v.Width < 1 {
		v.Width = 1
	}
	if v.Height < 1 {
		v.Height = 1
	}
	if cells < 1 {
		cells = 1
	}

	verticalOffset := min(v.ScrollOff, max(0, (v.Height-1)/2))
	if pos.Line < v.TopLine+verticalOffset {
		v.TopLine = pos.Line - verticalOffset
	}
	if pos.Line > v.TopLine+v.Height-1-verticalOffset {
		v.TopLine = pos.Line - (v.Height - 1 - verticalOffset)
	}
	maxTop := max(0, lineCount-v.Height)
	v.TopLine = min(max(v.TopLine, 0), maxTop)

	horizontalOffset := min(v.ScrollOff, max(0, (v.Width-1)/2))
	if cell < v.LeftCell+horizontalOffset {
		v.LeftCell = cell - horizontalOffset
	}
	if cell+cells > v.LeftCell+v.Width-horizontalOffset {
		v.LeftCell = cell + cells - v.Width + horizontalOffset
	}
	// A cluster wider than the viewport cannot be fully shown; keep its first
	// cell visible instead of scrolling the cursor off the left edge.
	if v.LeftCell > cell {
		v.LeftCell = cell
	}
	v.LeftCell = max(v.LeftCell, 0)
}

// VisibleLines returns the buffer lines and screen rows currently visible.
func (v *Viewport) VisibleLines(buffer *Buffer) []VisibleLine {
	if len(buffer.Lines) == 0 || v.Height < 1 {
		return nil
	}
	top := min(max(v.TopLine, 0), max(0, len(buffer.Lines)-1))
	bottom := min(top+v.Height, len(buffer.Lines))
	lines := make([]VisibleLine, 0, bottom-top)
	for line := top; line < bottom; line++ {
		lines = append(lines, VisibleLine{Line: line, Row: line - top})
	}
	return lines
}
