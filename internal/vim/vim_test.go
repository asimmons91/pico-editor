package vim

import (
	"strings"
	"testing"

	"github.com/asimmons91/pico-editor/internal/editor"
	"github.com/asimmons91/pico-editor/internal/input"
)

func testEditor(t *testing.T, text string, line, runeIndex int) *editor.Editor {
	t.Helper()
	buffer := editor.New("test.lua")
	if text == "" {
		buffer.Lines = []string{""}
	} else {
		buffer.Lines = strings.Split(text, "\n")
	}
	buffer.TrailingNewline = false
	buffer.MarkSaved()
	ed := editor.NewEditor(buffer)
	ed.SetCursor(editor.Pos{Line: line, Rune: runeIndex})
	return ed
}

func feed(t *testing.T, v *Vim, ed *editor.Editor, keys string) []Result {
	t.Helper()
	events := parseKeys(t, keys)
	results := make([]Result, 0, len(events))
	for _, event := range events {
		results = append(results, v.Handle(event, ed))
	}
	return results
}

func parseKeys(t *testing.T, keys string) []input.Event {
	t.Helper()
	var events []input.Event
	for index := 0; index < len(keys); {
		if keys[index] == '<' {
			end := strings.IndexByte(keys[index:], '>')
			if end > 0 {
				rawName := keys[index+1 : index+end]
				if strings.HasPrefix(strings.ToLower(rawName), "text:") {
					events = append(events, input.Event{Kind: input.KindText, Runes: []rune(rawName[5:])})
					index += end + 1
					continue
				}
				name := strings.ToLower(rawName)
				if event, ok := namedKey(name); ok {
					events = append(events, event)
					index += end + 1
					continue
				}
			}
		}
		r, size := decodeRune(keys[index:])
		event, ok := keyEventForRune(r)
		if !ok {
			t.Fatalf("no key mapping for %q", r)
		}
		events = append(events, event)
		if r >= 0x20 {
			events = append(events, input.Event{Kind: input.KindText, Mods: event.Mods, Runes: []rune{r}})
		}
		index += size
	}
	return events
}

func namedKey(name string) (input.Event, bool) {
	var key input.Key
	mods := input.Mods(0)
	switch name {
	case "esc":
		key = input.KeyEsc
	case "c-[":
		key = input.KeyLeftBracket
		mods = input.ModCtrl
	case "enter":
		key = input.KeyEnter
	case "tab":
		key = input.KeyTab
	case "bs":
		key = input.KeyBackspace
	case "del":
		key = input.KeyDelete
	case "space":
		key = input.KeySpace
	case "up":
		key = input.KeyUp
	case "down":
		key = input.KeyDown
	case "left":
		key = input.KeyLeft
	case "right":
		key = input.KeyRight
	case "home":
		key = input.KeyHome
	case "end":
		key = input.KeyEnd
	case "c-d":
		key = input.KeyD
		mods = input.ModCtrl
	case "c-u":
		key = input.KeyU
		mods = input.ModCtrl
	case "c-f":
		key = input.KeyF
		mods = input.ModCtrl
	case "c-b":
		key = input.KeyB
		mods = input.ModCtrl
	case "c-o":
		key = input.KeyO
		mods = input.ModCtrl
	case "c-i":
		key = input.KeyI
		mods = input.ModCtrl
	case "c-r":
		key = input.KeyR
		mods = input.ModCtrl
	case "c-v":
		key = input.KeyV
		mods = input.ModCtrl
	case "c-w":
		key = input.KeyW
		mods = input.ModCtrl
	case "c-j":
		key = input.KeyJ
		mods = input.ModCtrl
	case "c-k":
		key = input.KeyK
		mods = input.ModCtrl
	case "c-p":
		key = input.KeyP
		mods = input.ModCtrl
	case "c-n":
		key = input.KeyN
		mods = input.ModCtrl
	case "c-space":
		key = input.KeySpace
		mods = input.ModCtrl
	case "pgup":
		key = input.KeyPageUp
	case "pgdown":
		key = input.KeyPageDown
	default:
		return input.Event{}, false
	}
	return input.Event{Kind: input.KindKeyDown, Key: key, Mods: mods}, true
}

func keyEventForRune(r rune) (input.Event, bool) {
	shift := false
	var key input.Key
	switch {
	case r >= 'a' && r <= 'z':
		key = input.KeyA + input.Key(r-'a')
	case r >= 'A' && r <= 'Z':
		key = input.KeyA + input.Key(r-'A')
		shift = true
	case r >= '0' && r <= '9':
		key = input.Key0 + input.Key(r-'0')
	case r == ' ':
		key = input.KeySpace
	case r == ':':
		key, shift = input.KeySemicolon, true
	case r == ';':
		key = input.KeySemicolon
	case r == '"':
		key, shift = input.KeyApostrophe, true
	case r == '\'':
		key = input.KeyApostrophe
	case r == '<':
		key, shift = input.KeyComma, true
	case r == '>':
		key, shift = input.KeyPeriod, true
	case r == ',':
		key = input.KeyComma
	case r == '.':
		key = input.KeyPeriod
	case r == '/':
		key = input.KeySlash
	case r == '?':
		key, shift = input.KeySlash, true
	case r == '[':
		key = input.KeyLeftBracket
	case r == ']':
		key = input.KeyRightBracket
	case r == '{':
		key, shift = input.KeyLeftBracket, true
	case r == '}':
		key, shift = input.KeyRightBracket, true
	case r == '\\':
		key = input.KeyBackslash
	case r == '|':
		key, shift = input.KeyBackslash, true
	case r == '-':
		key = input.KeyMinus
	case r == '_':
		key, shift = input.KeyMinus, true
	case r == '=':
		key = input.KeyEqual
	case r == '+':
		key, shift = input.KeyEqual, true
	case r == '~':
		key, shift = input.KeyBacktick, true
	case r == '`':
		key = input.KeyBacktick
	case r == '%':
		key, shift = input.Key5, true
	case r == '#':
		key, shift = input.Key3, true
	case r == '*':
		key, shift = input.Key8, true
	case r == '^':
		key, shift = input.Key6, true
	case r == '$':
		key, shift = input.Key4, true
	case r == '!':
		key, shift = input.Key1, true
	case r == '@':
		key, shift = input.Key2, true
	case r == '&':
		key, shift = input.Key7, true
	case r == '(':
		key, shift = input.Key9, true
	case r == ')':
		key, shift = input.Key0, true
	default:
		return input.Event{}, false
	}
	mods := input.Mods(0)
	if shift {
		mods = input.ModShift
	}
	return input.Event{Kind: input.KindKeyDown, Key: key, Mods: mods}, true
}

func decodeRune(text string) (rune, int) {
	for _, r := range text {
		return r, len(string(r))
	}
	return 0, 0
}

func TestSmoke(t *testing.T) {
	cases := []struct {
		name   string
		buffer string
		cursor [2]int
		keys   string
		want   string
	}{
		{"dw", "one two three", [2]int{0, 4}, "dw", "one three"},
		{"dd", "one\ntwo\nthree", [2]int{1, 0}, "dd", "one\nthree"},
		{"insert", "abc", [2]int{0, 1}, "iXY<esc>", "aXYbc"},
		{"append", "abc", [2]int{0, 1}, "aXY<esc>", "abXYc"},
		{"delete char", "abc", [2]int{0, 1}, "x", "ac"},
		{"change word", "one two", [2]int{0, 0}, "cwX<esc>", "X two"},
		{"yank paste", "one", [2]int{0, 0}, "yyp", "one\none"},
		{"replace", "abc", [2]int{0, 1}, "rZ", "aZc"},
		{"join", "one\ntwo", [2]int{0, 0}, "J", "one two"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ed := testEditor(t, tc.buffer, tc.cursor[0], tc.cursor[1])
			v := New()
			feed(t, v, ed, tc.keys)
			got := strings.Join(ed.Buffer.Lines, "\n")
			if got != tc.want {
				t.Fatalf("buffer = %q, want %q (mode %s)", got, tc.want, v.Mode)
			}
		})
	}
}

func TestSmokeExSubstitute(t *testing.T) {
	ed := testEditor(t, "old old\nkeep\nold", 0, 0)
	v := New()
	feed(t, v, ed, ":%s/old/new/g<enter>")
	got := strings.Join(ed.Buffer.Lines, "\n")
	if got != "new new\nkeep\nnew" {
		t.Fatalf("buffer = %q", got)
	}
}

func TestSmokeVisualBlock(t *testing.T) {
	ed := testEditor(t, "abcd\nefgh\nijkl", 0, 1)
	v := New()
	feed(t, v, ed, "<c-v>jld")
	got := strings.Join(ed.Buffer.Lines, "\n")
	if got != "ad\neh\nijkl" {
		t.Fatalf("buffer = %q", got)
	}
}
