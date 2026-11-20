package vim

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/asimmons91/pico-editor/internal/editor"
)

var exCommandNames = []string{
	"write", "wq", "quit", "edit", "bnext", "bprevious", "buffer", "ls",
	"tabnext", "tabprevious", "help", "noh", "substitute", "diag",
	"messages", "reload", "x",
}

var exAliases = map[string]string{
	"w":    "write",
	"q":    "quit",
	"e":    "edit",
	"bn":   "bnext",
	"bp":   "bprevious",
	"b":    "buffer",
	"h":    "help",
	"s":    "substitute",
	"tabn": "tabnext",
	"tabp": "tabprevious",
}

type exRange struct {
	first int
	last  int
	set   bool
}

func (v *Vim) executeEx(ed *editor.Editor, line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	v.exHistory = appendHistory(v.exHistory, line)

	rangeSpec, rest, ok := parseExRange(line, ed, v)
	if !ok {
		v.ringBell()
		return
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		v.message(msgNotEditorCommand)
		v.ringBell()
		return
	}

	name, argument, bang, ok := splitExCommand(rest)
	if !ok {
		v.ringBell()
		return
	}
	command, ok := resolveExCommand(name)
	if !ok {
		v.message(msgNotEditorCommand)
		v.ringBell()
		return
	}

	switch command {
	case "write":
		if ed.Buffer.Path == "" {
			v.message(msgNoFileName)
			v.ringBell()
			return
		}
		v.queue(Action{Kind: ActionSave})
	case "wq":
		if ed.Buffer.Path == "" {
			v.message(msgNoFileName)
			v.ringBell()
			return
		}
		v.queue(Action{Kind: ActionSave, QuitAfter: true})
	case "x":
		if ed.Buffer.Path == "" {
			v.message(msgNoFileName)
			v.ringBell()
			return
		}
		v.queue(Action{Kind: ActionSave, OnlyIfDirty: true, QuitAfter: true})
	case "quit":
		if ed.Buffer.Dirty() && !bang {
			v.message(msgNoWriteSinceChange)
			v.ringBell()
			return
		}
		v.queue(Action{Kind: ActionQuit, Force: bang})
	case "edit":
		v.recordJump(ed)
		if ed.Buffer.Dirty() && !bang {
			v.message(msgNoWriteSinceChange)
			v.ringBell()
			return
		}
		path := strings.TrimSpace(argument)
		v.queue(Action{Kind: ActionOpen, Path: path, Force: bang})
	case "bnext":
		v.queue(Action{Kind: ActionSwitchBuffer, Delta: 1})
	case "bprevious":
		v.queue(Action{Kind: ActionSwitchBuffer, Delta: -1})
	case "buffer":
		argument = strings.TrimSpace(argument)
		if argument == "" {
			v.queue(Action{Kind: ActionListBuffers})
			return
		}
		number, err := strconv.Atoi(argument)
		if err != nil || number < 1 {
			v.message(msgNoBuffer)
			v.ringBell()
			return
		}
		v.queue(Action{Kind: ActionSwitchBuffer, Number: number})
	case "ls":
		v.queue(Action{Kind: ActionListBuffers})
	case "tabnext":
		v.queue(Action{Kind: ActionSwitchTab, Delta: 1})
	case "tabprevious":
		v.queue(Action{Kind: ActionSwitchTab, Delta: -1})
	case "help":
		topic := strings.TrimSpace(argument)
		switch topic {
		case "", "keys", "input", "lsp", "config", "troubleshooting":
		default:
			v.message("E149: Sorry, no help for " + topic)
			v.ringBell()
			return
		}
		v.queue(Action{Kind: ActionHelp, Topic: topic})
	case "noh":
		v.ClearSearchHighlight()
	case "substitute":
		v.substitute(ed, rangeSpec, argument)
	case "diag":
		v.info(":diag is available in M5")
	case "messages":
		v.info(":messages is available in M7")
	case "reload":
		v.info(":reload is available in M7")
	default:
		v.message(msgNotEditorCommand)
		v.ringBell()
	}
}

func splitExCommand(rest string) (name, argument string, bang bool, ok bool) {
	if rest == "" {
		return "", "", false, false
	}
	index := 0
	for index < len(rest) {
		r, size := utf8.DecodeRuneInString(rest[index:])
		if !unicode.IsLetter(r) {
			break
		}
		index += size
	}
	if index == 0 {
		return "", "", false, false
	}
	name = rest[:index]
	remainder := rest[index:]
	if strings.HasPrefix(remainder, "!") {
		bang = true
		remainder = remainder[1:]
	}
	return name, remainder, bang, true
}

func resolveExCommand(name string) (string, bool) {
	if canonical, ok := exAliases[name]; ok {
		return canonical, true
	}
	for _, command := range exCommandNames {
		if command == name {
			return command, true
		}
	}
	matches := make(map[string]bool)
	for _, command := range exCommandNames {
		if strings.HasPrefix(command, name) {
			matches[command] = true
		}
	}
	for alias := range exAliases {
		if strings.HasPrefix(alias, name) {
			matches[exAliases[alias]] = true
		}
	}
	if len(matches) == 1 {
		for command := range matches {
			return command, true
		}
	}
	return "", false
}

func parseExRange(line string, ed *editor.Editor, v *Vim) (exRange, string, bool) {
	current := ed.Cursor.Pos.Line
	whole := exRange{first: 0, last: len(ed.Buffer.Lines) - 1}
	if strings.HasPrefix(line, "%") {
		return whole, line[1:], true
	}
	if strings.HasPrefix(line, "'<,'>") {
		if !v.hasVisualRange {
			return exRange{}, "", false
		}
		return exRange{first: v.lastVisualRange[0], last: v.lastVisualRange[1], set: true}, line[len("'<,'>"):], true
	}
	if line == "." || strings.HasPrefix(line, ".") {
		return exRange{first: current, last: current, set: true}, line[1:], true
	}
	if strings.HasPrefix(line, "$") {
		return exRange{first: len(ed.Buffer.Lines) - 1, last: len(ed.Buffer.Lines) - 1, set: true}, line[1:], true
	}
	if line[0] < '0' || line[0] > '9' {
		return exRange{first: current, last: current}, line, true
	}
	first, consumed := parseExNumber(line)
	rest := line[consumed:]
	if strings.HasPrefix(rest, ",") {
		second, secondConsumed := parseExNumber(rest[1:])
		if secondConsumed == 0 {
			return exRange{}, "", false
		}
		rest = rest[1+secondConsumed:]
		first = clampLine(first-1, len(ed.Buffer.Lines))
		second = clampLine(second-1, len(ed.Buffer.Lines))
		if first > second {
			first, second = second, first
		}
		return exRange{first: first, last: second, set: true}, rest, true
	}
	first = clampLine(first-1, len(ed.Buffer.Lines))
	return exRange{first: first, last: first, set: true}, rest, true
}

func parseExNumber(text string) (int, int) {
	index := 0
	for index < len(text) && text[index] >= '0' && text[index] <= '9' {
		index++
	}
	if index == 0 {
		return 0, 0
	}
	number, _ := strconv.Atoi(text[:index])
	return number, index
}

func (v *Vim) substitute(ed *editor.Editor, lineRange exRange, args string) {
	// splitExCommand stops at the first non-letter, so "s/old/new/g" leaves
	// args as "/old/new/g". A bang is accepted and ignored.
	args = strings.TrimPrefix(strings.TrimSpace(args), "!")
	if args == "" {
		v.message(msgNoPreviousCommand)
		v.ringBell()
		return
	}
	delimiter, size := utf8.DecodeRuneInString(args)
	if unicode.IsLetter(delimiter) || unicode.IsDigit(delimiter) || delimiter == '\\' {
		v.message("E146: Regular expressions can't be delimited by letters")
		v.ringBell()
		return
	}
	args = args[size:]
	pattern, remainder, ok := readDelimited(args, delimiter)
	if !ok {
		v.message("E486: Pattern not found")
		v.ringBell()
		return
	}
	replacement, remainder, ok := readDelimited(remainder, delimiter)
	if !ok {
		v.message("E486: Pattern not found")
		v.ringBell()
		return
	}
	flags := remainder
	global := strings.ContainsRune(flags, 'g')
	insensitive := strings.ContainsRune(flags, 'i')
	if pattern == "" {
		pattern = v.search.pattern
	}
	if pattern == "" {
		v.message(msgNoPreviousCommand)
		v.ringBell()
		return
	}
	mode := caseSmart
	if insensitive {
		mode = caseInsensitive
	}
	expression, err := compileVim(pattern, mode)
	if err != nil {
		v.message("E486: Pattern not found")
		v.ringBell()
		return
	}

	first := clampLine(lineRange.first, len(ed.Buffer.Lines))
	last := clampLine(lineRange.last, len(ed.Buffer.Lines))
	newLines := make([]string, 0, last-first+1)
	total := 0
	changedLines := 0
	firstChangedLine := first
	for line := first; line <= last; line++ {
		old := ed.Buffer.Lines[line]
		matches := expression.FindAllStringSubmatchIndex(old, -1)
		if len(matches) == 0 {
			newLines = append(newLines, old)
			continue
		}
		if !global && len(matches) > 1 {
			matches = matches[:1]
		}
		var out strings.Builder
		position := 0
		for _, match := range matches {
			out.WriteString(old[position:match[0]])
			out.WriteString(expandReplacement(old, match, replacement))
			position = match[1]
			total++
		}
		out.WriteString(old[position:])
		newLines = append(newLines, out.String())
		changedLines++
		if firstChangedLine == first && changedLines == 1 {
			firstChangedLine = line
		}
	}
	if total == 0 {
		v.message(msgPatternNotFound)
		v.ringBell()
		return
	}
	oldText := strings.Join(ed.Buffer.Lines[first:last+1], "\n")
	newText := strings.Join(newLines, "\n")
	if oldText == newText {
		v.message(msgPatternNotFound)
		v.ringBell()
		return
	}
	ed.EndGroup()
	if _, err := ed.Apply(editor.Edit{
		Start: editor.Pos{Line: first},
		End:   editor.Pos{Line: last, Rune: utf8.RuneCountInString(ed.Buffer.Lines[last])},
		Text:  newText,
	}); err != nil {
		v.message("E486: Pattern not found")
		v.ringBell()
		return
	}
	ed.SetCursor(editor.Pos{Line: firstChangedLine})
	v.info(strconv.Itoa(total) + " substitutions on " + strconv.Itoa(changedLines) + " lines")
}

func readDelimited(text string, delimiter rune) (string, string, bool) {
	var out strings.Builder
	escaped := false
	for index := 0; index < len(text); {
		r, size := utf8.DecodeRuneInString(text[index:])
		if escaped {
			out.WriteRune('\\')
			out.WriteRune(r)
			escaped = false
			index += size
			continue
		}
		if r == '\\' {
			escaped = true
			index += size
			continue
		}
		if r == delimiter {
			return out.String(), text[index+size:], true
		}
		out.WriteRune(r)
		index += size
	}
	return "", "", false
}

func expandReplacement(line string, match []int, replacement string) string {
	var out strings.Builder
	for index := 0; index < len(replacement); {
		r, size := utf8.DecodeRuneInString(replacement[index:])
		if r == '&' {
			out.WriteString(line[match[0]:match[1]])
			index += size
			continue
		}
		if r == '\\' && index+size < len(replacement) {
			next, nextSize := utf8.DecodeRuneInString(replacement[index+size:])
			switch {
			case next >= '0' && next <= '9':
				group := int(next - '0')
				if group*2+1 < len(match) && match[group*2] >= 0 {
					out.WriteString(line[match[group*2]:match[group*2+1]])
				}
				index += size + nextSize
				continue
			case next == '/':
				out.WriteRune('/')
				index += size + nextSize
				continue
			case next == '\\':
				out.WriteRune('\\')
				index += size + nextSize
				continue
			default:
				out.WriteRune(next)
				index += size + nextSize
				continue
			}
		}
		out.WriteRune(r)
		index += size
	}
	return out.String()
}
