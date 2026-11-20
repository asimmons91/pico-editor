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
// newline, and is preserved. Apply this exactly once at an input boundary: a
// second pass would treat a preserved lone CR that is now adjacent to the
// inserted LF as another CRLF. Buffer.Apply itself expects canonical text.
func NormalizeNewlines(text string) string {
	if !strings.Contains(text, "\r\n") {
		return text
	}
	return strings.ReplaceAll(text, "\r\n", "\n")
}

// Apply replaces a range and returns the recorded change. Text must use
// canonical LF newlines; input layers normalize CRLF exactly once before
// calling. A lone CR is a P8SCII character and is preserved. Apply does not
// move a cursor; Editor wraps it for cursor-aware operations.
func (b *Buffer) Apply(edit Edit) (Change, error) {
	return b.apply(edit, true)
}

// applyRaw is the undo/redo path. It allows range endpoints inside a cluster so
// that reversing an insertion which merged with an adjacent VS16 can restore
// the original text. The endpoints still must be valid rune offsets, and the
// resulting buffer remains valid UTF-8.
func (b *Buffer) applyRaw(edit Edit) (Change, error) {
	return b.apply(edit, false)
}

func (b *Buffer) apply(edit Edit, requireBoundaries bool) (Change, error) {
	text := edit.Text
	if !utf8.ValidString(text) {
		return Change{}, ErrInvalidText
	}
	if requireBoundaries {
		if err := b.validatePos(edit.Start); err != nil {
			return Change{}, err
		}
		if err := b.validatePos(edit.End); err != nil {
			return Change{}, err
		}
	} else {
		if err := b.validateRunePos(edit.Start); err != nil {
			return Change{}, err
		}
		if err := b.validateRunePos(edit.End); err != nil {
			return Change{}, err
		}
	}
	if edit.End.Before(edit.Start) {
		return Change{}, ErrInvalidRange
	}
	if edit.Start == edit.End && text == "" {
		return Change{}, nil
	}

	oldText, err := b.textInRange(edit.Start, edit.End, requireBoundaries)
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
