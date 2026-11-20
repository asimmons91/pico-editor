package vim

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

type caseMode uint8

const (
	caseSmart caseMode = iota
	caseSensitive
	caseInsensitive
)

// translateVim converts the Vim-magic subset in docs/VIM.md to Go regexp
// syntax. It returns whether the expression should be case-insensitive.
func translateVim(pattern string, mode caseMode) (string, bool, error) {
	insensitive := mode == caseInsensitive
	if mode == caseSmart && !containsUpper(pattern) {
		insensitive = true
	}

	var out strings.Builder
	inClass := false
	for i := 0; i < len(pattern); {
		r, size := utf8.DecodeRuneInString(pattern[i:])
		if r == utf8.RuneError && size == 1 {
			return "", false, ErrInvalidPattern
		}
		if r == '\\' {
			if i+size >= len(pattern) {
				out.WriteString(`\\`)
				i += size
				continue
			}
			next, nextSize := utf8.DecodeRuneInString(pattern[i+size:])
			switch next {
			case '(', ')', '|', '+', '?', '{', '}':
				out.WriteRune(next)
			case 'c':
				insensitive = true
			case 'C':
				insensitive = false
			case '<', '>':
				out.WriteString(`\b`)
			case 'n':
				out.WriteString(`\n`)
			case 't':
				out.WriteString(`\t`)
			case '/':
				out.WriteRune('/')
			case '\\':
				out.WriteString(`\\`)
			case '.', '*', '[', ']', '^', '$':
				out.WriteRune('\\')
				out.WriteRune(next)
			default:
				out.WriteRune('\\')
				out.WriteRune(next)
			}
			i += size + nextSize
			continue
		}
		if inClass {
			if r == ']' {
				inClass = false
			}
			out.WriteRune(r)
			i += size
			continue
		}
		switch r {
		case '[':
			inClass = true
			out.WriteRune(r)
		case '(', ')', '|', '+', '?', '{', '}':
			out.WriteRune('\\')
			out.WriteRune(r)
		default:
			out.WriteRune(r)
		}
		i += size
	}
	return out.String(), insensitive, nil
}

func compileVim(pattern string, mode caseMode) (*regexp.Regexp, error) {
	translated, insensitive, err := translateVim(pattern, mode)
	if err != nil {
		return nil, err
	}
	if insensitive {
		translated = "(?i)" + translated
	}
	expression, err := regexp.Compile(translated)
	if err != nil {
		return nil, ErrInvalidPattern
	}
	return expression, nil
}

func containsUpper(text string) bool {
	for _, r := range text {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}
