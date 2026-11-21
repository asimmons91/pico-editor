package app

import (
	"fmt"
	"strings"

	"github.com/asimmons91/pico-editor/internal/editor"
	"github.com/asimmons91/pico-editor/internal/font"
	"github.com/asimmons91/pico-editor/internal/lsp"
	"github.com/asimmons91/pico-editor/internal/render"
	"github.com/asimmons91/pico-editor/internal/vim"
)

func (s *Session) drawBuffer(surface *render.Surface, ed *editor.Editor, topRow, first, last, height int) {
	viewport := &ed.Viewport
	matches := s.Vim.SearchHighlights(ed)
	current := currentSearchMatch(matches, ed.Cursor.Pos)
	documentURI := s.activeURI()
	tokens := s.LSPTokens[documentURI]
	baseLine := 0
	if buffer := s.CurrentBuffer(); buffer != nil {
		baseLine = s.baseLine(buffer)
	}
	for _, visible := range viewport.VisibleLines(ed.Buffer) {
		if visible.Line < first || visible.Line >= last || visible.Row >= height {
			continue
		}
		line := ed.Buffer.Lines[visible.Line]
		y := (topRow + visible.Row) * font.LineAdvance
		currentLine := visible.Line == ed.Cursor.Pos.Line
		if currentLine {
			surface.FillRect(0, y, surface.Width, font.CellHeight, currentLineBackground)
		}
		if ed.Selection.Kind == editor.SelectionLine {
			firstLine, lastLine := ed.Selection.LineRange()
			if visible.Line >= firstLine && visible.Line <= lastLine {
				surface.FillRect(0, y, surface.Width, font.CellHeight, textSelectionBackground)
			}
		}
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
			thisMatch := current >= 0 && clusterSearched(matches[current:current+1], visible.Line, cluster.Rune)
			foreground := uint8(render.DefaultForeground)
			if color, ok := tokens.ColorAt(visible.Line, cluster.Rune); ok {
				foreground = color
			}
			background := uint8(render.DefaultBackground)
			if currentLine {
				background = currentLineBackground
			}
			if searched {
				background = searchBackground
			}
			if thisMatch && !selected {
				foreground, background = searchCurrentForeground, searchCurrentBackground
			}
			if selected {
				background = textSelectionBackground
			}
			switch {
			case cluster.Tab:
				if background != render.DefaultBackground {
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
		s.drawDiagnosticsForLine(surface, ed, documentURI, baseLine, visible.Line, y)
	}
}

// currentSearchMatch returns the index of the match containing pos, or -1.
func currentSearchMatch(matches []editor.Match, pos editor.Pos) int {
	for index, match := range matches {
		if !pos.Before(match.Start) && pos.Before(match.End) {
			return index
		}
	}
	return -1
}

func (s *Session) drawDiagnosticsForLine(surface *render.Surface, ed *editor.Editor, documentURI string, baseLine, line, y int) {
	for _, diagnostic := range s.LSPDiagnostics[documentURI] {
		if diagnostic.Severity != lsp.SeverityError && diagnostic.Severity != lsp.SeverityWarning {
			continue
		}
		start, end, ok := lsp.EditorRange(ed.Buffer, baseLine, diagnostic.Range)
		if !ok || line < start.Line || line > end.Line {
			continue
		}
		if start.Line < line {
			start = editor.Pos{Line: line}
		}
		if end.Line > line {
			end = editor.Pos{Line: line, Rune: len([]rune(ed.Buffer.Lines[line]))}
		}
		startCell := ed.Buffer.CellColumn(start) - ed.Viewport.LeftCell
		endCell := ed.Buffer.CellColumn(end) - ed.Viewport.LeftCell
		if endCell <= startCell {
			endCell = startCell + 1
		}
		color := uint8(8)
		if diagnostic.Severity == lsp.SeverityWarning {
			color = 9
		}
		surface.FillRect(startCell*font.NarrowWidth, y+font.CellHeight-1, (endCell-startCell)*font.NarrowWidth, 1, color)
	}
}

func (s *Session) drawCursor(surface *render.Surface, ed *editor.Editor, topRow, first, last int) {
	if _, _, _, active := s.Vim.Prompt(); active {
		return
	}
	position := ed.Cursor.Pos
	viewport := &ed.Viewport
	if first == last {
		surface.DrawCursor(0, topRow*font.LineAdvance, font.NarrowWidth, render.CursorBlock)
		return
	}
	if position.Line < first || position.Line >= last {
		return
	}
	if position.Line < viewport.TopLine || position.Line >= viewport.TopLine+viewport.Height {
		return
	}
	info := ed.Buffer.Info(position)
	x := (info.Cell - viewport.LeftCell) * font.NarrowWidth
	y := (topRow + position.Line - viewport.TopLine) * font.LineAdvance
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

	buffer := s.CurrentBuffer()
	data := statusData{
		mode:      statusMode(s.Vim.Mode),
		file:      s.statusFile(buffer),
		dirty:     ed.Buffer.Dirty(),
		lspStatus: s.LSPStatus,
	}
	if s.P8 != nil {
		data.inputMode = s.P8.ModeLabel()
	}
	info := ed.Buffer.Info(ed.Cursor.Pos)
	data.line = info.Pos.Line + 1
	data.column = info.Cell + 1
	data.errors, data.warnings = s.DiagnosticCounts(s.activeURI())
	if tokens, ok := s.LSPTokens[s.activeURI()]; ok {
		data.deprecated = tokens.DeprecatedAt(ed.Cursor.Pos.Line, ed.Cursor.Pos.Rune)
	}
	data.sizes, data.sizeColor = s.statusSizes(buffer)
	data.zColor = data.sizeColor
	if message, isError := s.statusMessage(); message != "" {
		data.message = message
		data.messageError = isError
	}
	if s.LSP == nil {
		data.lspStatus = lsp.StatusOff
	}
	drawStatusLine(surface, y, layoutStatus(GridColumns, data))
}

// drawStatusLine draws segments with a two-cell gap and returns the final pixel
// x coordinate. Surface.DrawText returns cells, so status drawing must convert
// between cells and pixels explicitly.
func drawStatusLine(surface *render.Surface, y int, segments []statusSegment) int {
	x := 0
	for index, segment := range segments {
		if index > 0 {
			x += 2 * font.NarrowWidth
		}
		cells := surface.DrawText(x, y, segment.text, segment.color, render.DefaultBackground)
		x += cells * font.NarrowWidth
	}
	return x
}

func statusMode(mode vim.Mode) string {
	switch mode {
	case vim.ModeInsert:
		return "I"
	case vim.ModeVisualChar:
		return "V"
	case vim.ModeVisualLine:
		return "VL"
	case vim.ModeVisualBlock:
		return "VB"
	case vim.ModeCommandLine:
		return ":"
	default:
		return "N"
	}
}

func (s *Session) statusFile(buffer *SessionBuffer) string {
	name := s.bufferName(buffer)
	if buffer != nil && buffer.Cart != nil {
		name = fmt.Sprintf("%s:%d", name, buffer.ActiveTab+1)
	}
	return name
}

func (s *Session) statusSizes(buffer *SessionBuffer) (string, uint8) {
	if buffer == nil {
		return "", 7
	}
	if buffer.Cart == nil && buffer.Owner == "" {
		return "T/C/Z n/a", 6
	}
	info, ok := s.LSPCartInfo[s.activeURI()]
	if !ok || info.Unavailable() {
		return "", 7
	}
	color := uint8(7)
	switch info.CompressedStatus {
	case "warning":
		color = 9
	case "error":
		color = 8
	}
	return fmt.Sprintf("T%d C%d Z%d", info.Tokens, info.Chars, info.Compressed), color
}

func (s *Session) statusMessage() (string, bool) {
	if s.DisplayMessage != "" {
		return s.DisplayMessage, s.DisplayMessageError
	}
	// Sessions created without the app's notification center (unit tests) keep
	// the legacy Status fallback. In the app, statuses are toasts and expire.
	if s.notify == nil && s.Status != "" {
		return s.Status, s.StatusError
	}
	return "", false
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
