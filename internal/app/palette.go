package app

import (
	"strings"

	"github.com/asimmons91/pico-editor/internal/font"
	"github.com/asimmons91/pico-editor/internal/input"
	"github.com/asimmons91/pico-editor/internal/render"
)

const (
	paletteCells = 48
	paletteRows  = 12
	paletteList  = 8
)

// Palette is the command-palette state.
type Palette struct {
	Query    string
	Selected int
	Top      int
	Matches  []int
}

func (g *Game) openPalette() {
	if g.palette == nil {
		g.palette = &Palette{}
	}
	g.refilterPalette()
}

func (g *Game) refilterPalette() {
	palette := g.palette
	if palette == nil {
		return
	}
	query := strings.ToLower(strings.TrimSpace(palette.Query))
	palette.Matches = palette.Matches[:0]
	for index, command := range g.commands {
		enabled := g.commandEnabled(command)
		idMatch := query != "" && strings.Contains(strings.ToLower(command.ID), query)
		if !enabled && !idMatch {
			continue
		}
		if query != "" && !fuzzyMatch(query, commandSearchText(command)) {
			continue
		}
		palette.Matches = append(palette.Matches, index)
	}
	palette.Selected = 0
	palette.Top = 0
}

func (g *Game) selectedCommand() (Command, bool) {
	palette := g.palette
	if palette == nil || palette.Selected < 0 || palette.Selected >= len(palette.Matches) {
		return Command{}, false
	}
	index := palette.Matches[palette.Selected]
	if index < 0 || index >= len(g.commands) {
		return Command{}, false
	}
	return g.commands[index], true
}

func (p *Palette) move(delta int) {
	if p == nil || len(p.Matches) == 0 || delta == 0 {
		return
	}
	p.Selected = min(max(0, p.Selected+delta), len(p.Matches)-1)
	p.ensureVisible()
}

func (p *Palette) page(delta int) {
	p.move(delta * paletteList)
}

func (p *Palette) home() {
	if p == nil || len(p.Matches) == 0 {
		return
	}
	p.Selected = 0
	p.Top = 0
}

func (p *Palette) end() {
	if p == nil || len(p.Matches) == 0 {
		return
	}
	p.Selected = len(p.Matches) - 1
	p.ensureVisible()
}

func (p *Palette) ensureVisible() {
	if p.Selected < p.Top {
		p.Top = p.Selected
	}
	if p.Selected >= p.Top+paletteList {
		p.Top = p.Selected - paletteList + 1
	}
	p.Top = max(0, p.Top)
}

func (g *Game) handlePalette(ev input.Event) (quit, changed bool) {
	palette := g.palette
	if palette == nil {
		return false, false
	}
	switch ev.Kind {
	case input.KindText:
		palette.Query += string(ev.Runes)
		g.refilterPalette()
		return false, true
	case input.KindKeyDown:
	default:
		return false, false
	}
	switch ev.Key {
	case input.KeyEsc:
		g.palette = nil
	case input.KeyEnter:
		command, ok := g.selectedCommand()
		if !ok {
			return false, true
		}
		g.palette = nil
		return command.Run(g)
	case input.KeyUp:
		palette.move(-1)
	case input.KeyDown:
		palette.move(1)
	case input.KeyPageUp:
		palette.page(-1)
	case input.KeyPageDown:
		palette.page(1)
	case input.KeyHome:
		palette.home()
	case input.KeyEnd:
		palette.end()
	case input.KeyBackspace:
		if palette.Query != "" {
			runes := []rune(palette.Query)
			palette.Query = string(runes[:len(runes)-1])
			g.refilterPalette()
		}
	case input.KeyU:
		if ev.Mods&input.ModCtrl != 0 {
			palette.Query = ""
			g.refilterPalette()
		}
	}
	return false, true
}

func (g *Game) drawPalette(surface *render.Surface) {
	palette := g.palette
	if palette == nil {
		return
	}
	width := paletteCells * font.NarrowWidth
	height := paletteRows * font.LineAdvance
	x := (surface.Width - width) / 2
	y := (surface.Height - height) / 2
	drawPanel(surface, x, y, width, height)
	surface.DrawText(x+font.NarrowWidth, y, " COMMANDS ", 10, render.DefaultBackground)

	query := palette.Query
	if query == "" {
		query = "type to filter by title, category, or id"
	}
	surface.DrawText(x+font.NarrowWidth, y+font.LineAdvance, "> "+query, 6, render.DefaultBackground)

	listY := y + 2*font.LineAdvance
	if len(palette.Matches) == 0 {
		surface.DrawText(x+font.NarrowWidth, listY, "no matching commands", 8, render.DefaultBackground)
	}
	for row := 0; row < paletteList; row++ {
		index := palette.Top + row
		if index >= len(palette.Matches) {
			break
		}
		command := g.commands[palette.Matches[index]]
		rowY := listY + row*font.LineAdvance
		bg := uint8(render.DefaultBackground)
		fg := uint8(7)
		if index == palette.Selected {
			bg = selectionBackground
			fg = uint8(render.DefaultBackground)
			surface.FillRect(x+1, rowY, width-2, font.CellHeight, bg)
		}
		title := command.Title
		if command.Category != "" {
			title = command.Category + ": " + strings.TrimPrefix(command.Title, command.Category+": ")
		}
		keyCells := font.CellWidth(command.Key)
		titleCells := paletteCells - 3 - keyCells
		if titleCells < 10 {
			titleCells = 10
		}
		surface.DrawText(x+font.NarrowWidth, rowY, clipCells(title, titleCells), fg, bg)
		if command.Key != "" {
			keyX := x + width - 2*font.NarrowWidth - keyCells*font.NarrowWidth
			keyColor := uint8(6)
			if index == palette.Selected {
				keyColor = fg
			}
			surface.DrawText(keyX, rowY, command.Key, keyColor, bg)
		}
	}

	footerY := y + (paletteRows-2)*font.LineAdvance
	surface.DrawText(x+font.NarrowWidth, footerY, "Enter run   Esc close   Up/Down page", 6, render.DefaultBackground)
}
