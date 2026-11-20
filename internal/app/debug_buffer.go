package app

import (
	"fmt"
	"path/filepath"

	"github.com/asimmons91/pico-editor/internal/editor"
	"github.com/asimmons91/pico-editor/internal/font"
	"github.com/asimmons91/pico-editor/internal/input"
	"github.com/asimmons91/pico-editor/internal/render"
)

const (
	debugRows       = GridRows - 1
	debugStatusRow  = GridRows - 1
	debugSelection  = 7
	debugStatusFg   = 6
	debugStatusErr  = 8
	debugCursorCell = 1
)

// DebugBuffer is the temporary M2 buffer view. It uses a direct key map rather
// than Vim and is deleted when M3 lands.
type DebugBuffer struct {
	Editor      *editor.Editor
	Status      string
	ErrorStatus bool
}

// NewDebugBuffer opens path and prepares the debug view.
func NewDebugBuffer(path string) (*DebugBuffer, error) {
	buffer, err := editor.Open(path)
	if err != nil {
		return nil, err
	}
	ed := editor.NewEditor(buffer)
	ed.Viewport.Width = GridColumns
	ed.Viewport.Height = debugRows
	ed.Viewport.ScrollOff = 2
	ed.EnsureVisible()
	return &DebugBuffer{Editor: ed}, nil
}

// Draw composes the buffer, selection, cursor, and status row.
func (d *DebugBuffer) Draw(surface *render.Surface, cursorVisible bool) {
	surface.Clear(render.DefaultBackground)
	viewport := &d.Editor.Viewport
	for _, visible := range viewport.VisibleLines(d.Editor.Buffer) {
		line := d.Editor.Buffer.Lines[visible.Line]
		y := visible.Row * font.LineAdvance
		for cluster := range editor.LineClusters(line, d.Editor.Buffer.TabWidth) {
			x := (cluster.Cell - viewport.LeftCell) * font.NarrowWidth
			if x >= surface.Width {
				break
			}
			if x+cluster.Pixels <= 0 {
				continue
			}
			selected := d.clusterSelected(visible.Line, cluster.Rune)
			foreground := uint8(render.DefaultForeground)
			background := uint8(render.DefaultBackground)
			if selected {
				foreground, background = background, debugSelection
			}
			switch {
			case cluster.Tab:
				if selected {
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
	if cursorVisible {
		d.drawCursor(surface)
	}
	d.drawStatus(surface)
}

func (d *DebugBuffer) clusterSelected(line, runeOffset int) bool {
	selection := d.Editor.Selection
	switch selection.Kind {
	case editor.SelectionLine:
		first, last := selection.LineRange()
		return line >= first && line <= last
	case editor.SelectionChar:
		start, end := selection.Normalized()
		position := editor.Pos{Line: line, Rune: runeOffset}
		return !position.Before(start) && !end.Before(position)
	default:
		return false
	}
}

func (d *DebugBuffer) drawCursor(surface *render.Surface) {
	viewport := &d.Editor.Viewport
	position := d.Editor.Cursor.Pos
	if position.Line < viewport.TopLine || position.Line >= viewport.TopLine+viewport.Height {
		return
	}
	info := d.Editor.Buffer.Info(position)
	cells := d.Editor.Buffer.ClusterAt(position).Cells
	if cells < debugCursorCell {
		cells = debugCursorCell
	}
	x := (info.Cell - viewport.LeftCell) * font.NarrowWidth
	if x >= surface.Width || x+cells*font.NarrowWidth <= 0 {
		return
	}
	y := (position.Line - viewport.TopLine) * font.LineAdvance
	surface.DrawCursor(x, y, cells*font.NarrowWidth, render.CursorBlock)
}

func (d *DebugBuffer) drawStatus(surface *render.Surface) {
	y := debugStatusRow * font.LineAdvance
	surface.FillRect(0, y, surface.Width, font.CellHeight, render.DefaultBackground)

	name := d.Editor.Buffer.Path
	if name == "" {
		name = "[No Name]"
	} else {
		name = filepath.Base(name)
	}
	marker := ""
	if d.Editor.Buffer.Dirty() {
		marker = " *"
	}
	info := d.Editor.Buffer.Info(d.Editor.Cursor.Pos)
	status := fmt.Sprintf("%s%s %d:%d", name, marker, info.Pos.Line+1, info.Cell+1)
	if d.Status != "" {
		status += "  " + d.Status
	}
	color := uint8(debugStatusFg)
	if d.ErrorStatus {
		color = debugStatusErr
	}
	surface.DrawText(0, y, status, color, render.DefaultBackground)
}

// HandleText inserts committed text and keeps the insert undo group open.
func (d *DebugBuffer) HandleText(runes []rune) bool {
	if len(runes) == 0 {
		return false
	}
	d.Editor.BeginGroup()
	if d.Editor.Selection.Kind != editor.SelectionNone {
		if _, err := d.Editor.DeleteSelection(); err != nil {
			d.SetStatus(err.Error(), true)
			return true
		}
		d.Editor.ClearSelection()
	}
	if _, err := d.Editor.Insert(string(runes)); err != nil {
		d.SetStatus(err.Error(), true)
		return true
	}
	d.SetStatus("", false)
	return true
}

// HandleKey applies the temporary M2 key map. It reports whether the view needs
// a redraw.
func (d *DebugBuffer) HandleKey(event input.Event) bool {
	if event.Key == input.KeyUnknown || input.IsModifier(event.Key) {
		return false
	}
	shift := event.Mods&input.ModShift != 0
	control := event.Mods&input.ModCtrl != 0

	switch {
	case event.Key == input.KeyS && control:
		if event.Repeat {
			return false
		}
		d.Editor.EndGroup()
		if err := d.Editor.Save(); err != nil {
			d.SetStatus(err.Error(), true)
		} else {
			d.SetStatus("saved", false)
		}
		return true
	case event.Key == input.KeyZ && control:
		d.Editor.EndGroup()
		if _, ok := d.Editor.Undo(); ok {
			d.SetStatus("undo", false)
		}
		return true
	case event.Key == input.KeyY && control:
		d.Editor.EndGroup()
		if _, ok := d.Editor.Redo(); ok {
			d.SetStatus("redo", false)
		}
		return true
	case event.Key == input.KeyA && control:
		if event.Repeat {
			return false
		}
		d.Editor.EndGroup()
		d.Editor.SelectAll()
		return true
	case event.Key == input.KeyLeft && control:
		return d.move(shift, func() { d.Editor.MoveWordBackward(1) })
	case event.Key == input.KeyRight && control:
		return d.move(shift, func() { d.Editor.MoveWordForward(1) })
	case event.Key == input.KeyHome && control:
		return d.move(shift, d.Editor.MoveBufferStart)
	case event.Key == input.KeyEnd && control:
		return d.move(shift, d.Editor.MoveBufferEnd)
	case event.Key == input.KeyEsc:
		d.Editor.EndGroup()
		d.Editor.ClearSelection()
		d.SetStatus("", false)
		return true
	}

	switch event.Key {
	case input.KeyLeft:
		return d.move(shift, func() { d.Editor.MoveLeft(1) })
	case input.KeyRight:
		return d.move(shift, func() { d.Editor.MoveRight(1) })
	case input.KeyUp:
		return d.move(shift, func() { d.Editor.MoveUp(1) })
	case input.KeyDown:
		return d.move(shift, func() { d.Editor.MoveDown(1) })
	case input.KeyHome:
		return d.move(shift, d.Editor.MoveLineStart)
	case input.KeyEnd:
		return d.move(shift, d.Editor.MoveLineEnd)
	case input.KeyPageUp:
		return d.move(shift, d.Editor.MovePageUp)
	case input.KeyPageDown:
		return d.move(shift, d.Editor.MovePageDown)
	case input.KeyEnter:
		d.Editor.BeginGroup()
		if d.Editor.Selection.Kind != editor.SelectionNone {
			if _, err := d.Editor.DeleteSelection(); err != nil {
				d.SetStatus(err.Error(), true)
				return true
			}
			d.Editor.ClearSelection()
		}
		if _, err := d.Editor.SplitLine(); err != nil {
			d.SetStatus(err.Error(), true)
		}
		return true
	case input.KeyBackspace:
		d.Editor.BeginGroup()
		if d.Editor.Selection.Kind != editor.SelectionNone {
			if _, err := d.Editor.DeleteSelection(); err != nil {
				d.SetStatus(err.Error(), true)
			}
			d.Editor.ClearSelection()
			return true
		}
		if _, err := d.Editor.DeleteClusterBefore(); err != nil {
			d.SetStatus(err.Error(), true)
		}
		return true
	case input.KeyDelete:
		d.Editor.BeginGroup()
		if d.Editor.Selection.Kind != editor.SelectionNone {
			if _, err := d.Editor.DeleteSelection(); err != nil {
				d.SetStatus(err.Error(), true)
			}
			d.Editor.ClearSelection()
			return true
		}
		if _, err := d.Editor.DeleteCluster(); err != nil {
			d.SetStatus(err.Error(), true)
		}
		return true
	case input.KeyTab:
		d.Editor.BeginGroup()
		if d.Editor.Selection.Kind != editor.SelectionNone {
			if _, err := d.Editor.DeleteSelection(); err != nil {
				d.SetStatus(err.Error(), true)
				return true
			}
			d.Editor.ClearSelection()
		}
		if _, err := d.Editor.InsertTab(); err != nil {
			d.SetStatus(err.Error(), true)
		}
		return true
	}
	return false
}

func (d *DebugBuffer) move(shift bool, motion func()) bool {
	if shift {
		if d.Editor.Selection.Kind != editor.SelectionChar {
			d.Editor.StartSelection(editor.SelectionChar)
		}
	} else {
		d.Editor.ClearSelection()
	}
	motion()
	if shift {
		d.Editor.ExtendSelection()
	}
	return true
}

// SetStatus replaces the status message.
func (d *DebugBuffer) SetStatus(message string, isError bool) {
	d.Status = message
	d.ErrorStatus = isError
}

// SetPaste inserts clipboard text through the normal insert path. File
// normalization happens exactly once in Buffer.Apply.
func (d *DebugBuffer) SetPaste(text string) {
	if text == "" {
		return
	}
	d.HandleText([]rune(text))
}
