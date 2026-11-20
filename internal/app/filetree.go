package app

import (
	"path/filepath"
	"strings"

	"github.com/asimmons91/pico-editor/internal/font"
	"github.com/asimmons91/pico-editor/internal/input"
	"github.com/asimmons91/pico-editor/internal/render"
	"github.com/asimmons91/pico-editor/internal/workspace"
)

const (
	treeColumns = 20
	treeRows    = GridRows - 1 // rows 0..40; status stays visible
)

// TreeState is the file tree overlay's view state.
type TreeState struct {
	Visible      bool
	Focused      bool
	Selected     string
	Collapsed    map[string]bool
	Filter       string
	Filtering    bool
	filterSaved  string
	pendingSlash bool
	Top          int
}

func (t *TreeState) Init(session *Session) {
	if t.Collapsed == nil {
		t.Collapsed = make(map[string]bool)
	}
	if t.Selected == "" {
		t.Selected = session.defaultTreePath()
	}
}

func (s *Session) defaultTreePath() string {
	if current := s.CurrentBuffer(); current != nil && current.Editor.Buffer.Path != "" {
		return current.Editor.Buffer.Path
	}
	rows := s.treeRows()
	if len(rows) > 0 {
		return rows[0].Entry.Path
	}
	return ""
}

type treeRow struct {
	Entry *workspace.Entry
	Depth int
}

func (s *Session) treeRows() []treeRow {
	if s.Workspace == nil {
		return nil
	}
	filter := strings.ToLower(s.Tree.Filter)
	var rows []treeRow
	var walk func(entries []*workspace.Entry, depth int)
	walk = func(entries []*workspace.Entry, depth int) {
		for _, entry := range entries {
			if !treeEntryMatches(entry, filter) {
				continue
			}
			rows = append(rows, treeRow{Entry: entry, Depth: depth})
			if entry.Dir && (filter != "" || !s.Tree.Collapsed[entry.Path]) {
				walk(entry.Children, depth+1)
			}
		}
	}
	walk(s.Workspace.Entries, 0)
	return rows
}

func treeEntryMatches(entry *workspace.Entry, filter string) bool {
	if filter == "" {
		return true
	}
	if !entry.Dir {
		return strings.Contains(strings.ToLower(entry.Name), filter)
	}
	for _, child := range entry.Children {
		if treeEntryMatches(child, filter) {
			return true
		}
	}
	return false
}

func (s *Session) treeSelection() (int, []treeRow) {
	rows := s.treeRows()
	for index, row := range rows {
		if row.Entry.Path == s.Tree.Selected {
			return index, rows
		}
	}
	if len(rows) > 0 {
		s.Tree.Selected = rows[0].Entry.Path
		return 0, rows
	}
	return -1, rows
}

func (s *Session) selectTreeIndex(index int, rows []treeRow) {
	if index < 0 || index >= len(rows) {
		return
	}
	s.Tree.Selected = rows[index].Entry.Path
	height := treeRows - 1
	if index < s.Tree.Top {
		s.Tree.Top = index
	}
	if index >= s.Tree.Top+height {
		s.Tree.Top = index - height + 1
	}
	s.Tree.Top = max(0, s.Tree.Top)
}

// ToggleTree implements Ctrl+Shift+B. A visible but unfocused tree is focused;
// a focused tree closes; a hidden tree opens and focuses.
func (s *Session) ToggleTree() bool {
	if s.Tree.Visible && s.Tree.Focused {
		s.Tree.Visible = false
		s.Tree.Focused = false
		return true
	}
	s.Tree.Visible = true
	s.Tree.Focused = true
	if s.Tree.Selected == "" {
		s.Tree.Selected = s.defaultTreePath()
	}
	s.treeSelection()
	return true
}

// FocusTree shows and focuses the tree.
func (s *Session) FocusTree() bool {
	if !s.Tree.Visible {
		s.Tree.Visible = true
	}
	s.Tree.Focused = true
	s.treeSelection()
	return true
}

func (s *Session) handleTree(ev input.Event) bool {
	if s.Tree.Filtering {
		return s.handleTreeFilter(ev)
	}
	if ev.Kind == input.KindText {
		return true
	}
	if ev.Kind != input.KindKeyDown {
		return false
	}
	index, rows := s.treeSelection()
	switch ev.Key {
	case input.KeyEsc:
		s.Tree.Focused = false
	case input.KeyQ:
		if ev.Mods == 0 {
			s.Tree.Visible = false
			s.Tree.Focused = false
		}
	case input.KeyJ, input.KeyDown:
		s.selectTreeIndex(min(index+1, len(rows)-1), rows)
	case input.KeyK, input.KeyUp:
		s.selectTreeIndex(max(index-1, 0), rows)
	case input.KeyH, input.KeyLeft:
		if index >= 0 && index < len(rows) {
			entry := rows[index].Entry
			if entry.Dir && !s.Tree.Collapsed[entry.Path] {
				s.Tree.Collapsed[entry.Path] = true
			} else {
				s.selectParent(rows, index)
			}
		}
	case input.KeyL, input.KeyRight:
		if index >= 0 && index < len(rows) && rows[index].Entry.Dir {
			delete(s.Tree.Collapsed, rows[index].Entry.Path)
		}
	case input.KeyEnter:
		s.activateTreeRow(rows, index)
	case input.KeyR:
		if ev.Mods == 0 {
			s.refreshWorkspace()
		}
	case input.KeySlash:
		s.Tree.Filtering = true
		s.Tree.filterSaved = s.Tree.Filter
		s.Tree.pendingSlash = true
	}
	return true
}

func (s *Session) handleTreeFilter(ev input.Event) bool {
	switch ev.Kind {
	case input.KindText:
		if ev.Mods&(input.ModCtrl|input.ModAlt|input.ModMeta) != 0 {
			return true
		}
		if s.Tree.pendingSlash {
			s.Tree.pendingSlash = false
			if string(ev.Runes) == "/" {
				return true
			}
		}
		s.Tree.Filter += string(ev.Runes)
		s.Tree.Top = 0
		s.Tree.Selected = ""
		s.treeSelection()
		return true
	case input.KindKeyDown:
	default:
		return false
	}
	switch ev.Key {
	case input.KeyEnter:
		s.Tree.Filtering = false
		s.Tree.filterSaved = ""
		s.Tree.pendingSlash = false
	case input.KeyEsc:
		s.Tree.Filtering = false
		s.Tree.Filter = s.Tree.filterSaved
		s.Tree.filterSaved = ""
		s.Tree.pendingSlash = false
		s.Tree.Selected = ""
		s.treeSelection()
	case input.KeyBackspace:
		runes := []rune(s.Tree.Filter)
		if len(runes) > 0 {
			s.Tree.Filter = string(runes[:len(runes)-1])
			s.Tree.Selected = ""
			s.treeSelection()
		}
	}
	return true
}

func (s *Session) selectParent(rows []treeRow, index int) {
	depth := rows[index].Depth
	for candidate := index - 1; candidate >= 0; candidate-- {
		if rows[candidate].Depth < depth {
			s.selectTreeIndex(candidate, rows)
			return
		}
	}
}

func clampTreeTop(top, selection, total, heightRows int) int {
	if heightRows < 1 {
		return 0
	}
	if selection >= 0 && selection < top {
		top = selection
	}
	if selection >= 0 && selection >= top+heightRows {
		top = selection - heightRows + 1
	}
	maxTop := max(0, total-heightRows)
	return min(max(top, 0), maxTop)
}

func (s *Session) activateTreeRow(rows []treeRow, index int) {
	if index < 0 || index >= len(rows) {
		return
	}
	entry := rows[index].Entry
	if entry.Dir {
		if s.Tree.Collapsed[entry.Path] {
			delete(s.Tree.Collapsed, entry.Path)
		} else {
			s.Tree.Collapsed[entry.Path] = true
		}
		return
	}
	s.OpenPath(entry.Path, false)
}

func (s *Session) refreshWorkspace() bool {
	if s.Workspace == nil {
		return false
	}
	if err := s.Workspace.Refresh(); err != nil {
		s.ShowError("Refresh failed", err)
		return true
	}
	if s.Tree.Selected == "" {
		s.Tree.Selected = s.defaultTreePath()
	}
	s.treeSelection()
	if s.Workspace.Truncated {
		s.SetStatus("file tree truncated at 20000 entries", true)
	} else if len(s.Workspace.Warnings) > 0 {
		s.workspaceStatus(s.Workspace.Warnings)
	}
	return true
}

func (s *Session) drawTree(surface *render.Surface) {
	width := treeColumns * font.NarrowWidth
	height := treeRows * font.LineAdvance
	surface.FillRect(0, 0, width, height, render.DefaultBackground)
	surface.FillRect(width-font.NarrowWidth, 0, font.NarrowWidth, height, 6)

	header := "FILES"
	if s.Tree.Filtering {
		header = "/" + s.Tree.Filter
	} else if s.Tree.Filter != "" {
		header = "FILES /" + s.Tree.Filter
	}
	surface.DrawText(2, 0, clipCells(header, treeColumns-2), 10, render.DefaultBackground)

	rows := s.treeRows()
	selection := -1
	for index, row := range rows {
		if row.Entry.Path == s.Tree.Selected {
			selection = index
			break
		}
	}
	if selection < 0 && len(rows) > 0 {
		selection = 0
	}
	heightRows := treeRows - 1
	top := clampTreeTop(s.Tree.Top, selection, len(rows), heightRows)
	for row := 0; row < heightRows; row++ {
		index := top + row
		if index >= len(rows) {
			break
		}
		entry := rows[index]
		y := (row + 1) * font.LineAdvance
		background := uint8(render.DefaultBackground)
		foreground := uint8(7)
		if index == selection && s.Tree.Focused {
			background = selectionBackground
			foreground = uint8(render.DefaultBackground)
		} else if index == selection {
			foreground = uint8(10)
		}
		prefix := "  "
		if entry.Entry.Dir {
			if s.Tree.Collapsed[entry.Entry.Path] {
				prefix = " +"
			} else {
				prefix = " -"
			}
		}
		name := entry.Entry.Name
		if entry.Entry.Dir {
			name += "/"
		}
		if open := s.bufferForPath(entry.Entry.Path); open != nil {
			var marker string
			switch {
			case open.Editor.Buffer.Dirty():
				marker = "*"
			case open == s.CurrentBuffer():
				marker = ">"
			default:
				marker = "o"
			}
			prefix = marker + prefix[1:]
		}
		text := prefix + strings.Repeat(" ", entry.Depth) + name
		surface.FillRect(0, y, width-font.NarrowWidth, font.CellHeight, background)
		surface.DrawText(2, y, clipCells(text, treeColumns-3), foreground, background)
	}
}

func (s *Session) bufferForPath(path string) *SessionBuffer {
	for _, buffer := range s.Buffers {
		bufferPath, err := filepath.Abs(buffer.Editor.Buffer.Path)
		if err == nil && filepath.Clean(bufferPath) == filepath.Clean(path) {
			return buffer
		}
	}
	return nil
}

func (s *Session) bufferIndex(buffer *SessionBuffer) int {
	for index, candidate := range s.Buffers {
		if candidate == buffer {
			return index
		}
	}
	return -1
}

// ClickTree selects the row under a canvas-space click. It reports whether the
// click was inside the tree.
func (s *Session) ClickTree(x, y int) bool {
	if !s.Tree.Visible || x < 0 || y < 0 || x >= treeColumns*font.NarrowWidth || y >= treeRows*font.LineAdvance {
		return false
	}
	row := y/font.LineAdvance - 1
	if row < 0 {
		return true
	}
	rows := s.treeRows()
	selection := -1
	for index, row := range rows {
		if row.Entry.Path == s.Tree.Selected {
			selection = index
			break
		}
	}
	top := clampTreeTop(s.Tree.Top, selection, len(rows), treeRows-1)
	index := top + row
	if index >= 0 && index < len(rows) {
		s.Tree.Selected = rows[index].Entry.Path
	}
	s.Tree.Focused = true
	return true
}
