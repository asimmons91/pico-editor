package vim

import (
	"unicode"
	"unicode/utf8"

	"github.com/asimmons91/pico-editor/internal/editor"
)

type objectRange struct {
	start    editor.Pos
	end      editor.Pos
	linewise bool
	ok       bool
}

func (v *Vim) applyTextObject(ed *editor.Editor, key rune, inner bool) {
	operator := v.Operator
	if operator == OpNone {
		v.ringBell()
		return
	}
	count := v.takeCount()
	if v.opCount > 1 {
		count *= v.opCount
		v.opCount = 0
	}
	v.Operator = OpNone

	var object objectRange
	switch key {
	case 'w':
		object = wordObject(ed, false, inner, count)
	case 'W':
		object = wordObject(ed, true, inner, count)
	case '(', ')':
		object = bracketObject(ed, '(', ')', inner)
	case '{', '}':
		object = bracketObject(ed, '{', '}', inner)
	case '[', ']':
		object = bracketObject(ed, '[', ']', inner)
	case '"':
		object = quoteObject(ed, '"', inner)
	case '\'':
		object = quoteObject(ed, '\'', inner)
	case 'p':
		object = paragraphObject(ed, !inner)
	default:
		v.ringBell()
		return
	}
	if !object.ok {
		v.ringBell()
		return
	}
	if operator == OpShiftRight || operator == OpShiftLeft {
		first, last := textObjectLines(ed, object, inner)
		v.applyOperatorLines(ed, operator, first, last, key, count, ed.Cursor.Pos)
		return
	}
	if object.linewise {
		v.applyOperatorLines(ed, operator, object.start.Line, object.end.Line, key, count, ed.Cursor.Pos)
		return
	}
	if object.start == object.end {
		v.ringBell()
		return
	}
	v.applyOperatorRange(ed, operator, object.start, object.end, key, count, ed.Cursor.Pos)
}

func textObjectLines(ed *editor.Editor, object objectRange, inner bool) (int, int) {
	if object.linewise {
		return object.start.Line, object.end.Line
	}
	first := object.start.Line
	last := object.end.Line
	if first > last {
		first, last = last, first
	}
	if inner && first < last && object.start.Rune > 0 {
		first++
	}
	if last > first && object.end.Rune == 0 {
		last--
	}
	if first > last {
		first = object.start.Line
		last = first
	}
	return clampLine(first, len(ed.Buffer.Lines)), clampLine(last, len(ed.Buffer.Lines))
}

func wordObject(ed *editor.Editor, big, inner bool, count int) objectRange {
	line := ed.Buffer.Lines[ed.Cursor.Pos.Line]
	if len(line) == 0 {
		return objectRange{start: editor.Pos{Line: ed.Cursor.Pos.Line}, end: editor.Pos{Line: ed.Cursor.Pos.Line}}
	}
	position := ed.Cursor.Pos.Rune
	if position >= utf8.RuneCountInString(line) {
		position = prevClusterRune(line, utf8.RuneCountInString(line))
	}
	class := runeClassAt(line, position)
	if big {
		class = classNonSpace
		if baseRuneAt(line, position) == ' ' || baseRuneAt(line, position) == '\t' {
			class = classSpace
		}
	}

	start := position
	for start > 0 {
		previous := prevClusterRune(line, start)
		previousClass := runeClassAt(line, previous)
		if big {
			previousClass = classNonSpace
			if baseRuneAt(line, previous) == ' ' || baseRuneAt(line, previous) == '\t' {
				previousClass = classSpace
			}
		}
		if previousClass != class {
			break
		}
		start = previous
	}
	end := nextClusterRune(line, position)
	for end < utf8.RuneCountInString(line) {
		nextClass := runeClassAt(line, end)
		if big {
			nextClass = classNonSpace
			if baseRuneAt(line, end) == ' ' || baseRuneAt(line, end) == '\t' {
				nextClass = classSpace
			}
		}
		if nextClass != class {
			break
		}
		end = nextClusterRune(line, end)
	}

	// Count extends the object to further word runs on the same line.
	for i := 1; i < count; i++ {
		cursor := end
		for cursor < utf8.RuneCountInString(line) {
			cursorClass := runeClassAt(line, cursor)
			if big {
				cursorClass = classNonSpace
				if baseRuneAt(line, cursor) == ' ' || baseRuneAt(line, cursor) == '\t' {
					cursorClass = classSpace
				}
			}
			if cursorClass != classSpace {
				break
			}
			cursor = nextClusterRune(line, cursor)
		}
		for cursor < utf8.RuneCountInString(line) {
			cursorClass := runeClassAt(line, cursor)
			if big {
				cursorClass = classNonSpace
				if baseRuneAt(line, cursor) == ' ' || baseRuneAt(line, cursor) == '\t' {
					cursorClass = classSpace
				}
			}
			if cursorClass != class {
				break
			}
			cursor = nextClusterRune(line, cursor)
		}
		end = cursor
	}

	if !inner && class != classSpace {
		// Around a word includes trailing whitespace, or leading whitespace at
		// the end of a line.
		cursor := end
		for cursor < utf8.RuneCountInString(line) {
			r := baseRuneAt(line, cursor)
			if r != ' ' && r != '\t' {
				break
			}
			cursor = nextClusterRune(line, cursor)
		}
		if cursor > end {
			end = cursor
		} else {
			for start > 0 {
				previous := prevClusterRune(line, start)
				r := baseRuneAt(line, previous)
				if r != ' ' && r != '\t' {
					break
				}
				start = previous
			}
		}
	}
	return objectRange{
		start: editor.Pos{Line: ed.Cursor.Pos.Line, Rune: start},
		end:   editor.Pos{Line: ed.Cursor.Pos.Line, Rune: end},
		ok:    true,
	}
}

type charClass uint8

const (
	classSpace charClass = iota
	classWord
	classPunct
	classNonSpace
)

func runeClassAt(line string, runeIndex int) charClass {
	return classOfRune(baseRuneAt(line, runeIndex))
}

func classOfRune(r rune) charClass {
	switch {
	case unicode.IsSpace(r):
		return classSpace
	case unicode.IsLetter(r), unicode.IsDigit(r), r == '_':
		return classWord
	default:
		return classPunct
	}
}

func baseRuneAt(line string, runeIndex int) rune {
	offset := 0
	seen := 0
	for offset < len(line) && seen < runeIndex {
		_, size := utf8.DecodeRuneInString(line[offset:])
		offset += size
		seen++
	}
	if offset >= len(line) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(line[offset:])
	return r
}

type bracketPair struct {
	open  editor.Pos
	close editor.Pos
}

func bracketObject(ed *editor.Editor, open, close rune, inner bool) objectRange {
	pairs := bracketPairs(ed, open, close)
	if len(pairs) == 0 {
		return objectRange{}
	}
	cursor := ed.Cursor.Pos
	var best *bracketPair
	for i := range pairs {
		pair := &pairs[i]
		if !cursor.Before(pair.open) && !pair.close.Before(cursor) {
			if best == nil || pair.open.Compare(best.open) > 0 {
				best = pair
			}
		}
	}
	if best == nil {
		// Vim searches forward on the current line for an opening bracket when
		// the cursor is outside a pair.
		for i := range pairs {
			pair := &pairs[i]
			if pair.open.Line == cursor.Line && !pair.open.Before(cursor) {
				best = pair
				break
			}
		}
	}
	if best == nil {
		return objectRange{}
	}
	if inner {
		start := ed.Buffer.NextCluster(best.open)
		return objectRange{start: start, end: best.close, ok: true}
	}
	return objectRange{start: best.open, end: ed.Buffer.NextCluster(best.close), ok: true}
}

func bracketPairs(ed *editor.Editor, open, close rune) []bracketPair {
	var stack []editor.Pos
	var pairs []bracketPair
	for line := 0; line < len(ed.Buffer.Lines); line++ {
		text := ed.Buffer.Lines[line]
		position := 0
		for position < utf8.RuneCountInString(text) {
			r := baseRuneAt(text, position)
			pos := editor.Pos{Line: line, Rune: position}
			switch r {
			case open:
				stack = append(stack, pos)
			case close:
				if len(stack) > 0 {
					start := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					pairs = append(pairs, bracketPair{open: start, close: pos})
				}
			}
			position = nextClusterRune(text, position)
		}
	}
	return pairs
}

func quoteObject(ed *editor.Editor, quote rune, inner bool) objectRange {
	line := ed.Buffer.Lines[ed.Cursor.Pos.Line]
	var quotes []int
	position := 0
	for position < utf8.RuneCountInString(line) {
		if baseRuneAt(line, position) == quote && !escapedAt(line, position) {
			quotes = append(quotes, position)
		}
		position = nextClusterRune(line, position)
	}
	cursor := ed.Cursor.Pos.Rune
	selected := -1
	for i := 0; i+1 < len(quotes); i += 2 {
		open, close := quotes[i], quotes[i+1]
		if cursor >= open && cursor <= close {
			selected = i
			break
		}
	}
	if selected < 0 {
		for i := 0; i+1 < len(quotes); i += 2 {
			if quotes[i] >= cursor {
				selected = i
				break
			}
		}
	}
	if selected < 0 {
		return objectRange{}
	}
	open, close := quotes[selected], quotes[selected+1]
	if inner {
		return objectRange{
			start: editor.Pos{Line: ed.Cursor.Pos.Line, Rune: nextClusterRune(line, open)},
			end:   editor.Pos{Line: ed.Cursor.Pos.Line, Rune: close},
			ok:    true,
		}
	}
	return objectRange{
		start: editor.Pos{Line: ed.Cursor.Pos.Line, Rune: open},
		end:   editor.Pos{Line: ed.Cursor.Pos.Line, Rune: nextClusterRune(line, close)},
		ok:    true,
	}
}

func escapedAt(line string, runeIndex int) bool {
	backslashes := 0
	position := runeIndex
	for position > 0 {
		previous := prevClusterRune(line, position)
		if baseRuneAt(line, previous) != '\\' {
			break
		}
		backslashes++
		position = previous
	}
	return backslashes%2 == 1
}

func paragraphObject(ed *editor.Editor, around bool) objectRange {
	lines := ed.Buffer.Lines
	line := ed.Cursor.Pos.Line
	first, last := line, line
	if isBlankLine(lines[line]) {
		for first > 0 && isBlankLine(lines[first-1]) {
			first--
		}
		for last+1 < len(lines) && isBlankLine(lines[last+1]) {
			last++
		}
	} else {
		for first > 0 && !isBlankLine(lines[first-1]) {
			first--
		}
		for last+1 < len(lines) && !isBlankLine(lines[last+1]) {
			last++
		}
	}
	if around {
		for last+1 < len(lines) && isBlankLine(lines[last+1]) {
			last++
		}
		if last == line {
			for first > 0 && isBlankLine(lines[first-1]) {
				first--
			}
		}
	}
	return objectRange{
		start:    editor.Pos{Line: first},
		end:      editor.Pos{Line: last},
		linewise: true,
		ok:       true,
	}
}
