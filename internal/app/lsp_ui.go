package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/asimmons91/pico-editor/internal/editor"
	"github.com/asimmons91/pico-editor/internal/font"
	"github.com/asimmons91/pico-editor/internal/lsp"
	"github.com/asimmons91/pico-editor/internal/render"
)

// CompletionPopup is the transient completion menu.
type CompletionPopup struct {
	Items    []lsp.CompletionItem
	Selected int
	Manual   bool
	Trigger  string
	Prefix   string
	Anchor   lsp.Position
}

// HoverPopup is plain-text hover content.
type HoverPopup struct {
	Text string
}

// SignaturePopup is the active signature-help state.
type SignaturePopup struct {
	Help lsp.SignatureHelp
}

// DiagnosticItem is one row in the :diag list.
type DiagnosticItem struct {
	URI      string
	Range    lsp.Range
	Severity lsp.Severity
	Message  string
}

// DiagnosticList is the :diag overlay.
type DiagnosticList struct {
	Items    []DiagnosticItem
	Selected int
}

func (s *Session) moveCompletion(delta int) {
	if s.Completion == nil || len(s.Completion.Items) == 0 || delta == 0 {
		return
	}
	next := s.Completion.Selected + delta
	if next < 0 {
		next = 0
	}
	if next >= len(s.Completion.Items) {
		next = len(s.Completion.Items) - 1
	}
	s.Completion.Selected = next
}

func (s *Session) acceptCompletion() {
	popup := s.Completion
	s.Completion = nil
	if popup == nil || len(popup.Items) == 0 {
		return
	}
	buffer := s.CurrentBuffer()
	if buffer == nil {
		return
	}
	item := popup.Items[popup.Selected]
	ed := buffer.Editor
	baseLine := s.baseLine(buffer)

	var edits []completionEdit
	mainText := item.InsertText
	if mainText == "" {
		mainText = item.Label
	}
	mainEdit := editor.Edit{Start: ed.Cursor.Pos, End: ed.Cursor.Pos, Text: editor.NormalizeNewlines(mainText)}
	if item.TextEdit != nil {
		start, end, ok := lsp.EditorRange(ed.Buffer, baseLine, item.TextEdit.Range)
		if !ok {
			s.SetStatus("completion edit is outside the code view", true)
			return
		}
		mainEdit = editor.Edit{Start: start, End: end, Text: editor.NormalizeNewlines(item.TextEdit.NewText)}
	}
	edits = append(edits, completionEdit{edit: mainEdit, main: true})
	for _, additional := range item.Additional {
		start, end, ok := lsp.EditorRange(ed.Buffer, baseLine, additional.Range)
		if !ok {
			s.SetStatus("completion edit is outside the code view", true)
			return
		}
		edits = append(edits, completionEdit{edit: editor.Edit{Start: start, End: end, Text: editor.NormalizeNewlines(additional.NewText)}})
	}
	if !s.completionEditsAllowed(ed, completionEdits(edits)) {
		s.SetStatus("completion edit is outside the active tab", true)
		return
	}
	if s.completionEditsOverlap(completionEdits(edits)) {
		// The spec requires the main item to survive an overlapping additional
		// edit. Drop the additional edits instead of corrupting the buffer.
		edits = edits[:1]
		s.SetStatus("completion additional edits overlapped; inserted the main item", true)
	}
	// Apply additional edits in descending position order so earlier offsets
	// stay valid, then apply the main edit.
	for i := 1; i < len(edits); i++ {
		for j := i + 1; j < len(edits); j++ {
			if edits[i].edit.Start.Before(edits[j].edit.Start) {
				edits[i], edits[j] = edits[j], edits[i]
			}
		}
	}
	main := edits[0]
	shiftedStart := main.edit.Start
	shiftedEnd := main.edit.End
	for _, additional := range edits[1:] {
		ed.BeginGroup()
		if _, err := ed.Apply(additional.edit); err != nil {
			s.SetStatus("completion edit failed", true)
			return
		}
		if !additional.edit.End.Before(main.edit.Start) {
			continue
		}
		shiftedStart = shiftAfterEdit(shiftedStart, additional.edit)
		shiftedEnd = shiftAfterEdit(shiftedEnd, additional.edit)
	}
	ed.BeginGroup()
	main.edit.Start = shiftedStart
	main.edit.End = shiftedEnd
	if _, err := ed.Apply(main.edit); err != nil {
		s.SetStatus("completion edit failed", true)
		return
	}
	s.syncLSPCurrent()
	s.tokenDue = time.Now().Add(120 * time.Millisecond)
	s.cartInfoDue = time.Now().Add(300 * time.Millisecond)
}

type completionEdit struct {
	edit editor.Edit
	main bool
}

func completionEdits(items []completionEdit) []editor.Edit {
	out := make([]editor.Edit, 0, len(items))
	for _, item := range items {
		out = append(out, item.edit)
	}
	return out
}

func (s *Session) completionEditsAllowed(ed *editor.Editor, edits []editor.Edit) bool {
	if ed.WindowEmpty() {
		return true
	}
	first, last := ed.WindowFirst(), ed.WindowLast()
	for _, edit := range edits {
		if edit.Start.Line < first || edit.End.Line > last || edit.Start.Line > edit.End.Line {
			return false
		}
	}
	return true
}

func (s *Session) completionEditsOverlap(edits []editor.Edit) bool {
	for i := range edits {
		for j := i + 1; j < len(edits); j++ {
			a, b := edits[i], edits[j]
			if !a.End.Before(b.Start) && !b.End.Before(a.Start) {
				return true
			}
		}
	}
	return false
}

// shiftAfterEdit moves pos after an edit that ends at or before pos. Additional
// completion edits are usually absent; this keeps multi-edit items correct when
// the server sends them.
func shiftAfterEdit(pos editor.Pos, edit editor.Edit) editor.Pos {
	if pos.Before(edit.Start) {
		return pos
	}
	if pos.Line < edit.End.Line || (pos.Line == edit.End.Line && pos.Rune < edit.End.Rune) {
		return edit.Start
	}
	newEnd := posAfterEditText(edit.Start, edit.Text)
	if edit.Start.Line == edit.End.Line {
		if pos.Line == edit.End.Line {
			pos.Rune = newEnd.Rune + (pos.Rune - edit.End.Rune)
		}
		return pos
	}
	if pos.Line == edit.End.Line {
		return editor.Pos{Line: newEnd.Line, Rune: newEnd.Rune + (pos.Rune - edit.End.Rune)}
	}
	pos.Line += newEnd.Line - edit.End.Line
	return pos
}

func posAfterEditText(start editor.Pos, text string) editor.Pos {
	lines := strings.Split(text, "\n")
	if len(lines) == 1 {
		return editor.Pos{Line: start.Line, Rune: start.Rune + len([]rune(text))}
	}
	return editor.Pos{Line: start.Line + len(lines) - 1, Rune: len([]rune(lines[len(lines)-1]))}
}

type statusSegment struct {
	text  string
	color uint8
}

func (s *Session) lspStatusSegments() []statusSegment {
	if s.LSP == nil {
		return nil
	}
	var diagnostics []statusSegment
	errors, warnings := s.DiagnosticCounts(s.activeURI())
	if errors > 0 {
		diagnostics = append(diagnostics, statusSegment{fmt.Sprintf("E%d", errors), 8})
	}
	if warnings > 0 {
		diagnostics = append(diagnostics, statusSegment{fmt.Sprintf("W%d", warnings), 9})
	}
	var sizes []statusSegment
	buffer := s.CurrentBuffer()
	if buffer != nil {
		if buffer.Cart == nil {
			sizes = append(sizes, statusSegment{"T/C/Z n/a", 6})
		} else if info, ok := s.LSPCartInfo[s.activeURI()]; ok && !info.Unavailable() {
			color := uint8(7)
			switch info.CompressedStatus {
			case "warning":
				color = 9
			case "error":
				color = 8
			}
			sizes = append(sizes,
				statusSegment{fmt.Sprintf("T%d/%d", info.Tokens, info.TokenLimit), 7},
				statusSegment{fmt.Sprintf("C%d/%d", info.Chars, info.CharLimit), 7},
				statusSegment{fmt.Sprintf("Z%d", info.Compressed), color},
			)
		}
	}
	var server []statusSegment
	switch s.LSPStatus {
	case lsp.StatusReady:
		server = append(server, statusSegment{"LSP", 11})
	case lsp.StatusStarting:
		server = append(server, statusSegment{"LSP...", 10})
	case lsp.StatusFailed:
		server = append(server, statusSegment{"LSP!", 8})
	}
	return fitStatusSegments(diagnostics, sizes, server)
}

func fitStatusSegments(diagnostics, sizes, server []statusSegment) []statusSegment {
	segments := append(append(append([]statusSegment(nil), diagnostics...), sizes...), server...)
	for segmentCells(segments) > 44 && len(sizes) > 0 {
		sizes = sizes[:0]
		segments = append(append(append([]statusSegment(nil), diagnostics...), sizes...), server...)
	}
	for segmentCells(segments) > 44 && len(diagnostics) > 0 {
		diagnostics = nil
		segments = append(append(append([]statusSegment(nil), diagnostics...), sizes...), server...)
	}
	return segments
}

func segmentCells(segments []statusSegment) int {
	cells := 0
	for _, segment := range segments {
		cells += font.CellWidth(segment.text) + 1
	}
	return cells
}

func drawStatusSegments(surface *render.Surface, x, y int, segments []statusSegment) int {
	for _, segment := range segments {
		x += surface.DrawText(x, y, segment.text, segment.color, render.DefaultBackground)
		x += font.NarrowWidth
	}
	return x
}

func (s *Session) drawCompletion(surface *render.Surface, ed *editor.Editor, top int) {
	popup := s.Completion
	if popup == nil || len(popup.Items) == 0 {
		return
	}
	rows := min(10, len(popup.Items))
	widthCells := 36
	for _, item := range popup.Items {
		line := item.Label
		if item.Detail != "" {
			line += "  " + item.Detail
		}
		widthCells = max(widthCells, min(56, font.CellWidth(line)+2))
	}
	width := widthCells * font.NarrowWidth
	height := (rows + 2) * font.LineAdvance
	cursorLine := top + ed.Cursor.Pos.Line - ed.Viewport.TopLine
	cursorCell := ed.Buffer.Info(ed.Cursor.Pos).Cell - ed.Viewport.LeftCell
	x := cursorCell * font.NarrowWidth
	x = min(max(0, x), surface.Width-width)
	y := (cursorLine + 1) * font.LineAdvance
	if y+height > statusRow*font.LineAdvance {
		y = cursorLine*font.LineAdvance - height
	}
	y = max(0, y)
	drawPanel(surface, x, y, width, height)
	for index := 0; index < rows; index++ {
		item := popup.Items[index]
		rowY := y + (index+1)*font.LineAdvance
		bg := uint8(render.DefaultBackground)
		fg := uint8(7)
		if index == popup.Selected {
			bg = selectionBackground
			fg = uint8(render.DefaultBackground)
			surface.FillRect(x+1, rowY, width-2, font.CellHeight, bg)
		}
		line := item.Label
		if item.Detail != "" {
			line += "  " + item.Detail
		}
		surface.DrawText(x+font.NarrowWidth, rowY, clipCells(line, widthCells-2), fg, bg)
	}
	selected := popup.Items[popup.Selected]
	doc := strings.TrimSpace(selected.Documentation)
	if doc == "" {
		return
	}
	docRows := min(3, strings.Count(doc, "\n")+1)
	docY := y + height
	if docY+(docRows+2)*font.LineAdvance > statusRow*font.LineAdvance {
		docY = y - (docRows+2)*font.LineAdvance
	}
	if docY < 0 {
		return
	}
	drawPanel(surface, x, docY, width, (docRows+2)*font.LineAdvance)
	lines := strings.Split(doc, "\n")
	for index := 0; index < docRows && index < len(lines); index++ {
		surface.DrawText(x+font.NarrowWidth, docY+(index+1)*font.LineAdvance, clipCells(lines[index], widthCells-2), 6, render.DefaultBackground)
	}
}

func drawPanel(surface *render.Surface, x, y, width, height int) {
	surface.FillRect(x, y, width, height, render.DefaultBackground)
	surface.FillRect(x, y, width, 1, 6)
	surface.FillRect(x, y+height-1, width, 1, 6)
	surface.FillRect(x, y, 1, height, 6)
	surface.FillRect(x+width-1, y, 1, height, 6)
}

func (s *Session) drawHover(surface *render.Surface) {
	if s.Hover == nil {
		return
	}
	lines := wrapCells(s.Hover.Text, 40, 12)
	if len(lines) == 0 {
		return
	}
	width := 42 * font.NarrowWidth
	height := (len(lines) + 2) * font.LineAdvance
	x := (surface.Width - width) / 2
	y := (surface.Height - height) / 2
	drawPanel(surface, x, y, width, height)
	for index, line := range lines {
		surface.DrawText(x+font.NarrowWidth, y+(index+1)*font.LineAdvance, line, 7, render.DefaultBackground)
	}
}

func wrapCells(text string, width, maxLines int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	current := ""
	for _, word := range words {
		candidate := word
		if current != "" {
			candidate = current + " " + word
		}
		if font.CellWidth(candidate) > width && current != "" {
			lines = append(lines, current)
			current = word
			if len(lines) == maxLines {
				return lines
			}
			continue
		}
		current = candidate
	}
	if current != "" && len(lines) < maxLines {
		lines = append(lines, current)
	}
	return lines
}

func (s *Session) drawSignature(surface *render.Surface) {
	if s.Signature == nil || len(s.Signature.Help.Signatures) == 0 {
		return
	}
	count := min(3, len(s.Signature.Help.Signatures))
	width := 60 * font.NarrowWidth
	height := (count + 2) * font.LineAdvance
	x := (surface.Width - width) / 2
	y := statusRow*font.LineAdvance - height
	drawPanel(surface, x, y, width, height)
	active := s.Signature.Help.ActiveSignature
	for index := 0; index < count; index++ {
		signature := s.Signature.Help.Signatures[index]
		rowY := y + (index+1)*font.LineAdvance
		color := uint8(6)
		if index == active {
			color = 7
		}
		surface.DrawText(x+font.NarrowWidth, rowY, clipCells(signature.Label, 56), color, render.DefaultBackground)
		if index != active {
			continue
		}
		parameter := signature.ActiveParam
		if !signature.ActiveParamSet {
			parameter = s.Signature.Help.ActiveParameter
		}
		if parameter < 0 || parameter >= len(signature.Parameters) {
			continue
		}
		label := signature.Parameters[parameter].Label
		offset := strings.Index(signature.Label, label)
		if offset < 0 || label == "" {
			continue
		}
		prefix := signature.Label[:offset]
		cells := font.CellWidth(prefix)
		widthCells := font.CellWidth(label)
		if cells+widthCells > 56 {
			widthCells = max(1, 56-cells)
		}
		surface.InvertRect(x+font.NarrowWidth+cells*font.NarrowWidth, rowY, widthCells*font.NarrowWidth, font.CellHeight)
	}
}

func (s *Session) drawDiagnosticList(surface *render.Surface) {
	list := s.DiagList
	if list == nil || len(list.Items) == 0 {
		return
	}
	const widthCells = 60
	rows := min(30, len(list.Items))
	width := widthCells * font.NarrowWidth
	height := (rows + 2) * font.LineAdvance
	x := (surface.Width - width) / 2
	y := (surface.Height - height) / 2
	drawPanel(surface, x, y, width, height)
	surface.DrawText(x+font.NarrowWidth, y, " DIAGNOSTICS ", 10, render.DefaultBackground)
	top := 0
	if list.Selected >= rows {
		top = list.Selected - rows + 1
	}
	for row := 0; row < rows; row++ {
		index := top + row
		if index >= len(list.Items) {
			break
		}
		item := list.Items[index]
		rowY := y + (row+1)*font.LineAdvance
		bg := uint8(render.DefaultBackground)
		fg := uint8(7)
		if index == list.Selected {
			bg = selectionBackground
			fg = uint8(render.DefaultBackground)
			surface.FillRect(x+1, rowY, width-2, font.CellHeight, bg)
		}
		severity := "I"
		color := fg
		switch item.Severity {
		case lsp.SeverityError:
			severity = "E"
			if index != list.Selected {
				color = 8
			}
		case lsp.SeverityWarning:
			severity = "W"
			if index != list.Selected {
				color = 9
			}
		}
		line := item.Range.Start.Line + 1
		text := fmt.Sprintf("%s %4d  %s", severity, line, item.Message)
		surface.DrawText(x+font.NarrowWidth, rowY, clipCells(text, widthCells-2), color, bg)
	}
}

func (s *Session) clearLSPTransients() {
	s.Completion = nil
	s.Hover = nil
	s.Signature = nil
	s.DiagList = nil
}
