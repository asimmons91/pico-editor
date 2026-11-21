package app

import "github.com/asimmons91/pico-editor/internal/font"

// Gutter geometry. The gutter holds one diagnostic sign and a three-digit line
// number, matching the compact LazyVim-style editor layout.
const gutterWidth = 4

// editorLeft returns the first cell of the editor area. A visible tree reduces
// the editor instead of covering it.
func (s *Session) editorLeft() int {
	if s.Tree.Visible {
		return treeColumns
	}
	return 0
}

func (s *Session) editorWidth() int {
	return max(1, GridColumns-s.editorLeft())
}

// codeLeft is the first text cell after the line-number gutter.
func (s *Session) codeLeft() int {
	return s.editorLeft() + gutterWidth
}

func (s *Session) codeWidth() int {
	return max(1, s.editorWidth()-gutterWidth)
}

func (s *Session) editorLeftPx() int {
	return s.editorLeft() * font.NarrowWidth
}

func (s *Session) editorRightPx() int {
	return (s.editorLeft() + s.editorWidth()) * font.NarrowWidth
}

func (s *Session) codeLeftPx() int {
	return s.codeLeft() * font.NarrowWidth
}

// popupWidth limits editor floats to the current editor area with a small
// margin on each side.
func (s *Session) popupWidth(maxCells int) int {
	return min(maxCells, max(20, s.editorWidth()-2))
}

// popupLeft centers a float in the editor area, or falls back to the canvas
// when the float is wider than the editor.
func (s *Session) popupLeft(width int) int {
	left := s.editorLeftPx()
	right := s.editorRightPx()
	if width > right-left {
		return max(0, (CanvasWidth-width)/2)
	}
	return left + (right-left-width)/2
}

// clampPopupX keeps an anchored float inside the editor area when possible.
func (s *Session) clampPopupX(x, width int) int {
	left := s.editorLeftPx()
	right := s.editorRightPx()
	if width <= right-left {
		return min(max(left, x), right-width)
	}
	return min(max(0, x), CanvasWidth-width)
}

// resizeEditor applies the current sidebar width to the active viewport.
func (s *Session) resizeEditor() {
	ed := s.CurrentEditor()
	if ed == nil {
		return
	}
	ed.Viewport.Width = s.codeWidth()
	ed.EnsureVisible()
}

// setTreeVisible shows or hides the sidebar and reflows the editor area.
func (s *Session) setTreeVisible(visible bool) {
	if s.Tree.Visible == visible {
		return
	}
	s.Tree.Visible = visible
	if !visible {
		s.Tree.Focused = false
	}
	s.resizeEditor()
}
