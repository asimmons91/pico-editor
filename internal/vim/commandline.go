package vim

import (
	"strings"
	"unicode"

	"github.com/asimmons91/pico-editor/internal/editor"
	"github.com/asimmons91/pico-editor/internal/input"
)

func (v *Vim) openEx(ed *editor.Editor, preRange string) {
	saved := ed.Cursor.Pos
	if preRange == "" && v.Mode.Visual() {
		v.saveVisual(ed)
		preRange = "'<,'>"
		v.leaveVisual(ed)
	}
	text := []rune(preRange)
	v.cmdline = commandLineState{
		active:       true,
		kind:         commandEx,
		text:         text,
		cursor:       len(text),
		savedCursor:  saved,
		savedTopLine: ed.Viewport.TopLine,
		historyPos:   len(v.exHistory),
		preRange:     preRange,
	}
	v.Mode = ModeCommandLine
	v.Count = 0
	v.Operator = OpNone
	v.pending = pendingState{}
}

func (v *Vim) handleCommandLine(ev input.Event, ed *editor.Editor) {
	switch ev.Kind {
	case input.KindText:
		if len(ev.Runes) == 0 {
			return
		}
		v.insertPromptRunes(ev.Runes, ed)
		return
	case input.KindKeyDown:
	default:
		return
	}

	if isEscape(ev.Key, ev.Mods) {
		v.cancelCommandLine(ed)
		return
	}
	ctrl := ev.Mods&input.ModCtrl != 0
	switch ev.Key {
	case input.KeyEnter:
		if v.cmdline.kind == commandEx {
			line := string(v.cmdline.text)
			v.cmdline = commandLineState{}
			v.Mode = ModeNormal
			v.executeEx(ed, line)
			return
		}
		v.commitSearch(ed)
	case input.KeyBackspace:
		if v.cmdline.cursor > 0 {
			v.cmdline.text = append(v.cmdline.text[:v.cmdline.cursor-1], v.cmdline.text[v.cmdline.cursor:]...)
			v.cmdline.cursor--
			v.afterPromptEdit(ed)
		}
	case input.KeyDelete:
		if v.cmdline.cursor < len(v.cmdline.text) {
			v.cmdline.text = append(v.cmdline.text[:v.cmdline.cursor], v.cmdline.text[v.cmdline.cursor+1:]...)
			v.afterPromptEdit(ed)
		}
	case input.KeyLeft:
		if v.cmdline.cursor > 0 {
			v.cmdline.cursor--
		}
	case input.KeyRight:
		if v.cmdline.cursor < len(v.cmdline.text) {
			v.cmdline.cursor++
		}
	case input.KeyHome:
		v.cmdline.cursor = 0
	case input.KeyEnd:
		v.cmdline.cursor = len(v.cmdline.text)
	case input.KeyUp:
		v.promptHistory(-1)
		v.afterPromptEdit(ed)
	case input.KeyDown:
		v.promptHistory(1)
		v.afterPromptEdit(ed)
	case input.KeyTab:
		if v.cmdline.kind == commandEx {
			v.completeCommand()
		}
	case input.KeyU:
		if ctrl {
			v.cmdline.text = append([]rune(nil), v.cmdline.text[v.cmdline.cursor:]...)
			v.cmdline.cursor = 0
			v.afterPromptEdit(ed)
		}
	case input.KeyW:
		if ctrl {
			v.deletePromptWord()
			v.afterPromptEdit(ed)
		}
	}
}

func (v *Vim) insertPromptRunes(runes []rune, ed *editor.Editor) {
	text := make([]rune, 0, len(v.cmdline.text)+len(runes))
	text = append(text, v.cmdline.text[:v.cmdline.cursor]...)
	text = append(text, runes...)
	text = append(text, v.cmdline.text[v.cmdline.cursor:]...)
	v.cmdline.text = text
	v.cmdline.cursor += len(runes)
	v.afterPromptEdit(ed)
}

func (v *Vim) afterPromptEdit(ed *editor.Editor) {
	if v.cmdline.kind == commandSearchForward || v.cmdline.kind == commandSearchBackward {
		v.search.highlight = true
		v.updateSearchPreview(ed)
	}
}

func (v *Vim) deletePromptWord() {
	if v.cmdline.cursor == 0 {
		return
	}
	start := v.cmdline.cursor
	for start > 0 && unicode.IsSpace(v.cmdline.text[start-1]) {
		start--
	}
	for start > 0 && !unicode.IsSpace(v.cmdline.text[start-1]) {
		start--
	}
	v.cmdline.text = append(v.cmdline.text[:start], v.cmdline.text[v.cmdline.cursor:]...)
	v.cmdline.cursor = start
}

func (v *Vim) promptHistory(delta int) {
	history := v.exHistory
	if v.cmdline.kind != commandEx {
		history = v.search.history
	}
	if len(history) == 0 {
		return
	}
	if v.cmdline.historyPos < 0 {
		v.cmdline.historyPos = len(history)
	}
	if v.cmdline.draft == "" && v.cmdline.historyPos == len(history) {
		v.cmdline.draft = string(v.cmdline.text)
	}
	next := v.cmdline.historyPos + delta
	if next < 0 {
		next = 0
	}
	if next > len(history) {
		next = len(history)
	}
	v.cmdline.historyPos = next
	if next == len(history) {
		v.cmdline.text = []rune(v.cmdline.draft)
	} else {
		v.cmdline.text = []rune(history[next])
	}
	v.cmdline.cursor = len(v.cmdline.text)
}

func (v *Vim) cancelCommandLine(ed *editor.Editor) {
	if v.cmdline.kind == commandSearchForward || v.cmdline.kind == commandSearchBackward {
		v.cancelSearch(ed)
		return
	}
	v.cmdline = commandLineState{}
	v.Mode = ModeNormal
}

func (v *Vim) completeCommand() {
	text := string(v.cmdline.text)
	if strings.ContainsAny(text, " \t") {
		return
	}
	prefix := text
	matches := make([]string, 0, len(exCommandNames))
	for _, name := range exCommandNames {
		if strings.HasPrefix(name, prefix) {
			matches = append(matches, name)
		}
	}
	if len(matches) == 0 {
		return
	}
	completion := matches[0]
	for _, name := range matches[1:] {
		completion = commonPrefix(completion, name)
	}
	if completion == "" {
		return
	}
	if len(matches) == 1 {
		completion += " "
	}
	v.cmdline.text = []rune(completion)
	v.cmdline.cursor = len(v.cmdline.text)
}

func commonPrefix(a, b string) string {
	limit := min(len(a), len(b))
	for i := 0; i < limit; i++ {
		if a[i] != b[i] {
			return a[:i]
		}
	}
	return a[:limit]
}

// Prompt reports the active command-line prompt. The prefix is ":", "/", or
// "?"; cursor is a rune offset into text.
func (v *Vim) Prompt() (prefix, text string, cursor int, active bool) {
	if v.Mode != ModeCommandLine || !v.cmdline.active {
		return "", "", 0, false
	}
	switch v.cmdline.kind {
	case commandSearchForward:
		prefix = "/"
	case commandSearchBackward:
		prefix = "?"
	default:
		prefix = ":"
	}
	return prefix, string(v.cmdline.text), v.cmdline.cursor, true
}
