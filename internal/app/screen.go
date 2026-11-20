package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/asimmons91/pico-editor/internal/editor"
	"github.com/asimmons91/pico-editor/internal/font"
	"github.com/asimmons91/pico-editor/internal/render"
	"github.com/asimmons91/pico-editor/internal/vim"
)

func (s *Session) drawBuffer(surface *render.Surface, ed *editor.Editor) {
	viewport := &ed.Viewport
	matches := s.Vim.SearchHighlights(ed)
	for _, visible := range viewport.VisibleLines(ed.Buffer) {
		line := ed.Buffer.Lines[visible.Line]
		y := visible.Row * font.LineAdvance
		for cluster := range editor.LineClusters(line, ed.Buffer.TabWidth) {
			x := (cluster.Cell - viewport.LeftCell) * font.NarrowWidth
			if x >= surface.Width {
				break
			}
			if x+cluster.Pixels <= 0 {
				continue
			}
			selected := clusterSelected(ed, visible.Line, cluster.Rune)
			searched := clusterSearched(matches, visible.Line, cluster.Rune)
			foreground := uint8(render.DefaultForeground)
			background := uint8(render.DefaultBackground)
			if searched {
				background = searchBackground
			}
			if selected {
				foreground, background = background, selectionBackground
			}
			switch {
			case cluster.Tab:
				if selected || searched {
					surface.FillRect(x, y, cluster.Pixels, font.CellHeight, background)
				}
			case cluster.Known && cluster.Glyph.Printable:
				surface.DrawGlyph(x, y, font.LookupByte(cluster.Glyph.Byte), foreground, background)
			case cluster.Known:
				// P8SCII control glyphs occupy no cell and draw nothing.
			default:
				surface.DrawGlyph(x, y, font.Replacement, foreground, background)
			}
		}
	}
}

func (s *Session) drawCursor(surface *render.Surface, ed *editor.Editor) {
	if _, _, _, active := s.Vim.Prompt(); active {
		return
	}
	position := ed.Cursor.Pos
	viewport := &ed.Viewport
	if position.Line < viewport.TopLine || position.Line >= viewport.TopLine+viewport.Height {
		return
	}
	info := ed.Buffer.Info(position)
	x := (info.Cell - viewport.LeftCell) * font.NarrowWidth
	y := (position.Line - viewport.TopLine) * font.LineAdvance
	cells := ed.Buffer.ClusterAt(position).Cells
	width := cells * font.NarrowWidth
	if width <= 0 {
		width = font.NarrowWidth
	}
	if x >= surface.Width || x+width <= 0 {
		return
	}
	style := render.CursorBlock
	switch s.Vim.Mode {
	case vim.ModeInsert:
		style = render.CursorBar
		width = 2
	}
	surface.DrawCursor(x, y, width, style)
}

func clusterSelected(ed *editor.Editor, line, runeOffset int) bool {
	selection := ed.Selection
	switch selection.Kind {
	case editor.SelectionLine:
		first, last := selection.LineRange()
		return line >= first && line <= last
	case editor.SelectionBlock:
		start, end, ok := selection.BlockLineRange(ed.Buffer, line)
		if !ok {
			return false
		}
		position := editor.Pos{Line: line, Rune: runeOffset}
		return !position.Before(start) && position.Before(end)
	case editor.SelectionChar:
		start, end := selection.Normalized()
		position := editor.Pos{Line: line, Rune: runeOffset}
		return !position.Before(start) && !end.Before(position)
	default:
		return false
	}
}

func clusterSearched(matches []editor.Match, line, runeOffset int) bool {
	position := editor.Pos{Line: line, Rune: runeOffset}
	for _, match := range matches {
		if !position.Before(match.Start) && position.Before(match.End) {
			return true
		}
	}
	return false
}

func (s *Session) drawStatus(surface *render.Surface, ed *editor.Editor, cursorVisible bool) {
	y := statusRow * font.LineAdvance
	surface.FillRect(0, y, surface.Width, font.CellHeight, render.DefaultBackground)

	if prefix, text, cursor, active := s.Vim.Prompt(); active {
		surface.DrawText(0, y, prefix, render.DefaultForeground, render.DefaultBackground)
		surface.DrawText(font.NarrowWidth, y, text, render.DefaultForeground, render.DefaultBackground)
		if cursorVisible {
			before := string([]rune(text)[:min(cursor, len([]rune(text)))])
			cell := 1 + font.CellWidth(before)
			surface.DrawCursor(cell*font.NarrowWidth, y, 2, render.CursorBar)
		}
		return
	}

	mode := s.Vim.Mode.String()
	x := surface.DrawText(0, y, mode+"  ", 10, render.DefaultBackground)

	name := ed.Buffer.Path
	if name == "" {
		name = "[No Name]"
	} else {
		name = filepath.Base(name)
	}
	marker := ""
	if ed.Buffer.Dirty() {
		marker = " *"
	}
	info := ed.Buffer.Info(ed.Cursor.Pos)
	position := fmt.Sprintf("%d:%d", info.Pos.Line+1, info.Cell+1)
	status := fmt.Sprintf("%s%s  %s", name, marker, position)
	if s.Status != "" {
		status += "  " + s.Status
	}
	color := uint8(statusForeground)
	if s.StatusError {
		color = statusErrorColor
	}
	surface.DrawText(x*font.NarrowWidth, y, status, color, render.DefaultBackground)
}

func (s *Session) drawHelp(surface *render.Surface) {
	const panelWidth = 58
	const panelHeight = 34
	width := panelWidth * font.NarrowWidth
	height := panelHeight * font.LineAdvance
	x := (surface.Width - width) / 2
	y := (surface.Height - height) / 2

	surface.FillRect(x, y, width, height, render.DefaultBackground)
	surface.FillRect(x, y, width, 1, 6)
	surface.FillRect(x, y+height-1, width, 1, 6)
	surface.FillRect(x, y, 1, height, 6)
	surface.FillRect(x+width-1, y, 1, height, 6)
	surface.DrawText(x+4, y+4, clipCells(s.Help.Title, panelWidth-8), 10, render.DefaultBackground)

	offset := 4 * font.CellHeight
	maxLines := (height - offset - font.CellHeight) / font.LineAdvance
	if maxLines < 1 {
		return
	}
	if s.Help.Top < 0 {
		s.Help.Top = 0
	}
	if s.Help.Top > max(0, len(s.Help.Lines)-maxLines) {
		s.Help.Top = max(0, len(s.Help.Lines)-maxLines)
	}
	for index := 0; index < maxLines; index++ {
		lineIndex := s.Help.Top + index
		if lineIndex >= len(s.Help.Lines) {
			break
		}
		surface.DrawText(x+4, y+offset+index*font.LineAdvance, clipCells(s.Help.Lines[lineIndex], panelWidth-8), 7, render.DefaultBackground)
	}
}

// clipCells truncates text to a display-cell budget using the P8SCII width
// table, never splitting a cluster.
func clipCells(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	cells := 0
	var out strings.Builder
	for cluster := range font.Clusters(text) {
		width := cluster.Width / font.NarrowWidth
		if cells+width > limit {
			break
		}
		cells += width
		out.WriteString(cluster.Text)
	}
	return out.String()
}
