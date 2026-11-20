package vim

import (
	"unicode/utf8"

	"github.com/asimmons91/pico-editor/internal/editor"
)

const shiftWidth = 2

// applyOperatorMotion resolves a motion for the pending operator and applies it.
func (v *Vim) applyOperatorMotion(ed *editor.Editor, motion rune) {
	operator := v.Operator
	opCount := v.opCount
	v.opCount = 0
	countGiven := v.Count > 0 || opCount > 1
	count := v.takeCount()
	total := count
	if opCount > 1 {
		total *= opCount
	}
	start := ed.Cursor.Pos
	if operator == OpChange && len(ed.Buffer.Lines[start.Line]) == 0 {
		switch motion {
		case 'w', 'W', 'e', 'E', '$', 'l', '0', '^':
			record := changeRecord{action: changeOperator, operator: OpChange, motion: motion, count: total}
			v.Operator = OpNone
			v.enterInsertWithRecord(ed, &record)
			return
		}
	}
	var outcome motionOutcome
	if operator == OpChange && (motion == 'w' || motion == 'W') {
		ed.MoveWordEndOperator(total, motion == 'W')
		outcome = motionOutcome{start: start, target: ed.Cursor.Pos, inclusive: true}
	} else {
		outcome = v.moveForKey(ed, motion, total, countGiven, true)
	}
	if outcome.failed {
		v.Operator = OpNone
		v.ringBell()
		return
	}
	if motion == 'w' || motion == 'W' {
		// Vim does not let dw/cw delete the newline when the cursor starts on
		// the last word of a line.
		if outcome.target.Line > start.Line && hasNonBlankFrom(ed.Buffer.Lines[start.Line], start.Rune) {
			outcome.target = editor.Pos{
				Line: start.Line,
				Rune: utf8.RuneCountInString(ed.Buffer.Lines[start.Line]),
			}
		}
	}

	v.Operator = OpNone
	if operator == OpShiftRight || operator == OpShiftLeft {
		first := min(start.Line, outcome.target.Line)
		last := max(start.Line, outcome.target.Line)
		v.applyOperatorLines(ed, operator, first, last, motion, total, start)
		return
	}
	if outcome.linewise {
		first := min(start.Line, outcome.target.Line)
		last := max(start.Line, outcome.target.Line)
		v.applyOperatorLines(ed, operator, first, last, motion, total, start)
		return
	}
	rangeStart, rangeEnd := operatorCharRange(ed, start, outcome.target, outcome.inclusive)
	if rangeStart == rangeEnd {
		if operator == OpChange && len(ed.Buffer.Lines[start.Line]) == 0 {
			record := changeRecord{action: changeOperator, operator: OpChange, motion: motion, count: total}
			v.enterInsertWithRecord(ed, &record)
			return
		}
		v.ringBell()
		return
	}
	v.applyOperatorRange(ed, operator, rangeStart, rangeEnd, motion, total, start)
}

func (v *Vim) applyOperatorFind(ed *editor.Editor, spec findSpec, count int) {
	operator := v.Operator
	start := ed.Cursor.Pos
	opCount := v.opCount
	v.opCount = 0
	if !v.findMotion(ed, spec, count) {
		v.Operator = OpNone
		return
	}
	target := ed.Cursor.Pos
	v.Operator = OpNone
	// f/F/t/T are characterwise. Forward finds are inclusive; a till motion
	// stops before the target and is already exclusive.
	rangeStart, rangeEnd := operatorCharRange(ed, start, target, !spec.till)
	if rangeStart == rangeEnd {
		v.ringBell()
		return
	}
	total := count
	if opCount > 1 {
		total *= opCount
	}
	v.applyOperatorRange(ed, operator, rangeStart, rangeEnd, 'f', total, start)
	if v.replaying {
		return
	}
	switch {
	case operator == OpDelete && v.changeSet:
		v.lastChange.motion = 0
		v.lastChange.find = spec
		v.lastChange.hasFind = true
	case operator == OpChange && v.Mode == ModeInsert && v.insert.pendingDot != nil:
		v.insert.pendingDot.motion = 0
		v.insert.pendingDot.find = spec
		v.insert.pendingDot.hasFind = true
	}
}

// applyLineOperator handles dd/cc/yy/>>/<< and their counted forms.
func (v *Vim) applyLineOperator(ed *editor.Editor) {
	operator := v.Operator
	count := v.takeCount()
	if v.opCount > 1 {
		count *= v.opCount
		v.opCount = 0
	}
	v.Operator = OpNone
	first := ed.Cursor.Pos.Line
	last := min(first+count-1, ed.WindowLast())
	start := ed.Cursor.Pos
	v.applyOperatorLines(ed, operator, first, last, 0, count, start)
}

// applyLineOperatorAt applies a linewise operator when the motion selected a
// target line, as with dG and dgg.
func (v *Vim) applyLineOperatorAt(ed *editor.Editor, targetLine int) {
	operator := v.Operator
	v.Operator = OpNone
	start := ed.Cursor.Pos
	first := min(start.Line, targetLine)
	last := max(start.Line, targetLine)
	v.applyOperatorLines(ed, operator, first, last, 'G', last-first+1, start)
}

func (v *Vim) applyOperatorLines(ed *editor.Editor, operator Operator, first, last int, motion rune, count int, start editor.Pos) {
	first, last = min(first, last), max(first, last)
	first = clampVimLine(ed, first)
	last = clampVimLine(ed, last)
	selector := v.selectedRegister()

	switch operator {
	case OpYank:
		text, err := ed.LineRangeText(first, last)
		if err != nil {
			v.ringBell()
			return
		}
		v.recordRegister(text, registerLine, selector, true)
		ed.SetCursor(start)
	case OpDelete:
		text, err := ed.LineRangeText(first, last)
		if err != nil {
			v.ringBell()
			return
		}
		v.recordRegister(text, registerLine, selector, false)
		if _, err := ed.DeleteLines(first, last); err != nil {
			v.ringBell()
			return
		}
		targetLine := clampLine(first, len(ed.Buffer.Lines))
		ed.SetCursor(editor.Pos{Line: targetLine, Rune: firstNonBlankRune(ed.Buffer.Lines[targetLine])})
		v.recordChange(changeRecord{
			action:   changeOperator,
			operator: OpDelete,
			motion:   motion,
			count:    count,
			linewise: true,
		})
	case OpChange:
		text, err := ed.LineRangeText(first, last)
		if err != nil {
			v.ringBell()
			return
		}
		v.recordRegister(text, registerLine, selector, false)
		ed.BeginGroup()
		if err := v.changeLines(ed, first, last); err != nil {
			ed.EndGroup()
			v.ringBell()
			return
		}
		record := changeRecord{
			action:   changeOperator,
			operator: OpChange,
			motion:   motion,
			count:    count,
			linewise: true,
		}
		v.enterInsertWithRecord(ed, &record)
	case OpShiftRight, OpShiftLeft:
		delta := shiftWidth
		if operator == OpShiftLeft {
			delta = -shiftWidth
		}
		if _, err := ed.IndentLines(first, last, delta); err != nil {
			v.ringBell()
			return
		}
		ed.SetCursor(editor.Pos{Line: first, Rune: firstNonBlankRune(ed.Buffer.Lines[first])})
		v.recordChange(changeRecord{
			action:   changeOperator,
			operator: operator,
			motion:   motion,
			count:    count,
			linewise: true,
		})
	}
}

func (v *Vim) applyOperatorRange(ed *editor.Editor, operator Operator, start, end editor.Pos, motion rune, count int, cursorBefore editor.Pos) {
	if operator == OpShiftRight || operator == OpShiftLeft {
		first := min(start.Line, end.Line)
		last := max(start.Line, end.Line)
		v.applyOperatorLines(ed, operator, first, last, motion, count, cursorBefore)
		return
	}
	selector := v.selectedRegister()
	switch operator {
	case OpYank:
		text, err := ed.Buffer.TextInRange(start, end)
		if err != nil {
			v.ringBell()
			return
		}
		v.recordRegister(text, registerChar, selector, true)
		ed.SetCursor(cursorBefore)
	case OpDelete:
		text, err := ed.Buffer.TextInRange(start, end)
		if err != nil {
			v.ringBell()
			return
		}
		v.recordRegister(text, registerChar, selector, false)
		if _, err := ed.DeleteRange(start, end); err != nil {
			v.ringBell()
			return
		}
		ed.SetCursor(start)
		v.recordChange(changeRecord{
			action:   changeOperator,
			operator: OpDelete,
			motion:   motion,
			count:    count,
		})
	case OpChange:
		text, err := ed.Buffer.TextInRange(start, end)
		if err != nil {
			v.ringBell()
			return
		}
		v.recordRegister(text, registerChar, selector, false)
		ed.BeginGroup()
		if _, err := ed.DeleteRange(start, end); err != nil {
			ed.EndGroup()
			v.ringBell()
			return
		}
		record := changeRecord{
			action:   changeOperator,
			operator: OpChange,
			motion:   motion,
			count:    count,
		}
		v.enterInsertWithRecord(ed, &record)
	}
}

func (v *Vim) changeLines(ed *editor.Editor, first, last int) error {
	line := ed.Buffer.Lines[first]
	indent := firstNonBlankRune(line)
	end := editor.Pos{Line: last, Rune: utf8.RuneCountInString(ed.Buffer.Lines[last])}
	_, err := ed.Apply(editor.Edit{
		Start: editor.Pos{Line: first, Rune: indent},
		End:   end,
		Text:  "",
	})
	if err != nil {
		return err
	}
	ed.SetCursor(editor.Pos{Line: first, Rune: indent})
	return nil
}

// operatorCharRange converts a motion from start to target into a half-open
// edit range. Backward motions exclude the unchanged cursor cell.
func operatorCharRange(ed *editor.Editor, start, target editor.Pos, inclusive bool) (editor.Pos, editor.Pos) {
	if target.Before(start) {
		return target, start
	}
	end := target
	if inclusive {
		line := ed.Buffer.Lines[target.Line]
		if target.Rune < utf8.RuneCountInString(line) {
			end = ed.Buffer.NextCluster(target)
		}
	}
	if end == start {
		return start, start
	}
	return start, end
}

func hasNonBlankFrom(line string, runeIndex int) bool {
	offset := 0
	seen := 0
	for offset < len(line) && seen < runeIndex {
		_, size := utf8.DecodeRuneInString(line[offset:])
		offset += size
		seen++
	}
	for offset < len(line) {
		r, size := utf8.DecodeRuneInString(line[offset:])
		if r != ' ' && r != '\t' {
			return true
		}
		offset += size
	}
	return false
}
