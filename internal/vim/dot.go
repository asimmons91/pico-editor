package vim

import (
	"unicode/utf8"

	"github.com/asimmons91/pico-editor/internal/editor"
)

type changeKind uint8

const (
	changeNone changeKind = iota
	changeInsert
	changeOperator
	changeReplace
	changeJoin
	changeToggle
	changePaste
	changeVisual
)

type changeRecord struct {
	action   changeKind
	operator Operator
	motion   rune
	count    int
	text     string
	register byte
	linewise bool
	backward bool
	block    bool

	visualKind  editor.SelectionKind
	visualLines int
	visualCells int
	caseOp      rune
}

func (v *Vim) recordChange(record changeRecord) {
	if v.replaying {
		return
	}
	v.lastChange = record
	v.changeSet = true
}

func (v *Vim) repeatChange(ed *editor.Editor) {
	if !v.changeSet || v.lastChange.action == changeNone {
		v.ringBell()
		return
	}
	count := v.takeCount()
	record := v.lastChange
	v.replaying = true
	for i := 0; i < count; i++ {
		if !v.replay(ed, record) {
			v.ringBell()
			break
		}
	}
	v.replaying = false
}

func (v *Vim) replay(ed *editor.Editor, record changeRecord) bool {
	switch record.action {
	case changeInsert:
		if record.text == "" {
			return false
		}
		ed.BeginGroup()
		repeats := max(1, record.count)
		for i := 0; i < repeats; i++ {
			if _, err := ed.Insert(record.text); err != nil {
				ed.EndGroup()
				return false
			}
		}
		ed.EndGroup()
		v.clampNormal(ed)
		return true
	case changeOperator:
		return v.replayOperator(ed, record)
	case changeReplace:
		runes := []rune(record.text)
		if len(runes) == 0 {
			return false
		}
		v.replaceClusters(ed, runes[0], max(1, record.count))
		return true
	case changeJoin:
		v.Count = max(2, record.count)
		v.joinLines(ed)
		return true
	case changeToggle:
		v.Count = max(1, record.count)
		v.toggleCase(ed)
		return true
	case changePaste:
		v.Register = record.register
		v.pasteValue(ed, v.mustRegister(record.register), &pasteRequest{
			before:   record.backward,
			count:    max(1, record.count),
			register: record.register,
		})
		return true
	case changeVisual:
		return v.replayVisual(ed, record)
	default:
		return false
	}
}

func (v *Vim) replayOperator(ed *editor.Editor, record changeRecord) bool {
	count := max(1, record.count)
	if record.linewise {
		v.Operator = record.operator
		v.opCount = 1
		v.Count = count
		switch record.motion {
		case 0:
			v.applyLineOperator(ed)
		case 'G':
			v.applyLineOperatorAt(ed, min(ed.Cursor.Pos.Line+count-1, len(ed.Buffer.Lines)-1))
		default:
			v.applyOperatorMotion(ed, record.motion)
		}
		if record.operator == OpChange {
			if record.text != "" {
				_, _ = ed.Insert(record.text)
			}
			v.finishInsert(ed)
		}
		return true
	}

	if record.motion == 'l' && record.backward {
		v.Count = count
		v.deleteClusters(ed, false, record.operator == OpChange)
		if record.operator == OpChange {
			if record.text != "" {
				_, _ = ed.Insert(record.text)
			}
			v.finishInsert(ed)
		}
		return true
	}
	v.Operator = record.operator
	v.opCount = 1
	v.Count = count
	v.applyOperatorMotion(ed, record.motion)
	if record.operator == OpChange {
		if record.text != "" {
			_, _ = ed.Insert(record.text)
		}
		v.finishInsert(ed)
	}
	return true
}

func (v *Vim) replayVisual(ed *editor.Editor, record changeRecord) bool {
	anchor := ed.Cursor.Pos
	v.Mode = ModeVisualChar
	switch record.visualKind {
	case editor.SelectionLine:
		v.Mode = ModeVisualLine
	case editor.SelectionBlock:
		v.Mode = ModeVisualBlock
	}
	kind := editor.SelectionChar
	switch record.visualKind {
	case editor.SelectionLine:
		kind = editor.SelectionLine
	case editor.SelectionBlock:
		kind = editor.SelectionBlock
	}
	ed.Selection = editor.Selection{Anchor: anchor, Cursor: anchor, Kind: kind}
	target := anchor
	switch record.visualKind {
	case editor.SelectionLine:
		line := min(anchor.Line+max(0, record.visualLines-1), len(ed.Buffer.Lines)-1)
		target = editor.Pos{Line: line, Rune: utf8.RuneCountInString(ed.Buffer.Lines[line])}
		if target.Rune > 0 {
			target.Rune = prevClusterRune(ed.Buffer.Lines[line], target.Rune)
		}
	case editor.SelectionBlock:
		target = ed.Buffer.PosFromCell(anchor.Line, ed.Buffer.CellColumn(anchor)+record.visualCells)
	default:
		for i := 0; i < record.visualCells; i++ {
			next := ed.Buffer.NextCluster(target)
			if next == target {
				break
			}
			target = next
		}
	}
	ed.SetCursor(target)
	ed.Selection.Cursor = target
	v.visual = visualState{anchor: anchor, cursor: target, kind: kind}
	v.executeVisualOperator(record.caseOp, ed)
	if record.operator == OpChange && v.Mode == ModeInsert {
		if record.text != "" {
			_, _ = ed.Insert(record.text)
		}
		v.finishInsert(ed)
	}
	return true
}

func (v *Vim) mustRegister(selector byte) registerValue {
	if value, ok := v.registers.get(selector); ok {
		return value
	}
	return registerValue{}
}
