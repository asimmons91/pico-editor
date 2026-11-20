package editor

import (
	"strings"
	"unicode/utf8"
)

// Edit replaces the text between Start and End with Text. Both positions must
// be cluster boundaries in the current buffer.
type Edit struct {
	Start Pos
	End   Pos
	Text  string
}

// NormalizeNewlines converts CRLF to LF. A lone CR is a P8SCII character, not a
// newline, and is preserved.
func NormalizeNewlines(text string) string {
	if !strings.Contains(text, "\r\n") {
		return text
	}
	return strings.ReplaceAll(text, "\r\n", "\n")
}

// Apply replaces a range and returns the recorded change. It does not move a
// cursor; Editor wraps it for cursor-aware operations.
func (b *Buffer) Apply(edit Edit) (Change, error) {
	text := NormalizeNewlines(edit.Text)
	if !utf8.ValidString(text) {
		return Change{}, ErrInvalidText
	}
	if err := b.validatePos(edit.Start); err != nil {
		return Change{}, err
	}
	if err := b.validatePos(edit.End); err != nil {
		return Change{}, err
	}
	if edit.End.Before(edit.Start) {
		return Change{}, ErrInvalidRange
	}
	if edit.Start == edit.End && text == "" {
		return Change{}, nil
	}

	oldText, err := b.TextInRange(edit.Start, edit.End)
	if err != nil {
		return Change{}, err
	}
	if oldText == text {
		return Change{}, nil
	}

	first := b.Lines[edit.Start.Line]
	last := b.Lines[edit.End.Line]
	combined := first[:byteIndexForRune(first, edit.Start.Rune)] +
		text +
		last[byteIndexForRune(last, edit.End.Rune):]
	replacement := strings.Split(combined, "\n")

	lines := make([]string, 0, len(b.Lines)-(edit.End.Line-edit.Start.Line)+len(replacement))
	lines = append(lines, b.Lines[:edit.Start.Line]...)
	lines = append(lines, replacement...)
	lines = append(lines, b.Lines[edit.End.Line+1:]...)
	b.Lines = lines
	b.invalidate()

	return Change{
		Start:   edit.Start,
		OldText: oldText,
		NewText: text,
		Before:  edit.Start,
		After:   posAfterText(edit.Start, text),
	}, nil
}
