package editor

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Search describes a literal or regex search. Pattern is canonical text and
// may span lines with a literal newline.
type Search struct {
	Pattern   string
	Regex     bool
	SmartCase bool
	Forward   bool
}

// Match is a cluster-aligned half-open range. Wrapped reports that the search
// reached the end (or start) of the buffer and continued from the other end.
type Match struct {
	Start   Pos
	End     Pos
	Wrapped bool
}

// Find returns the first match at or after from when Forward is set, or the
// last match before from otherwise. It wraps at the buffer boundaries. An empty
// pattern finds nothing; an invalid regex returns ErrInvalidPattern without
// changing the Search value.
func (s Search) Find(text string, from Pos) (Match, bool, error) {
	if s.Pattern == "" {
		return Match{}, false, nil
	}
	pattern := s.Pattern
	if !s.Regex {
		pattern = regexp.QuoteMeta(pattern)
	}
	if s.SmartCase && !hasUpper(s.Pattern) {
		pattern = "(?i)" + pattern
	}
	expression, err := regexp.Compile(pattern)
	if err != nil {
		return Match{}, false, fmt.Errorf("%w: %v", ErrInvalidPattern, err)
	}

	index := newTextIndex(text)
	fromByte, err := index.posToByte(from)
	if err != nil {
		return Match{}, false, err
	}
	locations := expression.FindAllStringIndex(text, -1)
	if len(locations) == 0 {
		return Match{}, false, nil
	}

	location := locations[0]
	wrapped := false
	if s.Forward {
		found := false
		for _, candidate := range locations {
			if candidate[0] >= fromByte {
				location = candidate
				found = true
				break
			}
		}
		if !found {
			location = locations[0]
			wrapped = true
		}
	} else {
		found := false
		for i := len(locations) - 1; i >= 0; i-- {
			if locations[i][0] < fromByte {
				location = locations[i]
				found = true
				break
			}
		}
		if !found {
			location = locations[len(locations)-1]
			wrapped = true
		}
	}

	start, err := index.byteToPos(location[0])
	if err != nil {
		return Match{}, false, err
	}
	end, err := index.byteToPos(location[1])
	if err != nil {
		return Match{}, false, err
	}
	start = clampToBufferLines(index, start)
	end = clampToBufferLines(index, end)

	startLine := index.lineText(start.Line)
	start.Rune = clusterStart(startLine, start.Rune)

	endLine := index.lineText(end.Line)
	if end.Rune < utf8.RuneCountInString(endLine) && clusterStart(endLine, end.Rune) != end.Rune {
		end.Rune = clusterEnd(endLine, end.Rune)
	}
	if end.Before(start) {
		end = start
	}
	return Match{Start: start, End: end, Wrapped: wrapped}, true, nil
}

// clampToBufferLines mirrors Buffer's line model: a trailing newline is
// represented by TrailingNewline, not an extra empty line. Matches at the
// implicit terminator map to the last real line's end.
func clampToBufferLines(index *textIndex, position Pos) Pos {
	lineCount := len(index.starts)
	if strings.HasSuffix(index.text, "\n") {
		lineCount--
	}
	if lineCount < 1 {
		lineCount = 1
	}
	if position.Line < lineCount {
		return position
	}
	position.Line = lineCount - 1
	position.Rune = utf8.RuneCountInString(index.lineText(position.Line))
	return position
}

// FindIn is Find against a buffer snapshot.
func (s Search) FindIn(buffer *Buffer, from Pos) (Match, bool, error) {
	return s.Find(buffer.Text(), from)
}

func hasUpper(text string) bool {
	for _, r := range text {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}
