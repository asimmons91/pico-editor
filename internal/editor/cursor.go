package editor

import (
	"unicode"
	"unicode/utf8"
)

// Cursor is a cluster-boundary position plus the display column vertical
// motions try to preserve.
type Cursor struct {
	Pos         Pos
	DesiredCell int
}

// Motion is one cursor primitive. M3 maps keys and counts onto these.
type Motion uint8

const (
	MotionLeft Motion = iota
	MotionRight
	MotionUp
	MotionDown
	MotionWordForward
	MotionWordBackward
	MotionBigWordForward
	MotionBigWordBackward
	MotionLineStart
	MotionFirstNonBlank
	MotionLineEnd
	MotionBufferStart
	MotionBufferEnd
	MotionPageUp
	MotionPageDown
)

// Move applies a motion count times.
func (e *Editor) Move(motion Motion, count int) {
	if count <= 0 {
		return
	}
	e.BreakGroup()
	switch motion {
	case MotionLeft:
		e.moveLeft(count)
	case MotionRight:
		e.moveRight(count)
	case MotionUp:
		e.moveVertical(-count)
	case MotionDown:
		e.moveVertical(count)
	case MotionWordForward:
		e.moveWordForward(count, false)
	case MotionWordBackward:
		e.moveWordBackward(count, false)
	case MotionBigWordForward:
		e.moveWordForward(count, true)
	case MotionBigWordBackward:
		e.moveWordBackward(count, true)
	case MotionLineStart:
		e.setCursor(Pos{Line: e.Cursor.Pos.Line}, true)
	case MotionFirstNonBlank:
		e.setCursor(Pos{Line: e.Cursor.Pos.Line, Rune: firstNonBlank(e.Buffer.Lines[e.Cursor.Pos.Line])}, true)
	case MotionLineEnd:
		e.setCursor(Pos{Line: e.Cursor.Pos.Line, Rune: utf8.RuneCountInString(e.Buffer.Lines[e.Cursor.Pos.Line])}, true)
	case MotionBufferStart:
		e.setCursor(Pos{}, true)
	case MotionBufferEnd:
		last := len(e.Buffer.Lines) - 1
		e.setCursor(Pos{Line: last, Rune: utf8.RuneCountInString(e.Buffer.Lines[last])}, true)
	case MotionPageUp:
		e.movePage(-count)
	case MotionPageDown:
		e.movePage(count)
	}
	e.EnsureVisible()
}

// MoveLeft moves left, joining the previous line at line start.
func (e *Editor) MoveLeft(count int) { e.Move(MotionLeft, count) }

// MoveRight moves right, joining the next line at line end.
func (e *Editor) MoveRight(count int) { e.Move(MotionRight, count) }

// MoveUp moves up, preserving the desired display column.
func (e *Editor) MoveUp(count int) { e.Move(MotionUp, count) }

// MoveDown moves down, preserving the desired display column.
func (e *Editor) MoveDown(count int) { e.Move(MotionDown, count) }

// MoveWordForward moves to the next word start.
func (e *Editor) MoveWordForward(count int) { e.Move(MotionWordForward, count) }

// MoveWordBackward moves to the previous word start.
func (e *Editor) MoveWordBackward(count int) { e.Move(MotionWordBackward, count) }

// MoveBigWordForward moves to the next whitespace-delimited word start.
func (e *Editor) MoveBigWordForward(count int) { e.Move(MotionBigWordForward, count) }

// MoveBigWordBackward moves to the previous whitespace-delimited word start.
func (e *Editor) MoveBigWordBackward(count int) { e.Move(MotionBigWordBackward, count) }

// MoveLineStart moves to column zero.
func (e *Editor) MoveLineStart() { e.Move(MotionLineStart, 1) }

// MoveFirstNonBlank moves to the first non-blank cluster.
func (e *Editor) MoveFirstNonBlank() { e.Move(MotionFirstNonBlank, 1) }

// MoveLineEnd moves past the last cluster of the current line.
func (e *Editor) MoveLineEnd() { e.Move(MotionLineEnd, 1) }

// MoveBufferStart moves to the first line and column.
func (e *Editor) MoveBufferStart() { e.Move(MotionBufferStart, 1) }

// MoveBufferEnd moves past the last cluster of the last line.
func (e *Editor) MoveBufferEnd() { e.Move(MotionBufferEnd, 1) }

// MovePageUp moves up by one viewport height.
func (e *Editor) MovePageUp() { e.Move(MotionPageUp, 1) }

// MovePageDown moves down by one viewport height.
func (e *Editor) MovePageDown() { e.Move(MotionPageDown, 1) }

// MoveToCell moves to a display cell on the current line. The target snaps to a
// cluster start, so it never lands inside a wide glyph.
func (e *Editor) MoveToCell(cell int) {
	e.MoveTo(e.Cursor.Pos.Line, cell)
}

// MoveTo moves to a display cell on line.
func (e *Editor) MoveTo(line, cell int) {
	e.BreakGroup()
	if line < 0 {
		line = 0
	}
	if line >= len(e.Buffer.Lines) {
		line = len(e.Buffer.Lines) - 1
	}
	if cell < 0 {
		cell = 0
	}
	e.setCursor(e.Buffer.PosFromCell(line, cell), false)
	e.Cursor.DesiredCell = cell
	e.EnsureVisible()
}

func (e *Editor) moveLeft(count int) {
	position := e.Cursor.Pos
	for i := 0; i < count; i++ {
		if position.Rune > 0 {
			line := e.Buffer.Lines[position.Line]
			position.Rune = prevClusterBoundary(line, position.Rune)
			continue
		}
		if position.Line == 0 {
			break
		}
		position.Line--
		position.Rune = utf8.RuneCountInString(e.Buffer.Lines[position.Line])
	}
	e.setCursor(position, true)
}

func (e *Editor) moveRight(count int) {
	position := e.Cursor.Pos
	for i := 0; i < count; i++ {
		line := e.Buffer.Lines[position.Line]
		if position.Rune < utf8.RuneCountInString(line) {
			position.Rune = nextClusterBoundary(line, position.Rune)
			continue
		}
		if position.Line+1 >= len(e.Buffer.Lines) {
			break
		}
		position.Line++
		position.Rune = 0
	}
	e.setCursor(position, true)
}

func (e *Editor) moveVertical(delta int) {
	line := e.Cursor.Pos.Line + delta
	if line < 0 {
		line = 0
	}
	if line >= len(e.Buffer.Lines) {
		line = len(e.Buffer.Lines) - 1
	}
	position := e.Buffer.PosFromCell(line, e.Cursor.DesiredCell)
	e.setCursor(position, false)
}

func (e *Editor) movePage(direction int) {
	height := e.Viewport.Height
	if height < 1 {
		height = 1
	}
	e.moveVertical(direction * height)
}

type charClass uint8

const (
	classSpace charClass = iota
	classWord
	classPunct
)

func classOf(r rune) charClass {
	switch {
	case unicode.IsSpace(r):
		return classSpace
	case unicode.IsLetter(r), unicode.IsDigit(r), r == '_':
		return classWord
	default:
		return classPunct
	}
}

func (e *Editor) classAt(position Pos) charClass {
	line := e.Buffer.Lines[position.Line]
	if position.Rune >= utf8.RuneCountInString(line) {
		return classSpace
	}
	index := byteIndexForRune(line, position.Rune)
	r, _ := utf8.DecodeRuneInString(line[index:])
	return classOf(r)
}

func (e *Editor) atBufferStart(position Pos) bool {
	return position.Line == 0 && position.Rune == 0
}

func (e *Editor) atBufferEnd(position Pos) bool {
	last := len(e.Buffer.Lines) - 1
	return position.Line == last && position.Rune >= utf8.RuneCountInString(e.Buffer.Lines[last])
}

func (e *Editor) moveWordForward(count int, big bool) {
	position := e.Cursor.Pos
	for i := 0; i < count; i++ {
		if e.atBufferEnd(position) {
			break
		}
		if big {
			if e.classAt(position) != classSpace {
				for !e.atBufferEnd(position) && e.classAt(position) != classSpace {
					position = e.Buffer.NextCluster(position)
				}
			}
		} else {
			class := e.classAt(position)
			if class != classSpace {
				for !e.atBufferEnd(position) && e.classAt(position) == class {
					position = e.Buffer.NextCluster(position)
				}
			}
		}
		for !e.atBufferEnd(position) && e.classAt(position) == classSpace {
			position = e.Buffer.NextCluster(position)
		}
	}
	e.setCursor(position, true)
}

func (e *Editor) moveWordBackward(count int, big bool) {
	position := e.Cursor.Pos
	for i := 0; i < count; i++ {
		if e.atBufferStart(position) {
			break
		}
		previous := e.Buffer.PrevCluster(position)
		for !e.atBufferStart(previous) && e.classAt(previous) == classSpace {
			previous = e.Buffer.PrevCluster(previous)
		}
		if e.classAt(previous) != classSpace {
			if big {
				for !e.atBufferStart(previous) {
					earlier := e.Buffer.PrevCluster(previous)
					if e.classAt(earlier) == classSpace {
						break
					}
					previous = earlier
				}
			} else {
				class := e.classAt(previous)
				for !e.atBufferStart(previous) {
					earlier := e.Buffer.PrevCluster(previous)
					if e.classAt(earlier) != class {
						break
					}
					previous = earlier
				}
			}
		}
		position = previous
	}
	e.setCursor(position, true)
}
