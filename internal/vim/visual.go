package vim

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/asimmons91/pico-editor/internal/editor"
)

func (v *Vim) handleVisualRune(r rune, ed *editor.Editor) {
	switch r {
	case 'o':
		v.swapVisual(ed)
	case 'g':
		v.pending = pendingState{kind: pendG}
	case 'v':
		if v.Mode == ModeVisualChar {
			v.saveVisual(ed)
			v.leaveVisual(ed)
			return
		}
		v.setVisualKind(ed, ModeVisualChar)
	case 'V':
		if v.Mode == ModeVisualLine {
			v.saveVisual(ed)
			v.leaveVisual(ed)
			return
		}
		v.setVisualKind(ed, ModeVisualLine)
	case ':':
		v.openEx(ed, "")
	case 'd', 'c', 'y', '>', '<', '~', 'u', 'U', 'J':
		v.executeVisualOperator(r, ed)
	case 'p':
		v.visualPaste(ed, false)
	case 'P':
		v.visualPaste(ed, true)
	case 'I':
		if v.Mode == ModeVisualBlock {
			v.startBlockInsert(ed, false)
		} else {
			v.ringBell()
		}
	case 'A':
		if v.Mode == ModeVisualBlock {
			v.startBlockInsert(ed, true)
		} else {
			v.ringBell()
		}
	default:
		v.executeMotionKey(r, ed)
	}
}

func (v *Vim) setVisualKind(ed *editor.Editor, mode Mode) {
	kind := editor.SelectionChar
	switch mode {
	case ModeVisualLine:
		kind = editor.SelectionLine
	case ModeVisualBlock:
		kind = editor.SelectionBlock
	}
	v.Mode = mode
	ed.Selection.Kind = kind
	ed.Selection.Cursor = ed.Cursor.Pos
	v.visual = visualState{anchor: ed.Selection.Anchor, cursor: ed.Cursor.Pos, kind: kind}
}

func (v *Vim) swapVisual(ed *editor.Editor) {
	anchor := ed.Selection.Anchor
	cursor := ed.Cursor.Pos
	ed.Selection.Anchor = cursor
	ed.SetCursor(anchor)
	ed.Selection.Cursor = anchor
	v.visual.anchor, v.visual.cursor = ed.Selection.Anchor, ed.Selection.Cursor
}

func (v *Vim) executeVisualOperator(op rune, ed *editor.Editor) {
	if ed.Selection.Kind == editor.SelectionNone {
		v.leaveVisual(ed)
		return
	}
	record := v.visualRecord(op, ed)
	if ed.Selection.Kind == editor.SelectionBlock {
		v.executeBlockOperator(op, ed, record)
		return
	}

	first, last := ed.Selection.LineRange()
	registerKind := registerChar
	if ed.Selection.Kind == editor.SelectionLine {
		registerKind = registerLine
	}
	switch op {
	case 'd', 'c', 'y':
		text, ok := ed.SelectedText()
		if !ok {
			v.ringBell()
			v.leaveVisual(ed)
			return
		}
		selector := v.selectedRegister()
		if op != 'y' {
			v.recordRegister(text, registerKind, selector, false)
		} else {
			v.recordRegister(text, registerKind, selector, true)
		}
		if op == 'y' {
			start, _ := ed.Selection.Normalized()
			v.saveVisual(ed)
			ed.SetCursor(start)
			v.leaveVisual(ed)
			return
		}
		if op == 'c' && ed.Selection.Kind == editor.SelectionLine {
			ed.BeginGroup()
			if err := v.changeLines(ed, first, last); err != nil {
				ed.EndGroup()
				v.ringBell()
				v.leaveVisual(ed)
				return
			}
			v.saveVisual(ed)
			ed.ClearSelection()
			record.action = changeVisual
			record.operator = OpChange
			v.enterInsertWithRecord(ed, &record)
			return
		}
		if op == 'c' {
			ed.BeginGroup()
		}
		if _, err := ed.DeleteSelection(); err != nil {
			v.ringBell()
			v.leaveVisual(ed)
			return
		}
		v.saveVisual(ed)
		ed.ClearSelection()
		if op == 'c' {
			record.action = changeVisual
			record.operator = OpChange
			v.enterInsertWithRecord(ed, &record)
			return
		}
		v.leaveVisual(ed)
		v.recordChange(record)
	case '>', '<':
		delta := shiftWidth
		if op == '<' {
			delta = -shiftWidth
		}
		if _, err := ed.IndentLines(first, last, delta); err != nil {
			v.ringBell()
			v.leaveVisual(ed)
			return
		}
		v.saveVisual(ed)
		setFirstNonBlank(ed, first)
		v.leaveVisual(ed)
		v.recordChange(record)
	case '~', 'u', 'U':
		start, end := visualCharRange(ed)
		if ed.Selection.Kind == editor.SelectionLine {
			start = editor.Pos{Line: first}
			end = editor.Pos{Line: last, Rune: utf8.RuneCountInString(ed.Buffer.Lines[last])}
		}
		text, err := ed.Buffer.TextInRange(start, end)
		if err != nil {
			v.ringBell()
			v.leaveVisual(ed)
			return
		}
		if _, err := ed.Apply(editor.Edit{
			Start: start,
			End:   end,
			Text:  transformCase(text, op),
		}); err != nil {
			v.ringBell()
			v.leaveVisual(ed)
			return
		}
		v.saveVisual(ed)
		ed.SetCursor(start)
		v.leaveVisual(ed)
		v.recordChange(record)
	case 'J':
		if err := v.joinVisualLines(ed, first, last); err != nil {
			v.ringBell()
			v.leaveVisual(ed)
			return
		}
		v.saveVisual(ed)
		v.leaveVisual(ed)
		v.recordChange(record)
	}
}

func (v *Vim) visualRecord(op rune, ed *editor.Editor) changeRecord {
	first, last := ed.Selection.LineRange()
	record := changeRecord{
		action:      changeVisual,
		caseOp:      op,
		visualKind:  ed.Selection.Kind,
		visualLines: last - first + 1,
	}
	switch ed.Selection.Kind {
	case editor.SelectionBlock:
		_, _, left, right := ed.Selection.BlockRange(ed.Buffer)
		record.visualCells = right - left
		record.block = true
	case editor.SelectionChar:
		start, end := ed.Selection.Normalized()
		record.visualCells = ed.Buffer.Info(end).Cell - ed.Buffer.Info(start).Cell
	}
	return record
}

func visualCharRange(ed *editor.Editor) (editor.Pos, editor.Pos) {
	start, end := ed.Selection.Normalized()
	if end.Line < len(ed.Buffer.Lines) {
		line := ed.Buffer.Lines[end.Line]
		if end.Rune < utf8.RuneCountInString(line) {
			end = ed.Buffer.NextCluster(end)
		}
	}
	return start, end
}

func transformCase(text string, op rune) string {
	var out strings.Builder
	for _, r := range text {
		switch op {
		case '~':
			switch {
			case unicode.IsUpper(r):
				r = unicode.ToLower(r)
			case unicode.IsLower(r):
				r = unicode.ToUpper(r)
			}
		case 'u':
			r = unicode.ToLower(r)
		case 'U':
			r = unicode.ToUpper(r)
		}
		out.WriteRune(r)
	}
	return out.String()
}

func (v *Vim) joinVisualLines(ed *editor.Editor, first, last int) error {
	if last <= first {
		return nil
	}
	return v.joinLineRange(ed, first, last)
}

func (v *Vim) joinLineRange(ed *editor.Editor, first, last int) error {
	lines := ed.Buffer.Lines
	joined := lines[first]
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
	_, err := ed.Apply(editor.Edit{
		Start: editor.Pos{Line: first},
		End:   editor.Pos{Line: last, Rune: utf8.RuneCountInString(lines[last])},
		Text:  joined,
	})
	if err == nil {
		ed.SetCursor(editor.Pos{Line: first, Rune: firstNonBlankRune(joined)})
	}
	return err
}

func (v *Vim) executeBlockOperator(op rune, ed *editor.Editor, record changeRecord) {
	first, last, left, right := ed.Selection.BlockRange(ed.Buffer)
	width := right - left + 1
	switch op {
	case 'd', 'c', 'y':
		text := selectedBlockText(ed, first, last, width)
		selector := v.selectedRegister()
		if op == 'y' {
			v.recordRegister(text, registerBlock, selector, true)
			v.saveVisual(ed)
			ed.SetCursor(ed.Buffer.PosFromCell(first, left))
			v.leaveVisual(ed)
			return
		}
		v.recordRegister(text, registerBlock, selector, false)
		originals := append([]string(nil), ed.Buffer.Lines[first:last+1]...)
		historyLen := len(ed.History.Undo)
		if op == 'c' {
			ed.BeginGroup()
		}
		if _, err := deleteBlockLines(ed, first, last, left, right); err != nil {
			v.ringBell()
			v.leaveVisual(ed)
			return
		}
		v.saveVisual(ed)
		if op == 'c' {
			ed.ClearSelection()
			v.startBlockChangeInsert(ed, first, last, left, right, originals, historyLen, record)
			return
		}
		v.leaveVisual(ed)
		v.recordChange(record)
	case '>', '<':
		delta := shiftWidth
		if op == '<' {
			delta = -shiftWidth
		}
		if _, err := ed.IndentLines(first, last, delta); err != nil {
			v.ringBell()
			v.leaveVisual(ed)
			return
		}
		v.saveVisual(ed)
		ed.SetCursor(ed.Buffer.PosFromCell(first, left))
		v.leaveVisual(ed)
		v.recordChange(record)
	case '~', 'u', 'U':
		if err := transformBlock(ed, first, last, left, right, op); err != nil {
			v.ringBell()
			v.leaveVisual(ed)
			return
		}
		v.saveVisual(ed)
		ed.SetCursor(ed.Buffer.PosFromCell(first, left))
		v.leaveVisual(ed)
		v.recordChange(record)
	case 'J':
		if err := v.joinLineRange(ed, first, last); err != nil {
			v.ringBell()
			v.leaveVisual(ed)
			return
		}
		v.saveVisual(ed)
		v.leaveVisual(ed)
		v.recordChange(record)
	case 'p', 'P':
		v.visualPaste(ed, op == 'P')
	}
}

func selectedBlockText(ed *editor.Editor, first, last, width int) string {
	lines := make([]string, 0, last-first+1)
	for line := first; line <= last; line++ {
		start, end, ok := ed.Selection.BlockLineRange(ed.Buffer, line)
		text := ""
		if ok {
			text, _ = ed.Buffer.TextInRange(start, end)
		}
		if cellWidth := textCellWidth(ed, text); cellWidth < width {
			text += strings.Repeat(" ", width-cellWidth)
		}
		lines = append(lines, text)
	}
	return strings.Join(lines, "\n")
}

func textCellWidth(ed *editor.Editor, text string) int {
	cell := 0
	for cluster := range editor.LineClusters(text, ed.Buffer.TabWidth) {
		cell = cluster.Cell + cluster.Cells
	}
	return cell
}

func deleteBlockLines(ed *editor.Editor, first, last, left, right int) (string, error) {
	selection := ed.Selection
	replacement := make([]string, 0, last-first+1)
	for line := first; line <= last; line++ {
		text := ed.Buffer.Lines[line]
		start, end, ok := selection.BlockLineRange(ed.Buffer, line)
		if !ok {
			replacement = append(replacement, text)
			continue
		}
		startByte := byteOffsetForRune(text, start.Rune)
		endByte := byteOffsetForRune(text, end.Rune)
		replacement = append(replacement, text[:startByte]+text[endByte:])
	}
	_, err := ed.Apply(editor.Edit{
		Start: editor.Pos{Line: first},
		End:   editor.Pos{Line: last, Rune: utf8.RuneCountInString(ed.Buffer.Lines[last])},
		Text:  strings.Join(replacement, "\n"),
	})
	if err == nil {
		ed.SetCursor(ed.Buffer.PosFromCell(first, left))
	}
	return strings.Join(replacement, "\n"), err
}

func transformBlock(ed *editor.Editor, first, last, left, right int, op rune) error {
	selection := ed.Selection
	lines := make([]string, 0, last-first+1)
	for line := first; line <= last; line++ {
		text := ed.Buffer.Lines[line]
		start, end, ok := selection.BlockLineRange(ed.Buffer, line)
		if !ok {
			lines = append(lines, text)
			continue
		}
		startByte := byteOffsetForRune(text, start.Rune)
		endByte := byteOffsetForRune(text, end.Rune)
		lines = append(lines, text[:startByte]+transformCase(text[startByte:endByte], op)+text[endByte:])
	}
	_, err := ed.Apply(editor.Edit{
		Start: editor.Pos{Line: first},
		End:   editor.Pos{Line: last, Rune: utf8.RuneCountInString(ed.Buffer.Lines[last])},
		Text:  strings.Join(lines, "\n"),
	})
	if err == nil {
		ed.SetCursor(ed.Buffer.PosFromCell(first, left))
	}
	return err
}

func byteOffsetForRune(line string, runeIndex int) int {
	offset := 0
	seen := 0
	for offset < len(line) && seen < runeIndex {
		_, size := utf8.DecodeRuneInString(line[offset:])
		offset += size
		seen++
	}
	return offset
}

func (v *Vim) startBlockInsert(ed *editor.Editor, appendMode bool) {
	first, last, left, right := ed.Selection.BlockRange(ed.Buffer)
	v.saveVisual(ed)
	ed.ClearSelection()
	v.startBlockInsertAtRange(ed, first, last, left, right, appendMode)
}

func (v *Vim) startBlockChangeInsert(ed *editor.Editor, first, last, left, right int, originals []string, historyLen int, record changeRecord) {
	ed.BeginGroup()
	padLineToCell(ed, first, left)
	ed.SetCursor(ed.Buffer.PosFromCell(first, left))
	v.Mode = ModeInsert
	v.insert = insertState{
		active:       true,
		count:        1,
		start:        ed.Cursor.Pos,
		startHistory: historyLen,
		pendingDot:   &record,
		block: &blockInsert{
			first:       first,
			last:        last,
			cell:        left,
			originals:   originals,
			historyLen:  historyLen,
			changeBlock: true,
			left:        left,
			right:       right,
		},
	}
	v.Count = 0
	v.opCount = 0
	v.Operator = OpNone
	v.pending = pendingState{}
}

func (v *Vim) startBlockInsertAtRange(ed *editor.Editor, first, last, left, right int, appendMode bool) {
	originals := append([]string(nil), ed.Buffer.Lines[first:last+1]...)
	cell := left
	if appendMode {
		cell = right + 1
	}
	historyLen := len(ed.History.Undo)
	ed.BeginGroup()
	padLineToCell(ed, first, cell)
	ed.SetCursor(ed.Buffer.PosFromCell(first, cell))
	v.Mode = ModeInsert
	v.insert = insertState{
		active:       true,
		count:        1,
		start:        ed.Cursor.Pos,
		startHistory: historyLen,
		block: &blockInsert{
			first:      first,
			last:       last,
			cell:       cell,
			appendMode: appendMode,
			originals:  originals,
			historyLen: historyLen,
		},
	}
	v.Count = 0
	v.opCount = 0
	v.Operator = OpNone
	v.pending = pendingState{}
}

func padLineToCell(ed *editor.Editor, line, cell int) {
	width := ed.Buffer.CellColumn(editor.Pos{Line: line, Rune: utf8.RuneCountInString(ed.Buffer.Lines[line])})
	if width >= cell {
		return
	}
	text := ed.Buffer.Lines[line] + strings.Repeat(" ", cell-width)
	_, _ = ed.Apply(editor.Edit{
		Start: editor.Pos{Line: line},
		End:   editor.Pos{Line: line, Rune: utf8.RuneCountInString(ed.Buffer.Lines[line])},
		Text:  text,
	})
}

func (v *Vim) finishBlockInsert(ed *editor.Editor) {
	block := v.insert.block
	typed := ""
	if !ed.Cursor.Pos.Before(v.insert.start) {
		if text, err := ed.Buffer.TextInRange(v.insert.start, ed.Cursor.Pos); err == nil {
			typed = text
		}
	}
	typed = strings.ReplaceAll(typed, "\n", " ")
	ed.EndGroup()
	for len(ed.History.Undo) > block.historyLen {
		if _, ok := ed.Undo(); !ok {
			break
		}
	}
	record := changeRecord{action: changeInsert, text: typed, count: 1}
	if v.insert.pendingDot != nil {
		record = *v.insert.pendingDot
		record.text = typed
		record.count = 1
	}
	if block.changeBlock {
		newLines := make([]string, 0, block.last-block.first+1)
		for line := block.first; line <= block.last; line++ {
			original := ed.Buffer.Lines[line]
			newLines = append(newLines, replaceBlockCells(ed, original, block.left, block.right, typed))
		}
		_, _ = ed.Apply(editor.Edit{
			Start: editor.Pos{Line: block.first},
			End:   editor.Pos{Line: block.last, Rune: utf8.RuneCountInString(ed.Buffer.Lines[block.last])},
			Text:  strings.Join(newLines, "\n"),
		})
		ed.SetCursor(ed.Buffer.PosFromCell(block.first, block.left))
	} else if typed != "" {
		newLines := make([]string, 0, block.last-block.first+1)
		for _, line := range block.originals {
			newLines = append(newLines, insertAtCell(ed, line, block.cell, typed))
		}
		_, _ = ed.Apply(editor.Edit{
			Start: editor.Pos{Line: block.first},
			End:   editor.Pos{Line: block.last, Rune: utf8.RuneCountInString(ed.Buffer.Lines[block.last])},
			Text:  strings.Join(newLines, "\n"),
		})
		ed.SetCursor(ed.Buffer.PosFromCell(block.first, block.cell))
	}
	v.Mode = ModeNormal
	v.insert = insertState{}
	v.lastVisual = visualState{}
	v.clampNormal(ed)
	if typed != "" || block.changeBlock {
		v.recordChange(record)
	}
}

// replaceBlockCells removes cells left through right from line and inserts text
// at the left edge. It is used to finish a block change as one edit.
func replaceBlockCells(ed *editor.Editor, line string, left, right int, text string) string {
	temp := editor.New("")
	temp.TabWidth = ed.Buffer.TabWidth
	temp.Lines = []string{line}
	temp.MarkSaved()
	start, end := blockCellRange(tempEditor(ed, temp), 0, left, right)
	startByte := byteOffsetForRune(line, start.Rune)
	endByte := byteOffsetForRune(line, end.Rune)
	prefix := line[:startByte]
	if width := temp.CellColumn(start); width < left {
		prefix += strings.Repeat(" ", left-width)
	}
	return prefix + text + line[endByte:]
}

func tempEditor(ed *editor.Editor, buffer *editor.Buffer) *editor.Editor {
	return editor.NewEditor(buffer)
}

func insertAtCell(ed *editor.Editor, line string, cell int, text string) string {
	temp := editor.New("")
	temp.TabWidth = ed.Buffer.TabWidth
	temp.Lines = []string{line}
	width := temp.CellColumn(editor.Pos{Line: 0, Rune: utf8.RuneCountInString(line)})
	if cell > width {
		line += strings.Repeat(" ", cell-width)
	}
	position := temp.PosFromCell(0, cell)
	offset := byteOffsetForRune(line, position.Rune)
	return line[:offset] + text + line[offset:]
}

func (v *Vim) visualPaste(ed *editor.Editor, before bool) {
	selector := v.selectedRegister()
	value, ok, pending := v.registerTextForPaste(selector)
	if pending {
		v.pendingPaste = &pasteRequest{before: before, count: 1, register: selector, visual: true}
		v.saveVisual(ed)
		return
	}
	if !ok {
		v.message("E353: Nothing in register")
		v.ringBell()
		return
	}
	v.pasteOverSelection(ed, value)
}

func (v *Vim) pasteOverSelection(ed *editor.Editor, value registerValue) {
	if ed.Selection.Kind == editor.SelectionNone {
		return
	}
	first, last := ed.Selection.LineRange()
	block := ed.Selection.Kind == editor.SelectionBlock
	v.saveVisual(ed)
	record := changeRecord{action: changeVisual, caseOp: 'p', visualKind: ed.Selection.Kind}
	if block {
		_, _, left, right := ed.Selection.BlockRange(ed.Buffer)
		record.visualCells = right - left
		record.block = true
		if err := replaceBlock(ed, first, last, left, right, value); err != nil {
			v.ringBell()
		}
	} else {
		start, end := visualCharRange(ed)
		if ed.Selection.Kind == editor.SelectionLine {
			ed.Selection.Kind = editor.SelectionLine
			start = editor.Pos{Line: first}
			end = editor.Pos{Line: last, Rune: utf8.RuneCountInString(ed.Buffer.Lines[last])}
		}
		text := value.Text
		if value.Kind == registerLine && ed.Selection.Kind == editor.SelectionLine {
			text = strings.TrimSuffix(text, "\n")
		}
		if _, err := ed.Apply(editor.Edit{Start: start, End: end, Text: text}); err != nil {
			v.ringBell()
		} else {
			ed.SetCursor(start)
		}
	}
	ed.ClearSelection()
	v.Mode = ModeNormal
	v.recordChange(record)
}

func replaceBlock(ed *editor.Editor, first, last, left, right int, value registerValue) error {
	lines := strings.Split(value.Text, "\n")
	width := right - left + 1
	newLines := make([]string, 0, last-first+1)
	for line := first; line <= last; line++ {
		text := ed.Buffer.Lines[line]
		start, end := blockCellRange(ed, line, left, right)
		startByte := byteOffsetForRune(text, start.Rune)
		endByte := byteOffsetForRune(text, end.Rune)
		prefix := text[:startByte]
		suffix := text[endByte:]
		if prefixWidth := ed.Buffer.CellColumn(start); prefixWidth < left {
			prefix += strings.Repeat(" ", left-prefixWidth)
		}
		insert := ""
		if index := line - first; index < len(lines) {
			insert = lines[index]
		}
		if cellWidth := textCellWidth(ed, insert); cellWidth < width {
			insert += strings.Repeat(" ", width-cellWidth)
		}
		newLines = append(newLines, prefix+insert+suffix)
	}
	_, err := ed.Apply(editor.Edit{
		Start: editor.Pos{Line: first},
		End:   editor.Pos{Line: last, Rune: utf8.RuneCountInString(ed.Buffer.Lines[last])},
		Text:  strings.Join(newLines, "\n"),
	})
	if err == nil {
		ed.SetCursor(ed.Buffer.PosFromCell(first, left))
	}
	return err
}

// blockCellRange returns the cluster-aligned range covered by cells left
// through right on line. A wide cluster straddling an edge is included whole.
func blockCellRange(ed *editor.Editor, line, left, right int) (editor.Pos, editor.Pos) {
	text := ed.Buffer.Lines[line]
	start := editor.Pos{Line: line, Rune: utf8.RuneCountInString(text)}
	end := start
	found := false
	for cluster := range editor.LineClusters(text, ed.Buffer.TabWidth) {
		if cluster.Cells == 0 {
			continue
		}
		if cluster.Cell+cluster.Cells-1 < left || cluster.Cell > right {
			continue
		}
		if !found {
			start = editor.Pos{Line: line, Rune: cluster.Rune}
			found = true
		}
		end = editor.Pos{Line: line, Rune: cluster.Rune + utf8.RuneCountInString(cluster.Text)}
	}
	if !found {
		position := ed.Buffer.PosFromCell(line, left)
		return position, position
	}
	return start, end
}

func (v *Vim) scrollCursor(ed *editor.Editor, r rune) {
	height := max(1, ed.Viewport.Height)
	switch r {
	case 'z':
		ed.Viewport.TopLine = ed.Cursor.Pos.Line - height/2
	case 't':
		ed.Viewport.TopLine = ed.Cursor.Pos.Line
	case 'b':
		ed.Viewport.TopLine = ed.Cursor.Pos.Line - height + 1
	default:
		v.ringBell()
		return
	}
	ed.Viewport.TopLine = min(max(ed.Viewport.TopLine, 0), max(0, len(ed.Buffer.Lines)-height))
}
