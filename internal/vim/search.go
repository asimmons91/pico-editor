package vim

import (
	"regexp"
	"unicode"
	"unicode/utf8"

	"github.com/asimmons91/pico-editor/internal/editor"
)

func (v *Vim) openSearch(ed *editor.Editor, forward bool) {
	kind := commandSearchForward
	if !forward {
		kind = commandSearchBackward
	}
	v.cmdline = commandLineState{
		active:       true,
		kind:         kind,
		savedCursor:  ed.Cursor.Pos,
		savedTopLine: ed.Viewport.TopLine,
		historyPos:   len(v.search.history),
	}
	v.Mode = ModeCommandLine
	v.search.highlight = true
	v.Count = 0
	v.Operator = OpNone
	v.pending = pendingState{}
}

func (v *Vim) updateSearchPreview(ed *editor.Editor) {
	if v.cmdline.kind != commandSearchForward && v.cmdline.kind != commandSearchBackward {
		return
	}
	pattern := string(v.cmdline.text)
	if pattern == "" {
		return
	}
	forward := v.cmdline.kind == commandSearchForward
	if !v.previewSearch(ed, pattern, forward, v.cmdline.savedCursor) {
		return
	}
}

func (v *Vim) previewSearch(ed *editor.Editor, pattern string, forward bool, from editor.Pos) bool {
	if _, err := compileVim(pattern, caseSmart); err != nil {
		return false
	}
	search := v.searchObject(pattern)
	search.Forward = forward
	match, ok := findInWindow(ed, search, searchStart(ed, from, forward), forward)
	if !ok {
		return false
	}
	ed.SetCursor(match.Start)
	return true
}

func (v *Vim) commitSearch(ed *editor.Editor) {
	pattern := string(v.cmdline.text)
	forward := v.cmdline.kind == commandSearchForward
	saved := v.cmdline.savedCursor
	v.cmdline = commandLineState{}
	v.Mode = ModeNormal
	if pattern == "" {
		pattern = v.search.pattern
	}
	if pattern == "" {
		v.message(msgPatternNotFound)
		v.ringBell()
		return
	}
	if _, err := compileVim(pattern, caseSmart); err != nil {
		v.message(msgPatternNotFound)
		v.ringBell()
		return
	}
	v.search.pattern = pattern
	v.search.forward = forward
	v.search.highlight = true
	v.search.history = appendHistory(v.search.history, pattern)
	v.searchMoveFrom(ed, forward, 1, saved, true, true)
}

func (v *Vim) cancelSearch(ed *editor.Editor) {
	ed.SetCursor(v.cmdline.savedCursor)
	ed.Viewport.TopLine = v.cmdline.savedTopLine
	v.cmdline = commandLineState{}
	v.Mode = ModeNormal
}

func (v *Vim) searchMove(ed *editor.Editor, forward bool, count int, recordJump bool) {
	if v.search.pattern == "" {
		v.message(msgPatternNotFound)
		v.ringBell()
		return
	}
	if _, err := compileVim(v.search.pattern, caseSmart); err != nil {
		v.message(msgPatternNotFound)
		v.ringBell()
		return
	}
	v.searchMoveFrom(ed, forward, count, ed.Cursor.Pos, recordJump, true)
}

func (v *Vim) searchMoveFrom(ed *editor.Editor, forward bool, count int, from editor.Pos, recordJump, skipCurrent bool) {
	search := v.searchObject(v.search.pattern)
	search.Forward = forward
	position := from
	if skipCurrent {
		position = searchStart(ed, position, forward)
	}
	if recordJump {
		v.recordJump(ed)
	}
	var match editor.Match
	for i := 0; i < max(1, count); i++ {
		result, ok := findInWindow(ed, search, position, forward)
		if !ok {
			v.message(msgPatternNotFound)
			v.ringBell()
			return
		}
		match = result
		if forward {
			position = ed.Buffer.NextCluster(match.Start)
			if position == match.Start {
				break
			}
		} else {
			position = match.Start
		}
	}
	ed.SetCursor(match.Start)
}

// findInWindow searches only the active tab's lines. It wraps inside the tab,
// never jumping into a hidden separator or another code tab.
func findInWindow(ed *editor.Editor, search editor.Search, from editor.Pos, forward bool) (editor.Match, bool) {
	matches, err := search.FindAll(ed.Buffer.Text())
	if err != nil || len(matches) == 0 {
		return editor.Match{}, false
	}
	within := matches[:0]
	for _, match := range matches {
		if ed.HasLine(match.Start.Line) && ed.HasLine(match.End.Line) {
			within = append(within, match)
		}
	}
	if len(within) == 0 {
		return editor.Match{}, false
	}
	if forward {
		for _, match := range within {
			if !match.Start.Before(from) {
				return match, true
			}
		}
		return within[0], true
	}
	for index := len(within) - 1; index >= 0; index-- {
		if within[index].Start.Before(from) {
			return within[index], true
		}
	}
	return within[len(within)-1], true
}

func searchStart(ed *editor.Editor, from editor.Pos, forward bool) editor.Pos {
	if forward {
		return ed.Buffer.NextCluster(from)
	}
	return from
}

func (v *Vim) searchWord(ed *editor.Editor, forward bool) bool {
	if ed.WindowEmpty() {
		return false
	}
	line := ed.Buffer.Lines[ed.Cursor.Pos.Line]
	if len(line) == 0 {
		return false
	}
	position := ed.Cursor.Pos.Rune
	if position >= utf8.RuneCountInString(line) {
		position = prevClusterRune(line, position)
	}
	if !isKeywordRune(baseRuneAt(line, position)) {
		return false
	}
	start := position
	for start > 0 && isKeywordRune(baseRuneAt(line, prevClusterRune(line, start))) {
		start = prevClusterRune(line, start)
	}
	end := nextClusterRune(line, position)
	for end < utf8.RuneCountInString(line) && isKeywordRune(baseRuneAt(line, end)) {
		end = nextClusterRune(line, end)
	}
	word := runesBetween(line, start, end)
	if word == "" {
		return false
	}
	v.search.pattern = `\b` + regexp.QuoteMeta(word) + `\b`
	v.search.forward = forward
	v.search.highlight = true
	v.search.history = appendHistory(v.search.history, v.search.pattern)
	return true
}

func (v *Vim) searchObject(pattern string) editor.Search {
	translated, insensitive, err := translateVim(pattern, caseSmart)
	if err != nil {
		translated = regexp.QuoteMeta(pattern)
		insensitive = false
	}
	if insensitive {
		translated = "(?i)" + translated
	}
	return editor.Search{Pattern: translated, Regex: true}
}

// SearchHighlights returns all cluster-aligned matches for rendering. While a
// search prompt is open it previews the prompt text.
func (v *Vim) SearchHighlights(ed *editor.Editor) []editor.Match {
	if ed == nil || !v.search.highlight {
		return nil
	}
	pattern := v.search.pattern
	if v.Mode == ModeCommandLine && (v.cmdline.kind == commandSearchForward || v.cmdline.kind == commandSearchBackward) {
		pattern = string(v.cmdline.text)
	}
	if pattern == "" {
		return nil
	}
	search := v.searchObject(pattern)
	matches, err := search.FindAll(ed.Buffer.Text())
	if err != nil {
		return nil
	}
	filtered := matches[:0]
	for _, match := range matches {
		if ed.HasLine(match.Start.Line) && ed.HasLine(match.End.Line) {
			filtered = append(filtered, match)
		}
	}
	return filtered
}

// SearchPattern returns the committed search pattern.
func (v *Vim) SearchPattern() string {
	return v.search.pattern
}

// ClearSearchHighlight implements :noh.
func (v *Vim) ClearSearchHighlight() {
	v.search.highlight = false
}

func isKeywordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func runesBetween(line string, start, end int) string {
	offset := 0
	seen := 0
	beginByte := 0
	for offset < len(line) && seen < start {
		_, size := utf8.DecodeRuneInString(line[offset:])
		offset += size
		seen++
	}
	beginByte = offset
	for offset < len(line) && seen < end {
		_, size := utf8.DecodeRuneInString(line[offset:])
		offset += size
		seen++
	}
	return line[beginByte:offset]
}

func appendHistory(history []string, value string) []string {
	if value == "" {
		return history
	}
	if len(history) > 0 && history[len(history)-1] == value {
		return history
	}
	history = append(history, value)
	if len(history) > 100 {
		history = history[len(history)-100:]
	}
	return history
}
