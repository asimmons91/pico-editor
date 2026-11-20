package vim

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/asimmons91/pico-editor/internal/editor"
	"github.com/asimmons91/pico-editor/internal/input"
)

func (v *Vim) enterInsert(ed *editor.Editor, after, firstNonBlank bool) {
	count := v.takeCount()
	entry := entryInsert
	if firstNonBlank {
		entry = entryFirstNonBlank
		setFirstNonBlank(ed, ed.Cursor.Pos.Line)
	} else if after {
		entry = entryAppend
		line := ed.Buffer.Lines[ed.Cursor.Pos.Line]
		if ed.Cursor.Pos.Rune < utf8.RuneCountInString(line) {
			ed.SetCursor(ed.Buffer.NextCluster(ed.Cursor.Pos))
		}
	}
	v.beginInsert(ed, count, nil, entry)
}

func (v *Vim) enterInsertLineEnd(ed *editor.Editor) {
	count := v.takeCount()
	line := ed.Buffer.Lines[ed.Cursor.Pos.Line]
	ed.SetCursor(editor.Pos{Line: ed.Cursor.Pos.Line, Rune: utf8.RuneCountInString(line)})
	v.beginInsert(ed, count, nil, entryAppendEOL)
}

// EnterInsertAtCursor starts insert mode at the current cursor without moving
// it or opening a new line. The glyph picker uses it after inserting text in
// normal mode.
func (v *Vim) EnterInsertAtCursor(ed *editor.Editor) {
	ed.BeginGroup()
	v.Mode = ModeInsert
	v.insert = insertState{
		active:       true,
		count:        1,
		entry:        entryInsert,
		start:        ed.Cursor.Pos,
		startHistory: len(ed.History.Undo),
	}
	v.Count = 0
	v.opCount = 0
	v.Operator = OpNone
	v.pending = pendingState{}
}

func (v *Vim) enterInsertWithRecord(ed *editor.Editor, record *changeRecord) {
	v.beginInsert(ed, 1, record, entryInsert)
}

func (v *Vim) beginInsert(ed *editor.Editor, count int, record *changeRecord, entry insertEntry) {
	ed.BeginGroup()
	start := ed.Cursor.Pos
	v.Mode = ModeInsert
	v.insert = insertState{
		active:       true,
		count:        max(1, count),
		entry:        entry,
		start:        start,
		startHistory: len(ed.History.Undo),
		pendingDot:   record,
	}
	v.Count = 0
	v.opCount = 0
	v.Operator = OpNone
	v.pending = pendingState{}
}

func (v *Vim) openLine(ed *editor.Editor, below bool) {
	count := v.takeCount()
	ed.BeginGroup()
	line := ed.Cursor.Pos.Line
	if below {
		length := utf8.RuneCountInString(ed.Buffer.Lines[line])
		ed.SetCursor(editor.Pos{Line: line, Rune: length})
		_, _ = ed.Insert("\n")
		for i := 1; i < count; i++ {
			_, _ = ed.Insert("\n")
		}
	} else {
		ed.SetCursor(editor.Pos{Line: line})
		_, _ = ed.Insert("\n")
		ed.SetCursor(editor.Pos{Line: line})
		for i := 1; i < count; i++ {
			_, _ = ed.Insert("\n")
			ed.SetCursor(editor.Pos{Line: line})
		}
	}
	v.Mode = ModeInsert
	entry := entryOpenBelow
	if !below {
		entry = entryOpenAbove
	}
	v.insert = insertState{
		active:       true,
		count:        1,
		entry:        entry,
		start:        ed.Cursor.Pos,
		startHistory: len(ed.History.Undo),
	}
	v.Count = 0
	v.opCount = 0
	v.Operator = OpNone
	v.pending = pendingState{}
}

func (v *Vim) handleInsert(ev input.Event, ed *editor.Editor) {
	switch ev.Kind {
	case input.KindText:
		if len(ev.Runes) == 0 {
			return
		}
		ed.BeginGroup()
		_, _ = ed.Insert(string(ev.Runes))
		return
	case input.KindKeyDown:
	default:
		return
	}

	if isEscape(ev.Key, ev.Mods) {
		v.finishInsert(ed)
		return
	}
	ctrl := ev.Mods&input.ModCtrl != 0
	switch ev.Key {
	case input.KeyEnter:
		ed.BeginGroup()
		_, _ = ed.SplitLine()
	case input.KeyBackspace:
		ed.BeginGroup()
		_, _ = ed.DeleteClusterBefore()
	case input.KeyDelete:
		ed.BeginGroup()
		_, _ = ed.DeleteCluster()
	case input.KeyTab:
		ed.BeginGroup()
		_, _ = ed.InsertTab()
	case input.KeyLeft:
		ed.MoveLeft(1)
		ed.BeginGroup()
	case input.KeyRight:
		ed.MoveRight(1)
		ed.BeginGroup()
	case input.KeyUp:
		ed.MoveUp(1)
		ed.BeginGroup()
	case input.KeyDown:
		ed.MoveDown(1)
		ed.BeginGroup()
	case input.KeyHome:
		ed.MoveLineStart()
		ed.BeginGroup()
	case input.KeyEnd:
		ed.MoveLineEnd()
		ed.BeginGroup()
	case input.KeyW:
		if ctrl {
			v.deletePreviousWord(ed)
		}
	case input.KeyU:
		if ctrl {
			v.deleteToLineStart(ed)
		}
	case input.KeyJ, input.KeyK, input.KeyP:
		if ctrl {
			// P8SCII input modes are owned by M6. The key is deliberately not
			// inserted as text.
			v.info("P8SCII input modes are available in M6")
		}
	}
}

func (v *Vim) finishInsert(ed *editor.Editor) {
	if !v.insert.active {
		v.Mode = ModeNormal
		return
	}
	if v.insert.block != nil {
		v.finishBlockInsert(ed)
		return
	}

	typed := ""
	if !ed.Cursor.Pos.Before(v.insert.start) {
		if text, err := ed.Buffer.TextInRange(v.insert.start, ed.Cursor.Pos); err == nil {
			typed = text
		}
	}
	count := v.insert.count
	if count > 1 && typed != "" {
		for i := 1; i < count; i++ {
			ed.BeginGroup()
			_, _ = ed.Insert(typed)
		}
	}
	if typed != "" || v.insert.pendingDot != nil {
		if v.insert.pendingDot != nil {
			record := *v.insert.pendingDot
			record.text = typed
			v.recordChange(record)
		} else {
			v.recordChange(changeRecord{
				action: changeInsert,
				text:   typed,
				count:  count,
				entry:  v.insert.entry,
			})
		}
	}

	ed.EndGroup()
	v.Mode = ModeNormal
	if ed.Cursor.Pos.Rune > 0 {
		ed.SetCursor(ed.Buffer.PrevCluster(ed.Cursor.Pos))
	}
	v.clampNormal(ed)
	v.insert = insertState{}
}

func (v *Vim) deletePreviousWord(ed *editor.Editor) {
	start := ed.Cursor.Pos
	position := start
	line := ed.Buffer.Lines[position.Line]
	for position.Rune > 0 && baseRuneAt(line, prevClusterRune(line, position.Rune)) == ' ' {
		position.Rune = prevClusterRune(line, position.Rune)
	}
	for position.Rune > 0 && baseRuneAt(line, prevClusterRune(line, position.Rune)) != ' ' {
		position.Rune = prevClusterRune(line, position.Rune)
	}
	if position == start {
		return
	}
	ed.BeginGroup()
	_, _ = ed.DeleteRange(position, start)
}

func (v *Vim) deleteToLineStart(ed *editor.Editor) {
	start := ed.Cursor.Pos
	if start.Rune == 0 {
		return
	}
	ed.BeginGroup()
	_, _ = ed.DeleteRange(editor.Pos{Line: start.Line}, start)
}

func (v *Vim) deleteClusters(ed *editor.Editor, forward, change bool) {
	count := v.takeCount()
	if ed.WindowEmpty() {
		v.ringBell()
		return
	}
	start := ed.Cursor.Pos
	line := ed.Buffer.Lines[start.Line]
	rangeStart := start
	rangeEnd := start
	if forward {
		position := start.Rune
		for i := 0; i < count && position < utf8.RuneCountInString(line); i++ {
			position = nextClusterRune(line, position)
		}
		rangeEnd = editor.Pos{Line: start.Line, Rune: position}
	} else {
		position := start.Rune
		for i := 0; i < count && position > 0; i++ {
			position = prevClusterRune(line, position)
		}
		rangeStart = editor.Pos{Line: start.Line, Rune: position}
	}
	if rangeStart == rangeEnd {
		if change && len(line) == 0 {
			record := changeRecord{action: changeOperator, operator: OpChange, motion: 'l', count: count}
			v.enterInsertWithRecord(ed, &record)
			return
		}
		v.ringBell()
		return
	}
	text, err := ed.Buffer.TextInRange(rangeStart, rangeEnd)
	if err != nil {
		v.ringBell()
		return
	}
	selector := v.selectedRegister()
	v.recordRegister(text, registerChar, selector, false)
	if change {
		ed.BeginGroup()
	}
	if _, err := ed.DeleteRange(rangeStart, rangeEnd); err != nil {
		v.ringBell()
		return
	}
	ed.SetCursor(rangeStart)
	if change {
		record := changeRecord{action: changeOperator, operator: OpChange, motion: 'l', count: count}
		v.enterInsertWithRecord(ed, &record)
		return
	}
	v.recordChange(changeRecord{
		action:   changeOperator,
		operator: OpDelete,
		motion:   'l',
		count:    count,
		backward: !forward,
	})
}

func (v *Vim) changeWholeLines(ed *editor.Editor) {
	count := v.takeCount()
	first := ed.Cursor.Pos.Line
	last := min(first+count-1, ed.WindowLast())
	start := ed.Cursor.Pos
	v.applyOperatorLines(ed, OpChange, first, last, 0, count, start)
}

func (v *Vim) replaceClusters(ed *editor.Editor, replacement rune, count int) {
	if ed.WindowEmpty() {
		v.ringBell()
		return
	}
	start := ed.Cursor.Pos
	line := ed.Buffer.Lines[start.Line]
	position := start.Rune
	lastStart := start.Rune
	replaced := 0
	for replaced < count && position < utf8.RuneCountInString(line) {
		lastStart = position
		position = nextClusterRune(line, position)
		replaced++
	}
	if replaced == 0 {
		v.ringBell()
		return
	}
	text := strings.Repeat(string(replacement), replaced)
	if _, err := ed.Apply(editor.Edit{
		Start: start,
		End:   editor.Pos{Line: start.Line, Rune: position},
		Text:  text,
	}); err != nil {
		v.ringBell()
		return
	}
	ed.SetCursor(editor.Pos{Line: start.Line, Rune: lastStart})
	v.recordChange(changeRecord{
		action: changeReplace,
		text:   string(replacement),
		count:  replaced,
	})
}

func (v *Vim) joinLines(ed *editor.Editor) {
	count := v.Count
	v.Count = 0
	if count < 2 {
		count = 2
	}
	if ed.WindowEmpty() {
		v.ringBell()
		return
	}
	first := ed.Cursor.Pos.Line
	last := min(first+count-1, ed.WindowLast())
	if last <= first {
		v.ringBell()
		return
	}
	lines := ed.Buffer.Lines
	joined := lines[first]
	cursorRune := utf8.RuneCountInString(joined)
	for line := first + 1; line <= last; line++ {
		separator := " "
		if joined != "" {
			lastRune, _ := utf8.DecodeLastRuneInString(joined)
			if unicode.IsSpace(lastRune) {
				separator = ""
			}
		}
		if lines[line] != "" {
			firstRune, _ := utf8.DecodeRuneInString(lines[line])
			if firstRune == ')' || firstRune == ']' || firstRune == '}' {
				separator = ""
			}
		}
		joined += separator + lines[line]
	}
	if _, err := ed.Apply(editor.Edit{
		Start: editor.Pos{Line: first},
		End:   editor.Pos{Line: last, Rune: utf8.RuneCountInString(ed.Buffer.Lines[last])},
		Text:  joined,
	}); err != nil {
		v.ringBell()
		return
	}
	ed.SetCursor(editor.Pos{Line: first, Rune: cursorRune})
	v.recordChange(changeRecord{action: changeJoin, count: count})
}

func (v *Vim) toggleCase(ed *editor.Editor) {
	count := v.takeCount()
	if ed.WindowEmpty() {
		v.ringBell()
		return
	}
	start := ed.Cursor.Pos
	line := ed.Buffer.Lines[start.Line]
	position := start.Rune
	var replacements strings.Builder
	changed := 0
	for changed < count && position < utf8.RuneCountInString(line) {
		next := nextClusterRune(line, position)
		cluster := []rune(runesBetween(line, position, next))
		if len(cluster) > 0 {
			r := cluster[0]
			switch {
			case unicode.IsUpper(r):
				r = unicode.ToLower(r)
			case unicode.IsLower(r):
				r = unicode.ToUpper(r)
			}
			cluster[0] = r
		}
		replacements.WriteString(string(cluster))
		position = next
		changed++
	}
	if changed == 0 {
		v.ringBell()
		return
	}
	text := replacements.String()
	if _, err := ed.Apply(editor.Edit{
		Start: start,
		End:   editor.Pos{Line: start.Line, Rune: position},
		Text:  text,
	}); err != nil {
		v.ringBell()
		return
	}
	ed.SetCursor(editor.Pos{Line: start.Line, Rune: position})
	v.recordChange(changeRecord{action: changeToggle, count: changed})
}
