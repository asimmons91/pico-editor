package render

import "github.com/asimmons91/pico-editor/internal/font"

// DrawText draws one line of canonical Unicode text and returns the advanced
// cell width. Newlines are the caller's responsibility. Control clusters draw
// nothing and advance zero cells. Unknown clusters draw a 4x6 replacement box,
// advance one cell, and are logged once per rune.
func (s *Surface) DrawText(x, y int, text string, fg, bg uint8) int {
	cells := 0
	for cluster := range font.Clusters(text) {
		if cluster.Known {
			if !cluster.Meta.Printable {
				continue
			}
			s.DrawGlyph(x, y, cluster.Shape, fg, bg)
			x += cluster.Width
			cells += cluster.Width / font.NarrowWidth
			continue
		}
		s.DrawGlyph(x, y, font.Replacement, fg, bg)
		x += font.NarrowWidth
		cells++
		s.logUnknown(cluster.Text)
	}
	return cells
}

func (s *Surface) logUnknown(text string) {
	if s.Logger == nil {
		return
	}
	if s.unknown == nil {
		s.unknown = make(map[rune]struct{})
	}
	for _, r := range text {
		if _, seen := s.unknown[r]; seen {
			continue
		}
		s.unknown[r] = struct{}{}
		s.Logger.Debug("unknown glyph", "rune", string(r))
	}
}
