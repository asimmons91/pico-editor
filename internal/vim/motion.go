package vim

import (
	"strings"
	"unicode/utf8"

	"github.com/asimmons91/pico-editor/internal/editor"
)

type motionOutcome struct {
	start     editor.Pos
	target    editor.Pos
	linewise  bool
	inclusive bool
	failed    bool
}

// executeMotionKey resolves a motion in normal or visual mode.
func (v *Vim) executeMotionKey(key rune, ed *editor.Editor) {
	switch key {
	case 'f', 'F', 't', 'T':
		spec := findSpec{forward: key == 'f' || key == 't', till: key == 't' || key == 'T'}
		v.pending = pendingState{kind: pendFind, find: spec, count: v.takeCount()}
		return
	case ';', ',':
		if !v.lastFindOK {
			v.ringBell()
			return
		}
		spec := v.lastFind
		if key == ',' {
			spec.forward = !spec.forward
		}
		v.findMotion(ed, spec, v.takeCount())
		v.extendVisual(ed)
		return
	case 'g':
		v.pending = pendingState{kind: pendG}
		return
	}

	countGiven := v.Count > 0
	count := v.takeCount()
	outcome := v.moveForKey(ed, key, count, countGiven, false)
	if outcome.failed {
		v.ringBell()
		return
	}
	v.extendVisual(ed)
}

func (v *Vim) extendVisual(ed *editor.Editor) {
	if v.Mode.Visual() {
		ed.ExtendSelection()
	}
}

// moveForKey performs one motion and returns its operator metadata. The editor
// cursor is moved to the motion target.
func (v *Vim) moveForKey(ed *editor.Editor, key rune, count int, countGiven, forOperator bool) motionOutcome {
	start := ed.Cursor.Pos
	outcome := motionOutcome{start: start, target: start}
	if count < 1 {
		count = 1
	}

	switch key {
	case 'h':
		if v.Mode.Visual() {
			ed.MoveLeft(count)
		} else {
			for i := 0; i < count; i++ {
				if ed.Cursor.Pos.Rune == 0 {
					outcome.failed = i == 0
					break
				}
				ed.MoveLeft(1)
			}
		}
		outcome.inclusive = false
	case 'l':
		if v.Mode.Visual() {
			ed.MoveRight(count)
		} else {
			line := ed.Buffer.Lines[ed.Cursor.Pos.Line]
			for i := 0; i < count; i++ {
				if ed.Cursor.Pos.Rune >= utf8.RuneCountInString(line) {
					outcome.failed = i == 0
					break
				}
				next := ed.Buffer.NextCluster(ed.Cursor.Pos)
				if next.Line != ed.Cursor.Pos.Line {
					break
				}
				ed.SetCursor(next)
			}
		}
		outcome.inclusive = false
	case 'j':
		ed.MoveDown(count)
		outcome.linewise = true
	case 'k':
		ed.MoveUp(count)
		outcome.linewise = true
	case 'w':
		ed.MoveWordForward(count)
		outcome.inclusive = false
	case 'W':
		ed.MoveBigWordForward(count)
		outcome.inclusive = false
	case 'b':
		ed.MoveWordBackward(count)
		outcome.inclusive = false
	case 'B':
		ed.MoveBigWordBackward(count)
		outcome.inclusive = false
	case 'e':
		ed.MoveWordEnd(count)
		outcome.inclusive = true
	case 'E':
		ed.MoveBigWordEnd(count)
		outcome.inclusive = true
	case '0':
		ed.SetCursor(editor.Pos{Line: start.Line})
		outcome.inclusive = false
	case '^':
		setFirstNonBlank(ed, start.Line)
		outcome.inclusive = false
	case '$':
		line := start.Line
		if count > 1 {
			line = min(line+count-1, len(ed.Buffer.Lines)-1)
		}
		setLineEnd(ed, line)
		outcome.inclusive = true
	case 'G':
		line := len(ed.Buffer.Lines) - 1
		if countGiven {
			line = count - 1
		}
		line = clampLine(line, len(ed.Buffer.Lines))
		if forOperator {
			outcome.linewise = true
			outcome.target = editor.Pos{Line: line}
			return outcome
		}
		v.recordJump(ed)
		setFirstNonBlank(ed, line)
		outcome.linewise = true
	case '\n', '+':
		line := min(start.Line+count, len(ed.Buffer.Lines)-1)
		setFirstNonBlank(ed, line)
		outcome.linewise = true
	case '-':
		line := max(start.Line-count, 0)
		setFirstNonBlank(ed, line)
		outcome.linewise = true
	case '%':
		target, ok := matchBracket(ed, start)
		if !ok {
			outcome.failed = true
			return outcome
		}
		v.recordJump(ed)
		ed.SetCursor(target)
		outcome.inclusive = true
	case '{':
		line := paragraphBackward(ed.Buffer.Lines, start.Line, count)
		ed.SetCursor(editor.Pos{Line: line})
		outcome.inclusive = false
	case '}':
		line := paragraphForward(ed.Buffer.Lines, start.Line, count)
		ed.SetCursor(editor.Pos{Line: line})
		outcome.inclusive = false
	case 'H':
		line := min(ed.Viewport.TopLine+count-1, len(ed.Buffer.Lines)-1)
		setFirstNonBlank(ed, line)
		outcome.linewise = true
	case 'M':
		line := ed.Viewport.TopLine + max(0, ed.Viewport.Height/2)
		setFirstNonBlank(ed, clampLine(line, len(ed.Buffer.Lines)))
		outcome.linewise = true
	case 'L':
		line := ed.Viewport.TopLine + ed.Viewport.Height - 1 - (count - 1)
		setFirstNonBlank(ed, clampLine(line, len(ed.Buffer.Lines)))
		outcome.linewise = true
	case ctrlRune('d'):
		ed.MoveDown(max(1, ed.Viewport.Height/2) * count)
		outcome.linewise = true
	case ctrlRune('u'):
		ed.MoveUp(max(1, ed.Viewport.Height/2) * count)
		outcome.linewise = true
	case ctrlRune('f'):
		ed.MoveDown(max(1, ed.Viewport.Height-2) * count)
		outcome.linewise = true
	case ctrlRune('b'):
		ed.MoveUp(max(1, ed.Viewport.Height-2) * count)
		outcome.linewise = true
	case 'n', 'N':
		searchForward := key == 'n'
		if !v.search.forward {
			searchForward = !searchForward
		}
		v.searchMove(ed, searchForward, count, true)
	case '*', '#':
		if v.searchWord(ed, key == '*') {
			v.searchMove(ed, key == '*', count, true)
		} else {
			outcome.failed = true
			return outcome
		}
	default:
		outcome.failed = true
		return outcome
	}

	outcome.target = ed.Cursor.Pos
	return outcome
}

func (v *Vim) doubleGMotion(ed *editor.Editor) {
	start := ed.Cursor.Pos
	count := v.Count
	v.Count = 0
	line := 0
	if count > 0 {
		line = count - 1
	}
	line = clampLine(line, len(ed.Buffer.Lines))
	if v.Operator != OpNone {
		v.applyLineOperatorAt(ed, line)
		return
	}
	v.recordJump(ed)
	setFirstNonBlank(ed, line)
	_ = start
	v.extendVisual(ed)
}

func (v *Vim) wrappedLeft(ed *editor.Editor) {
	if ed.Cursor.Pos.Rune == 0 && ed.Cursor.Pos.Line == 0 {
		v.ringBell()
		return
	}
	ed.SetCursor(ed.Buffer.PrevCluster(ed.Cursor.Pos))
	v.extendVisual(ed)
}

func (v *Vim) wrappedRight(ed *editor.Editor) {
	needsWrap := false
	next := ed.Buffer.NextCluster(ed.Cursor.Pos)
	if next.Line == ed.Cursor.Pos.Line {
		line := ed.Buffer.Lines[ed.Cursor.Pos.Line]
		needsWrap = next.Rune >= utf8.RuneCountInString(line)
	}
	if needsWrap {
		if ed.Cursor.Pos.Line+1 >= len(ed.Buffer.Lines) {
			v.ringBell()
			return
		}
		ed.SetCursor(editor.Pos{Line: ed.Cursor.Pos.Line + 1})
	} else {
		ed.SetCursor(next)
	}
	v.extendVisual(ed)
}

func (v *Vim) findMotion(ed *editor.Editor, spec findSpec, count int) bool {
	if count < 1 {
		count = 1
	}
	lineNumber := ed.Cursor.Pos.Line
	line := ed.Buffer.Lines[lineNumber]
	scan := ed.Cursor.Pos.Rune
	if spec.hasRaw && spec.line == lineNumber {
		scan = spec.raw
	}
	result := ed.Cursor.Pos.Rune
	raw := scan
	foundAny := false
	for i := 0; i < count; i++ {
		found := -1
		if spec.forward {
			scan = nextClusterRune(line, scan)
			for runeIndex := scan; runeIndex < utf8.RuneCountInString(line); runeIndex = nextClusterRune(line, runeIndex) {
				if clusterMatches(line, runeIndex, spec.char) {
					found = runeIndex
					break
				}
			}
		} else {
			scan = prevClusterRune(line, scan)
			for runeIndex := scan; runeIndex >= 0; runeIndex = prevClusterRune(line, runeIndex) {
				if clusterMatches(line, runeIndex, spec.char) {
					found = runeIndex
					break
				}
				if runeIndex == 0 {
					break
				}
			}
		}
		if found < 0 {
			break
		}
		foundAny = true
		raw = found
		result = found
		if spec.till {
			if spec.forward {
				result = prevClusterRune(line, found)
			} else {
				result = nextClusterRune(line, found)
			}
		}
		scan = found
	}
	if !foundAny {
		v.ringBell()
		return false
	}
	spec.line = lineNumber
	spec.raw = raw
	spec.hasRaw = true
	v.lastFind = spec
	v.lastFindOK = true
	ed.SetCursor(editor.Pos{Line: lineNumber, Rune: result})
	v.extendVisual(ed)
	return true
}

func setFirstNonBlank(ed *editor.Editor, line int) {
	if line < 0 {
		line = 0
	}
	if line >= len(ed.Buffer.Lines) {
		line = len(ed.Buffer.Lines) - 1
	}
	ed.SetCursor(editor.Pos{Line: line, Rune: firstNonBlankRune(ed.Buffer.Lines[line])})
}

func setLineEnd(ed *editor.Editor, line int) {
	line = clampLine(line, len(ed.Buffer.Lines))
	length := utf8.RuneCountInString(ed.Buffer.Lines[line])
	if length == 0 {
		ed.SetCursor(editor.Pos{Line: line})
		return
	}
	ed.SetCursor(editor.Pos{Line: line, Rune: prevClusterRune(ed.Buffer.Lines[line], length)})
}

func firstNonBlankRune(line string) int {
	index := 0
	for _, r := range line {
		if r != ' ' && r != '\t' {
			return index
		}
		index++
	}
	return index
}

func clampLine(line, count int) int {
	if count <= 0 {
		return 0
	}
	return min(max(line, 0), count-1)
}

func nextClusterRune(line string, runeIndex int) int {
	length := utf8.RuneCountInString(line)
	if runeIndex >= length {
		return length
	}
	index := 0
	for index < len(line) {
		_, size := utf8.DecodeRuneInString(line[index:])
		next := index + size
		if next < len(line) && strings.HasPrefix(line[next:], "\ufe0f") {
			next += len("\ufe0f")
		}
		if runeIndex == 0 {
			return utf8.RuneCountInString(line[:next])
		}
		runeIndex--
		index = next
	}
	return length
}

func prevClusterRune(line string, runeIndex int) int {
	if runeIndex <= 0 {
		return 0
	}
	target := runeIndex - 1
	index := 0
	runes := 0
	last := 0
	for index < len(line) {
		last = runes
		_, size := utf8.DecodeRuneInString(line[index:])
		next := index + size
		if next < len(line) && strings.HasPrefix(line[next:], "\ufe0f") {
			next += len("\ufe0f")
		}
		runes += utf8.RuneCountInString(line[index:next])
		index = next
		if runes > target {
			return last
		}
	}
	return last
}

func clusterMatches(line string, runeIndex int, target rune) bool {
	if runeIndex < 0 || runeIndex >= utf8.RuneCountInString(line) {
		return false
	}
	offset := 0
	seen := 0
	for offset < len(line) && seen < runeIndex {
		_, size := utf8.DecodeRuneInString(line[offset:])
		offset += size
		seen++
	}
	if offset >= len(line) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(line[offset:])
	return r == target
}

func paragraphBackward(lines []string, line, count int) int {
	for i := 0; i < count; i++ {
		if line == 0 {
			break
		}
		line--
		for line > 0 && !isBlankLine(lines[line]) {
			line--
		}
	}
	return line
}

func paragraphForward(lines []string, line, count int) int {
	for i := 0; i < count; i++ {
		if line >= len(lines)-1 {
			line = len(lines) - 1
			break
		}
		line++
		for line < len(lines)-1 && !isBlankLine(lines[line]) {
			line++
		}
	}
	return line
}

func isBlankLine(line string) bool {
	return strings.TrimSpace(line) == ""
}
