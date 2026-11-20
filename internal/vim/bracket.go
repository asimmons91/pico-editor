package vim

import (
	"unicode/utf8"

	"github.com/asimmons91/pico-editor/internal/editor"
)

type bracketToken struct {
	char rune
	pos  editor.Pos
}

type matchedBracket struct {
	open  bracketToken
	close bracketToken
}

// matchBracket implements % over Lua source, ignoring brackets in strings and
// comments.
func matchBracket(ed *editor.Editor, from editor.Pos) (editor.Pos, bool) {
	tokens := scanBrackets(ed)
	within := tokens[:0]
	for _, token := range tokens {
		if ed.HasLine(token.pos.Line) {
			within = append(within, token)
		}
	}
	tokens = within
	if len(tokens) == 0 {
		return editor.Pos{}, false
	}
	target := -1
	for i, token := range tokens {
		if token.pos.Line == from.Line && token.pos.Rune >= from.Rune {
			target = i
			break
		}
	}
	if target < 0 {
		for i, token := range tokens {
			if token.pos.Line > from.Line {
				target = i
				break
			}
		}
	}
	if target < 0 {
		return editor.Pos{}, false
	}

	token := tokens[target]
	pairs := make([]matchedBracket, 0, len(tokens)/2)
	stack := make([]bracketToken, 0, 16)
	for _, candidate := range tokens {
		if isOpening(candidate.char) {
			stack = append(stack, candidate)
			continue
		}
		if len(stack) == 0 {
			continue
		}
		open := stack[len(stack)-1]
		if matches(open.char, candidate.char) {
			stack = stack[:len(stack)-1]
			pairs = append(pairs, matchedBracket{open: open, close: candidate})
		}
	}
	for _, pair := range pairs {
		if pair.open.pos == token.pos {
			return pair.close.pos, true
		}
		if pair.close.pos == token.pos {
			return pair.open.pos, true
		}
	}
	return editor.Pos{}, false
}

func scanBrackets(ed *editor.Editor) []bracketToken {
	text := ed.Buffer.Text()
	tokens := make([]bracketToken, 0, 64)
	line := 0
	column := 0
	lineComment := false
	var quote rune
	longLevel := -1

	for index := 0; index < len(text); {
		r, size := utf8.DecodeRuneInString(text[index:])
		nextIndex := index + size
		if r == '\n' {
			lineComment = false
			line++
			column = 0
			index = nextIndex
			continue
		}
		if longLevel >= 0 {
			if r == ']' && closesLong(text[index:], longLevel) {
				index += 1 + longLevel + 1
				column += 1 + longLevel + 1
				longLevel = -1
				continue
			}
			index = nextIndex
			column++
			continue
		}
		if lineComment {
			index = nextIndex
			column++
			continue
		}
		if quote != 0 {
			if r == '\\' && nextIndex < len(text) {
				_, nextSize := utf8.DecodeRuneInString(text[nextIndex:])
				index = nextIndex + nextSize
				column += 2
				continue
			}
			if r == quote {
				quote = 0
			}
			index = nextIndex
			column++
			continue
		}

		switch r {
		case '-':
			if nextIndex < len(text) && text[nextIndex] == '-' {
				rest := text[nextIndex+1:]
				if level, ok := opensLong(rest); ok {
					longLevel = level
					index = nextIndex + 1 + 1 + level + 1
					column += 2 + 1 + level + 1
					continue
				}
				lineComment = true
				index = nextIndex + 1
				column += 2
				continue
			}
		case '\'', '"':
			quote = r
		case '(':
			tokens = append(tokens, bracketToken{char: r, pos: editor.Pos{Line: line, Rune: column}})
		case '[':
			if level, ok := opensLong(text[index:]); ok {
				longLevel = level
				index += 1 + level + 1
				column += 1 + level + 1
				continue
			}
			tokens = append(tokens, bracketToken{char: r, pos: editor.Pos{Line: line, Rune: column}})
		case '{', ')', ']', '}':
			tokens = append(tokens, bracketToken{char: r, pos: editor.Pos{Line: line, Rune: column}})
		}
		index = nextIndex
		column++
	}
	return tokens
}

func opensLong(text string) (int, bool) {
	if len(text) == 0 || text[0] != '[' {
		return 0, false
	}
	level := 0
	index := 1
	for index < len(text) && text[index] == '=' {
		level++
		index++
	}
	if index < len(text) && text[index] == '[' {
		return level, true
	}
	return 0, false
}

func closesLong(text string, level int) bool {
	if len(text) == 0 || text[0] != ']' {
		return false
	}
	index := 1
	for i := 0; i < level; i++ {
		if index >= len(text) || text[index] != '=' {
			return false
		}
		index++
	}
	return index < len(text) && text[index] == ']'
}

func isOpening(r rune) bool {
	return r == '(' || r == '[' || r == '{'
}

func matches(open, close rune) bool {
	return (open == '(' && close == ')') ||
		(open == '[' && close == ']') ||
		(open == '{' && close == '}')
}
