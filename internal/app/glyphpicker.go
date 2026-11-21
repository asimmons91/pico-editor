package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/asimmons91/pico-editor/internal/font"
	"github.com/asimmons91/pico-editor/internal/input"
	"github.com/asimmons91/pico-editor/internal/lsp"
	"github.com/asimmons91/pico-editor/internal/render"
	"github.com/asimmons91/pico-editor/internal/vim"
)

const (
	glyphPickerCells   = 40
	glyphPickerRows    = 16
	glyphPickerList    = 10
	glyphPickerPreview = 4
)

// GlyphPicker is the searchable P8SCII picker state. Entries come from the
// pico-lsp catalog; the editor never owns glyph names or aliases.
type GlyphPicker struct {
	All      []lsp.Glyph
	Entries  []lsp.Glyph
	Query    string
	Selected int
	Top      int
}

// NewGlyphPicker builds a picker over a catalog snapshot.
func NewGlyphPicker(entries []lsp.Glyph) *GlyphPicker {
	picker := &GlyphPicker{All: append([]lsp.Glyph(nil), entries...)}
	picker.Refilter()
	return picker
}

// Refilter recomputes the visible entries and resets selection.
func (p *GlyphPicker) Refilter() {
	if p == nil {
		return
	}
	query := strings.ToLower(strings.TrimSpace(p.Query))
	p.Entries = p.Entries[:0]
	for _, entry := range p.All {
		if glyphMatches(entry, query) {
			p.Entries = append(p.Entries, entry)
		}
	}
	p.Selected = 0
	p.Top = 0
}

// Move changes the selected row.
func (p *GlyphPicker) Move(delta int) {
	if p == nil || len(p.Entries) == 0 || delta == 0 {
		return
	}
	p.Selected = min(max(0, p.Selected+delta), len(p.Entries)-1)
	p.ensureVisible()
}

// Page moves by one visible page.
func (p *GlyphPicker) Page(delta int) {
	p.Move(delta * glyphPickerList)
}

// Home selects the first entry.
func (p *GlyphPicker) Home() {
	if p == nil || len(p.Entries) == 0 {
		return
	}
	p.Selected = 0
	p.Top = 0
}

// End selects the last entry.
func (p *GlyphPicker) End() {
	if p == nil || len(p.Entries) == 0 {
		return
	}
	p.Selected = len(p.Entries) - 1
	p.ensureVisible()
}

// SelectedGlyph returns the highlighted catalog entry.
func (p *GlyphPicker) SelectedGlyph() (lsp.Glyph, bool) {
	if p == nil || p.Selected < 0 || p.Selected >= len(p.Entries) {
		return lsp.Glyph{}, false
	}
	return p.Entries[p.Selected], true
}

func (p *GlyphPicker) ensureVisible() {
	if p.Selected < p.Top {
		p.Top = p.Selected
	}
	if p.Selected >= p.Top+glyphPickerList {
		p.Top = p.Selected - glyphPickerList + 1
	}
	p.Top = max(0, p.Top)
}

func glyphMatches(entry lsp.Glyph, query string) bool {
	if query == "" {
		return true
	}
	fields := []string{
		entry.Name,
		entry.Text,
		entry.Category,
		entry.Description,
		fmt.Sprintf("0x%02x", entry.Byte),
		fmt.Sprintf("%d", entry.Byte),
	}
	fields = append(fields, entry.Aliases...)
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), query) {
			return true
		}
	}
	return false
}

// OpenGlyphPicker opens the picker or requests a catalog if LSP is still
// starting.
func (s *Session) OpenGlyphPicker() bool {
	s.clearP8Pending()
	s.glyphPickerPending = false
	if ed := s.CurrentEditor(); ed != nil && s.Vim != nil {
		s.Vim.ResetOperator()
	}
	if s.Vim != nil && s.Vim.Mode == vim.ModeCommandLine {
		s.SetStatus("glyph picker is not available in command-line mode", true)
		return true
	}
	if s.LSP == nil || s.LSPStatus != lsp.StatusReady {
		s.SetStatus("glyph picker requires pico-lsp", true)
		return true
	}
	if len(s.LSPGlyphs) == 0 {
		s.glyphPickerPending = true
		s.LSP.RequestGlyphs()
		s.SetStatus("loading glyph catalog", false)
		return true
	}
	s.clearLSPTransients()
	s.Help = nil
	s.DiagList = nil
	s.Tree.Focused = false
	s.GlyphPicker = NewGlyphPicker(s.LSPGlyphs)
	return true
}

// SetInputMode applies :input and the input.puny palette command. P8SCII
// modes are insert-only.
func (s *Session) SetInputMode(topic string) bool {
	if s.P8 == nil || s.Vim == nil {
		return false
	}
	switch topic {
	case "off":
		s.P8.Reset()
		return true
	case "puny", "hira", "kata":
	default:
		s.SetStatus("input mode must be puny, hira, kata, or off", true)
		return true
	}
	if s.Vim.Mode == vim.ModeNormal {
		if ed := s.CurrentEditor(); ed != nil {
			s.Vim.EnterInsertAtCursor(ed)
		}
	}
	if s.Vim.Mode != vim.ModeInsert {
		s.SetStatus("input modes require normal or insert mode", true)
		return true
	}
	s.P8.Pending = ""
	switch topic {
	case "puny":
		s.P8.Mode = input.P8ModePuny
	case "hira":
		s.P8.Mode = input.P8ModeHiragana
	case "kata":
		s.P8.Mode = input.P8ModeKatakana
	}
	return true
}

// TogglePuny switches the insert-mode puny input mode.
func (s *Session) TogglePuny() bool {
	if s.P8 == nil || s.Vim == nil {
		return false
	}
	if s.Vim.Mode == vim.ModeNormal {
		if ed := s.CurrentEditor(); ed != nil {
			s.Vim.EnterInsertAtCursor(ed)
		}
	}
	if s.Vim.Mode != vim.ModeInsert {
		s.SetStatus("input modes require normal or insert mode", true)
		return true
	}
	if s.P8.Mode == input.P8ModePuny {
		s.P8.Mode = input.P8ModeDefault
	} else {
		s.P8.Mode = input.P8ModePuny
	}
	s.P8.Pending = ""
	return true
}

func (s *Session) handleGlyphPicker(ev input.Event) bool {
	picker := s.GlyphPicker
	if picker == nil {
		return false
	}
	switch ev.Kind {
	case input.KindText:
		picker.Query += string(ev.Runes)
		picker.Refilter()
		return true
	case input.KindKeyDown:
	default:
		return true
	}
	if ev.Repeat {
		switch ev.Key {
		case input.KeyUp, input.KeyDown, input.KeyPageUp, input.KeyPageDown:
		default:
			return true
		}
	}
	switch ev.Key {
	case input.KeyEsc:
		s.GlyphPicker = nil
	case input.KeyEnter:
		entry, ok := picker.SelectedGlyph()
		if !ok {
			return true
		}
		s.GlyphPicker = nil
		s.insertGlyph(entry.Text)
	case input.KeyUp:
		picker.Move(-1)
	case input.KeyDown:
		picker.Move(1)
	case input.KeyPageUp:
		picker.Page(-1)
	case input.KeyPageDown:
		picker.Page(1)
	case input.KeyHome:
		picker.Home()
	case input.KeyEnd:
		picker.End()
	case input.KeyBackspace:
		if picker.Query != "" {
			runes := []rune(picker.Query)
			picker.Query = string(runes[:len(runes)-1])
			picker.Refilter()
		}
	}
	return true
}

func (s *Session) insertGlyph(text string) bool {
	buffer := s.CurrentBuffer()
	if buffer == nil || text == "" {
		return false
	}
	ed := buffer.Editor
	switch s.Vim.Mode {
	case vim.ModeInsert:
		ed.BeginGroup()
		if _, err := ed.Insert(text); err != nil {
			s.SetStatus(err.Error(), true)
			return false
		}
	case vim.ModeNormal:
		s.Vim.EnterInsertAtCursor(ed)
		if _, err := ed.Insert(text); err != nil {
			s.SetStatus(err.Error(), true)
			return false
		}
	case vim.ModeVisualChar, vim.ModeVisualLine, vim.ModeVisualBlock:
		result := s.Vim.ReplaceSelection(ed, text)
		s.applyVimResult(result)
	default:
		s.SetStatus("glyph insertion is not available in this mode", true)
		return false
	}
	s.syncCurrentCart()
	s.syncLSPCurrent()
	s.tokenDue = time.Now().Add(120 * time.Millisecond)
	s.cartInfoDue = time.Now().Add(300 * time.Millisecond)
	return true
}

func (s *Session) drawGlyphPicker(surface *render.Surface) {
	picker := s.GlyphPicker
	width := glyphPickerCells * font.NarrowWidth
	height := glyphPickerRows * font.LineAdvance
	x := (surface.Width - width) / 2
	y := (surface.Height - height) / 2
	drawPanel(surface, x, y, width, height)
	surface.DrawText(x+font.NarrowWidth, y, " P8SCII GLYPH PICKER ", panelTitle, render.DefaultBackground)

	query := picker.Query
	if query == "" {
		query = "filter: name alias 0x10 category"
	}
	surface.DrawText(x+font.NarrowWidth, y+font.LineAdvance, clipCells("> "+query, glyphPickerCells-1), 6, render.DefaultBackground)

	listY := y + 2*font.LineAdvance
	if len(picker.Entries) == 0 {
		surface.DrawText(x+font.NarrowWidth, listY, "no matching glyphs", 8, render.DefaultBackground)
	}
	for row := 0; row < glyphPickerList; row++ {
		index := picker.Top + row
		if index >= len(picker.Entries) {
			break
		}
		entry := picker.Entries[index]
		rowY := listY + row*font.LineAdvance
		bg := uint8(render.DefaultBackground)
		fg := uint8(7)
		if index == picker.Selected {
			bg = selectionBackground
			fg = selectionForeground
			surface.FillRect(x+1, rowY, width-2, font.CellHeight, bg)
		}
		surface.DrawText(x+font.NarrowWidth, rowY, clipCells(entry.Text, glyphPickerPreview), fg, bg)
		name := entry.Name
		if entry.Custom {
			name = "*" + name
		}
		surface.DrawText(x+6*font.NarrowWidth, rowY, clipCells(name, 18), fg, bg)
		surface.DrawText(x+25*font.NarrowWidth, rowY, fmt.Sprintf("0x%02x", entry.Byte), fg, bg)
		surface.DrawText(x+31*font.NarrowWidth, rowY, clipCells(entry.Category, 7), fg, bg)
	}

	previewY := y + (2+glyphPickerList)*font.LineAdvance
	surface.FillRect(x+1, previewY-1, width-2, 1, panelBorder)
	if entry, ok := picker.SelectedGlyph(); ok {
		if glyph, known := font.Lookup(entry.Text); known {
			surface.DrawGlyphScaled(x+font.NarrowWidth, previewY+1, glyph, 2, 7, render.DefaultBackground)
		}
		detailX := x + 6*font.NarrowWidth
		surface.DrawText(detailX, previewY, clipCells(fmt.Sprintf("%s  byte 0x%02x  %s", entry.Name, entry.Byte, entry.Category), glyphPickerCells-7), 7, render.DefaultBackground)
		details := entry.Description
		if len(entry.Aliases) > 0 {
			details += "  aliases: " + strings.Join(entry.Aliases, ", ")
		}
		surface.DrawText(detailX, previewY+font.LineAdvance, clipCells(details, glyphPickerCells-7), 6, render.DefaultBackground)
	}
	footerY := y + (glyphPickerRows-2)*font.LineAdvance
	surface.DrawText(x+font.NarrowWidth, footerY, "Enter insert   Esc close   Up/Down page", 6, render.DefaultBackground)
}
