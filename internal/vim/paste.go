package vim

import (
	"strings"
	"unicode/utf8"

	"github.com/asimmons91/pico-editor/internal/editor"
)

func (v *Vim) paste(ed *editor.Editor, before bool) {
	if v.clipboardBusy {
		v.info("clipboard paste pending")
		return
	}
	count := v.takeCount()
	selector := v.selectedRegister()
	value, ok, pending := v.registerTextForPaste(selector)
	if pending {
		v.clipboardBusy = true
		v.pendingPaste = &pasteRequest{
			before:   before,
			count:    count,
			register: selector,
			editor:   ed,
			pos:      ed.Cursor.Pos,
		}
		return
	}
	if !ok {
		v.message("E353: Nothing in register")
		v.ringBell()
		return
	}
	v.pasteValue(ed, value, &pasteRequest{before: before, count: count, register: selector})
	v.recordChange(changeRecord{
		action:   changePaste,
		register: selector,
		count:    count,
		backward: before,
	})
}

func (v *Vim) pasteValue(ed *editor.Editor, value registerValue, request *pasteRequest) {
	if request == nil {
		request = &pasteRequest{count: 1}
	}
	count := max(1, request.count)
	if value.Text == "" {
		return
	}
	switch value.Kind {
	case registerLine:
		v.pasteLines(ed, value.Text, request.before, count)
	case registerBlock:
		v.pasteBlock(ed, value.Text, request.before, count)
	default:
		v.pasteChars(ed, value.Text, request.before, count)
	}
}

func (v *Vim) pasteChars(ed *editor.Editor, text string, before bool, count int) {
	position := ed.Cursor.Pos
	if !before {
		line := ed.Buffer.Lines[position.Line]
		if position.Rune < utf8.RuneCountInString(line) {
			position = ed.Buffer.NextCluster(position)
		}
	}
	text = strings.Repeat(text, count)
	if _, err := ed.Apply(editor.Edit{Start: position, End: position, Text: text}); err != nil {
		v.ringBell()
		return
	}
	// Vim leaves the cursor on the last pasted cluster for characterwise puts.
	ed.SetCursor(ed.Buffer.PrevCluster(ed.Cursor.Pos))
	v.clampNormal(ed)
}

func (v *Vim) pasteLines(ed *editor.Editor, text string, before bool, count int) {
	text = strings.TrimSuffix(text, "\n")
	lines := strings.Split(text, "\n")
	if count > 1 {
		repeated := make([]string, 0, len(lines)*count)
		for i := 0; i < count; i++ {
			repeated = append(repeated, lines...)
		}
		lines = repeated
	}
	line := ed.Cursor.Pos.Line
	var start editor.Pos
	var insert string
	var firstNewLine int
	if before {
		start = editor.Pos{Line: line}
		insert = strings.Join(lines, "\n") + "\n"
		firstNewLine = line
	} else {
		start = editor.Pos{Line: line, Rune: utf8.RuneCountInString(ed.Buffer.Lines[line])}
		insert = "\n" + strings.Join(lines, "\n")
		firstNewLine = line + 1
	}
	if _, err := ed.Apply(editor.Edit{Start: start, End: start, Text: insert}); err != nil {
		v.ringBell()
		return
	}
	setFirstNonBlank(ed, firstNewLine)
}

func (v *Vim) pasteBlock(ed *editor.Editor, text string, before bool, count int) {
	if ed.WindowEmpty() {
		v.ringBell()
		return
	}
	lines := strings.Split(text, "\n")
	if count > 1 {
		for i, line := range lines {
			lines[i] = strings.Repeat(line, count)
		}
	}
	first := ed.Cursor.Pos.Line
	last := first + len(lines) - 1
	cell := ed.Buffer.CellColumn(ed.Cursor.Pos)
	if !before {
		cells := ed.Buffer.ClusterAt(ed.Cursor.Pos).Cells
		if cells > 0 {
			cell += cells
		}
	}
	lastExisting := len(ed.Buffer.Lines) - 1
	endLine := min(last, lastExisting)
	newLines := make([]string, 0, last-first+1)
	for line := first; line <= last; line++ {
		original := ""
		if line < len(ed.Buffer.Lines) {
			original = ed.Buffer.Lines[line]
		}
		newLines = append(newLines, insertAtCell(ed, original, cell, lines[line-first]))
	}
	start := editor.Pos{Line: first}
	end := editor.Pos{Line: endLine, Rune: utf8.RuneCountInString(ed.Buffer.Lines[endLine])}
	textToApply := strings.Join(newLines, "\n")
	if last > lastExisting {
		// Replace through the end of the existing buffer and let the inserted
		// newlines create the remaining lines.
		end = editor.Pos{Line: lastExisting, Rune: utf8.RuneCountInString(ed.Buffer.Lines[lastExisting])}
	}
	if _, err := ed.Apply(editor.Edit{Start: start, End: end, Text: textToApply}); err != nil {
		v.ringBell()
		return
	}
	ed.SetCursor(ed.Buffer.PosFromCell(first, cell))
	v.clampNormal(ed)
}
