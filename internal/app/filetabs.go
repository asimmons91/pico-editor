package app

import (
	"fmt"

	"github.com/asimmons91/pico-editor/internal/font"
	"github.com/asimmons91/pico-editor/internal/render"
)

type tabHit struct {
	Index     int
	StartCell int
	EndCell   int
	Label     string
	Width     int
}

func fitTabs(labels []string, active int, maxCells int) []tabHit {
	if len(labels) == 0 || maxCells <= 0 {
		return nil
	}
	widths := make([]int, len(labels))
	total := 0
	for index, label := range labels {
		widths[index] = font.CellWidth(label)
		total += widths[index]
	}
	start := 0
	if total > maxCells && active >= 0 && active < len(labels) {
		used := 0
		start = active
		for index := active; index >= 0; index-- {
			if used+widths[index] > maxCells {
				start = min(index+1, active)
				break
			}
			used += widths[index]
			start = index
		}
	}
	var hits []tabHit
	cell := 0
	for index := start; index < len(labels); index++ {
		if cell+widths[index] > maxCells {
			break
		}
		hits = append(hits, tabHit{
			Index:     index,
			StartCell: cell,
			EndCell:   cell + widths[index],
			Label:     labels[index],
			Width:     widths[index],
		})
		cell += widths[index]
	}
	return hits
}

func (s *Session) fileTabLabels() []string {
	labels := make([]string, 0, len(s.Buffers))
	for _, buffer := range s.Buffers {
		name := clipCells(s.bufferName(buffer), 28)
		if buffer.Editor.Buffer.Dirty() {
			labels = append(labels, " "+name+"* ")
			continue
		}
		labels = append(labels, " "+name+" ")
	}
	return labels
}

func (s *Session) drawFileTabs(surface *render.Surface) {
	y := fileTabRow * font.LineAdvance
	surface.FillRect(0, y, surface.Width, font.CellHeight, render.DefaultBackground)
	hits := fitTabs(s.fileTabLabels(), s.Current, GridColumns)
	for _, hit := range hits {
		background := uint8(render.DefaultBackground)
		foreground := uint8(7)
		if hit.Index == s.Current {
			background = 6
			foreground = uint8(render.DefaultBackground)
		}
		x := hit.StartCell * font.NarrowWidth
		surface.FillRect(x, y, hit.Width*font.NarrowWidth, font.CellHeight, background)
		surface.DrawText(x, y, hit.Label, foreground, background)
	}
}

func (s *Session) cartTabLabels(buffer *SessionBuffer) []string {
	if buffer == nil || buffer.Cart == nil {
		return nil
	}
	labels := make([]string, 0, len(buffer.Cart.Tabs))
	dirty := buffer.Editor.Buffer.Dirty()
	for index := range buffer.Cart.Tabs {
		label := fmt.Sprintf(" %d ", index)
		if index == buffer.ActiveTab && dirty {
			label = fmt.Sprintf(" %d* ", index)
		}
		labels = append(labels, label)
	}
	return labels
}

func (s *Session) drawCartTabs(surface *render.Surface) {
	buffer := s.CurrentBuffer()
	if buffer == nil || buffer.Cart == nil {
		return
	}
	y := cartTabRow * font.LineAdvance
	surface.FillRect(0, y, surface.Width, font.CellHeight, render.DefaultBackground)
	hits := fitTabs(s.cartTabLabels(buffer), buffer.ActiveTab, GridColumns)
	for _, hit := range hits {
		background := uint8(render.DefaultBackground)
		foreground := uint8(7)
		if hit.Index == buffer.ActiveTab {
			background = 6
			foreground = uint8(render.DefaultBackground)
		}
		x := hit.StartCell * font.NarrowWidth
		surface.FillRect(x, y, hit.Width*font.NarrowWidth, font.CellHeight, background)
		surface.DrawText(x, y, hit.Label, foreground, background)
	}
}

// ClickFileTabs switches to the file tab under a canvas-space click.
func (s *Session) ClickFileTabs(x, y int) bool {
	if y < fileTabRow*font.LineAdvance || y >= (fileTabRow+1)*font.LineAdvance {
		return false
	}
	cell := x / font.NarrowWidth
	for _, hit := range fitTabs(s.fileTabLabels(), s.Current, GridColumns) {
		if cell >= hit.StartCell && cell < hit.EndCell {
			if hit.Index != s.Current {
				s.endInsertMode()
				s.Current = hit.Index
				s.prepareBuffer(s.CurrentBuffer())
				s.clearLSPTransients()
				s.requestActiveCartInfo()
			}
			return true
		}
	}
	return true
}

// ClickCartTabs switches to the code tab under a canvas-space click.
func (s *Session) ClickCartTabs(x, y int) bool {
	buffer := s.CurrentBuffer()
	if buffer == nil || buffer.Cart == nil {
		return false
	}
	if y < cartTabRow*font.LineAdvance || y >= (cartTabRow+1)*font.LineAdvance {
		return false
	}
	cell := x / font.NarrowWidth
	for _, hit := range fitTabs(s.cartTabLabels(buffer), buffer.ActiveTab, GridColumns) {
		if cell >= hit.StartCell && cell < hit.EndCell {
			if hit.Index != buffer.ActiveTab {
				s.endInsertMode()
				s.saveActiveView(buffer)
				buffer.ActiveTab = hit.Index
				tab := buffer.Cart.Tabs[hit.Index]
				buffer.Editor.SetLineRange(tab.StartLine, tab.EndLine)
				restoreView(buffer.Editor, buffer.TabViews[hit.Index])
				s.clearLSPTransients()
			}
			return true
		}
	}
	return true
}
