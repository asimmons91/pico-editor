package app

import (
	"strings"
	"unicode"

	"github.com/asimmons91/pico-editor/internal/font"
	"github.com/asimmons91/pico-editor/internal/input"
	"github.com/asimmons91/pico-editor/internal/render"
)

// DialogKind identifies the app flow that owns a modal dialog.
type DialogKind uint8

const (
	DialogConflict DialogKind = iota + 1
	DialogDeleted
	DialogOutsideRoot
	DialogQuit
	DialogError
	DialogConfigError
)

// DialogOption is one modal choice.
type DialogOption struct {
	Label string
}

// Dialog is a small keyboard-driven modal panel.
type Dialog struct {
	Kind      DialogKind
	Title     string
	Message   []string
	Options   []DialogOption
	Selected  int
	Chosen    bool
	Operation string
	Path      string
}

// ShowDialog opens a modal dialog and selects defaultIndex.
func (s *Session) ShowDialog(kind DialogKind, title, message string, options []DialogOption, defaultIndex int) {
	s.glyphPickerPending = false
	if len(options) == 0 {
		options = []DialogOption{{Label: "OK"}}
	}
	if ed := s.CurrentEditor(); ed != nil {
		s.Vim.ResetPending(ed)
	}
	if defaultIndex < 0 || defaultIndex >= len(options) {
		defaultIndex = 0
	}
	s.Dialog = &Dialog{
		Kind:     kind,
		Title:    title,
		Message:  wrapDialogMessage(message),
		Options:  options,
		Selected: defaultIndex,
	}
	s.Help = nil
}

// ShowError opens a one-button error dialog.
func (s *Session) ShowError(title string, err error) {
	s.ShowErrorDetail(title, "", "", err)
}

// ShowErrorDetail adds the operation and path that produced an error.
func (s *Session) ShowErrorDetail(title, operation, path string, err error) {
	if err == nil {
		return
	}
	s.ShowDialog(DialogError, title, err.Error(), []DialogOption{{Label: "OK"}}, 0)
	if s.Dialog != nil {
		s.Dialog.Operation = operation
		s.Dialog.Path = path
	}
}

// TakeDialogResult returns the selected option once and closes the dialog.
func (s *Session) TakeDialogResult() (kind DialogKind, choice int, ok bool) {
	if s.Dialog == nil || !s.Dialog.Chosen {
		return 0, 0, false
	}
	kind = s.Dialog.Kind
	choice = s.Dialog.Selected
	s.Dialog = nil
	return kind, choice, true
}

// DialogOpen reports whether input is currently captured by a dialog.
func (s *Session) DialogOpen() bool {
	return s.Dialog != nil
}

func (s *Session) handleDialog(ev input.Event) bool {
	dialog := s.Dialog
	if dialog == nil || dialog.Chosen {
		return false
	}
	if ev.Kind != input.KindKeyDown {
		return false
	}
	switch ev.Key {
	case input.KeyLeft, input.KeyUp:
		dialog.Selected = (dialog.Selected - 1 + len(dialog.Options)) % len(dialog.Options)
		return true
	case input.KeyRight, input.KeyDown, input.KeyTab:
		if ev.Mods&input.ModShift != 0 && ev.Key == input.KeyTab {
			dialog.Selected = (dialog.Selected - 1 + len(dialog.Options)) % len(dialog.Options)
		} else {
			dialog.Selected = (dialog.Selected + 1) % len(dialog.Options)
		}
		return true
	case input.KeyEnter:
		dialog.Chosen = true
		return true
	case input.KeyEsc:
		if dialog.Kind == DialogConfigError {
			dialog.Selected = 0
		} else {
			dialog.Selected = cancelOption(dialog.Options)
		}
		dialog.Chosen = true
		return true
	}
	if ev.Mods&(input.ModCtrl|input.ModAlt|input.ModMeta) != 0 {
		return false
	}
	if ev.Key >= input.KeyA && ev.Key <= input.KeyZ {
		letter := rune('a' + int(ev.Key-input.KeyA))
		for index, option := range dialog.Options {
			if optionAccelerator(option.Label) == letter {
				dialog.Selected = index
				dialog.Chosen = true
				s.suppressText = letter
				return true
			}
		}
	}
	return false
}

func cancelOption(options []DialogOption) int {
	for index, option := range options {
		label := strings.ToLower(option.Label)
		if label == "cancel" || label == "ok" {
			return index
		}
	}
	return len(options) - 1
}

func optionAccelerator(label string) rune {
	for _, r := range label {
		if unicode.IsLetter(r) {
			return unicode.ToLower(r)
		}
	}
	return 0
}

func wrapDialogMessage(message string) []string {
	message = strings.ReplaceAll(message, "\t", " ")
	words := strings.Fields(message)
	if len(words) == 0 {
		return nil
	}
	const width = 40
	var lines []string
	current := ""
	for _, word := range words {
		if current == "" {
			current = word
			continue
		}
		if len([]rune(current))+1+len([]rune(word)) > width {
			lines = append(lines, current)
			current = word
			continue
		}
		current += " " + word
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func (s *Session) drawDialog(surface *render.Surface) {
	dialog := s.Dialog
	if dialog == nil {
		return
	}
	widthCells := 60
	heightRows := 5 + len(dialog.Message)
	if dialog.Kind == DialogError || dialog.Kind == DialogConfigError {
		widthCells = 48
		if heightRows < 10 {
			heightRows = 10
		}
	}
	if heightRows < 7 {
		heightRows = 7
	}
	width := widthCells * font.NarrowWidth
	height := heightRows * font.LineAdvance
	x := (surface.Width - width) / 2
	y := (surface.Height - height) / 2

	surface.FillRect(x, y, width, height, render.DefaultBackground)
	surface.FillRect(x, y, width, 1, 6)
	surface.FillRect(x, y+height-1, width, 1, 6)
	surface.FillRect(x, y, 1, height, 6)
	surface.FillRect(x+width-1, y, 1, height, 6)
	surface.DrawText(x+4, y+4, clipCells(dialog.Title, widthCells-8), 10, render.DefaultBackground)
	for index, line := range dialog.Message {
		surface.DrawText(x+4, y+12+index*font.LineAdvance, clipCells(line, widthCells-8), 7, render.DefaultBackground)
	}

	optionY := y + height - 2*font.LineAdvance
	if dialog.Path != "" || dialog.Operation != "" {
		detail := dialog.Operation
		if dialog.Path != "" {
			if detail != "" {
				detail += ": "
			}
			detail += dialog.Path
		}
		surface.DrawText(x+4, optionY-font.LineAdvance, clipCells(detail, widthCells-8), 6, render.DefaultBackground)
	}
	cursor := x + 4
	for index, option := range dialog.Options {
		label := " " + option.Label + " "
		cells := font.CellWidth(label)
		bg := uint8(render.DefaultBackground)
		fg := uint8(7)
		if index == dialog.Selected {
			bg = selectionBackground
			fg = uint8(render.DefaultBackground)
		}
		surface.FillRect(cursor, optionY, cells*font.NarrowWidth, font.CellHeight, bg)
		surface.DrawText(cursor, optionY, label, fg, bg)
		cursor += (cells + 1) * font.NarrowWidth
	}
}
