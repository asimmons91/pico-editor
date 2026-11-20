package vim

import (
	"strings"
	"testing"

	"github.com/asimmons91/pico-editor/internal/editor"
	"github.com/asimmons91/pico-editor/internal/input"
)

func TestTranslateVimMagic(t *testing.T) {
	cases := []struct {
		pattern     string
		mode        caseMode
		want        string
		insensitive bool
	}{
		{"a.b", caseSensitive, "a.b", false},
		{`a\.b`, caseSensitive, `a\.b`, false},
		{`\(a\|b\)`, caseSensitive, `(a|b)`, false},
		{`a\+`, caseSensitive, `a+`, false},
		{"a?", caseSensitive, `a\?`, false},
		{`a\{2,3\}`, caseSensitive, `a{2,3}`, false},
		{"a(b)", caseSensitive, `a\(b\)`, false},
		{"[a+]", caseSensitive, "[a+]", false},
		{"\\cFoo", caseSmart, "Foo", true},
		{"\\CFoo", caseSmart, "Foo", false},
		{"foo", caseSmart, "foo", true},
		{"Foo", caseSmart, "Foo", false},
		{"foo", caseInsensitive, "foo", true},
	}
	for _, tc := range cases {
		got, insensitive, err := translateVim(tc.pattern, tc.mode)
		if err != nil {
			t.Errorf("translateVim(%q) error = %v", tc.pattern, err)
			continue
		}
		if got != tc.want || insensitive != tc.insensitive {
			t.Errorf("translateVim(%q) = %q, %t; want %q, %t", tc.pattern, got, insensitive, tc.want, tc.insensitive)
		}
	}
}

func TestRegistersRotateAndAppend(t *testing.T) {
	store := newRegisterStore()
	store.store("one ", registerChar, 0, false)
	store.store("two ", registerChar, 0, false)
	if store.unnamed.Text != "two " {
		t.Fatalf("unnamed = %q, want two", store.unnamed.Text)
	}
	if store.numbered[0].Text != "two " || store.numbered[1].Text != "one " {
		t.Fatalf("numbered = %q, %q", store.numbered[0].Text, store.numbered[1].Text)
	}
	store.store("yank", registerChar, 0, true)
	if store.zero.Text != "yank" {
		t.Fatalf("zero = %q, want yank", store.zero.Text)
	}
	store.store("A", registerChar, 'a', true)
	store.store("B", registerChar, 'A', true)
	if value, _ := store.get('a'); value.Text != "AB" {
		t.Fatalf("named append = %q, want AB", value.Text)
	}
	store.store("discard", registerChar, '_', true)
	if store.unnamed.Text == "discard" {
		t.Fatal("black hole register changed the unnamed register")
	}
}

func TestDotRepeatChangeWord(t *testing.T) {
	ed := testEditor(t, "one two", 0, 0)
	v := New()
	feed(t, v, ed, "cwX<esc>w.")
	if got := strings.Join(ed.Buffer.Lines, "\n"); got != "X X" {
		t.Fatalf("buffer = %q, want X X", got)
	}
}

func TestYankDoesNotUpdateDot(t *testing.T) {
	ed := testEditor(t, "abc", 0, 0)
	v := New()
	feed(t, v, ed, "xyw.")
	if got := strings.Join(ed.Buffer.Lines, "\n"); got != "c" {
		t.Fatalf("buffer = %q, want c (dot should repeat x, not yank)", got)
	}
}

func TestClipboardReadCompletesPendingPaste(t *testing.T) {
	ed := testEditor(t, "x", 0, 0)
	v := New()
	results := feed(t, v, ed, `"+p`)
	var readAction *Action
	for index := len(results) - 1; index >= 0; index-- {
		if len(results[index].Actions) > 0 {
			readAction = &results[index].Actions[0]
			break
		}
	}
	if readAction == nil || readAction.Kind != ActionClipboardRead {
		t.Fatalf("actions = %+v, want a clipboard read", readAction)
	}
	result := v.CompleteClipboardRead("one\n", nil, ed)
	if result.Notify != "" {
		t.Fatalf("CompleteClipboardRead notify = %q", result.Notify)
	}
	if got := strings.Join(ed.Buffer.Lines, "\n"); got != "x\none" {
		t.Fatalf("buffer = %q, want x\\none", got)
	}
}

func TestJumpListTruncatesForwardBranch(t *testing.T) {
	ed := testEditor(t, "a\nb\nc\nd\ne", 0, 0)
	v := New()
	feed(t, v, ed, "G<c-o>")
	if ed.Cursor.Pos.Line != 0 {
		t.Fatalf("after ctrl-o line = %d, want 0", ed.Cursor.Pos.Line)
	}
	feed(t, v, ed, "<c-i>")
	if ed.Cursor.Pos.Line != 4 {
		t.Fatalf("after ctrl-i line = %d, want 4", ed.Cursor.Pos.Line)
	}
}

func TestExCommandPrefixAndAmbiguity(t *testing.T) {
	if got, ok := resolveExCommand("w"); !ok || got != "write" {
		t.Fatalf("resolve w = %q, %t", got, ok)
	}
	if got, ok := resolveExCommand("bu"); !ok || got != "buffer" {
		t.Fatalf("resolve bu = %q, %t", got, ok)
	}
	if _, ok := resolveExCommand("tab"); ok {
		t.Fatal("resolve tab should be ambiguous")
	}
}

func TestSubstituteRequiresOneUndoEntry(t *testing.T) {
	ed := testEditor(t, "old old", 0, 0)
	v := New()
	feed(t, v, ed, ":%s/old/new/g<enter>")
	if len(ed.History.Undo) != 1 {
		t.Fatalf("undo entries = %d, want 1", len(ed.History.Undo))
	}
	ed.Undo()
	if got := strings.Join(ed.Buffer.Lines, "\n"); got != "old old" {
		t.Fatalf("after undo = %q", got)
	}
}

func TestSearchDoesNotSplitVS16(t *testing.T) {
	// U+2764 U+FE0F is one P8SCII cluster for the heart glyph.
	ed := testEditor(t, "a\u2764\ufe0fb", 0, 0)
	v := New()
	v.Handle(input.Event{Kind: input.KindKeyDown, Key: input.KeySlash}, ed)
	v.Handle(input.Event{Kind: input.KindText, Runes: []rune("/")}, ed)
	v.Handle(input.Event{Kind: input.KindText, Runes: []rune("\u2764")}, ed)
	v.Handle(input.Event{Kind: input.KindKeyDown, Key: input.KeyEnter}, ed)
	if got := ed.Cursor.Pos; got != (editor.Pos{Line: 0, Rune: 1}) {
		t.Fatalf("search cursor = %+v, want the cluster start", got)
	}
	if got := ed.Buffer.NormalizePos(ed.Cursor.Pos); got != ed.Cursor.Pos {
		t.Fatalf("cursor %+v is not on a cluster boundary", ed.Cursor.Pos)
	}
}

func BenchmarkKeystroke64K(b *testing.B) {
	lines := make([]string, 8000)
	for index := range lines {
		lines[index] = "print(\"hello world\")"
	}
	buffer := editor.New("bench.lua")
	buffer.Lines = lines
	buffer.MarkSaved()
	ed := editor.NewEditor(buffer)
	v := New()
	event, _ := keyEventForRune('j')
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.Handle(event, ed)
	}
}

func TestClipboardPastePinnedToOriginalEditor(t *testing.T) {
	first := testEditor(t, "a", 0, 0)
	second := testEditor(t, "b", 0, 0)
	v := New()
	results := feed(t, v, first, `"+p`)
	foundRead := false
	for _, result := range results {
		for _, action := range result.Actions {
			if action.Kind == ActionClipboardRead {
				foundRead = true
			}
		}
	}
	if !foundRead {
		t.Fatal("clipboard paste did not request a read")
	}
	v.CompleteClipboardRead("one\n", nil, second)
	if got := strings.Join(first.Buffer.Lines, "\n"); got != "a\none" {
		t.Fatalf("original buffer = %q, want a\\none", got)
	}
	if got := strings.Join(second.Buffer.Lines, "\n"); got != "b" {
		t.Fatalf("current buffer = %q, want unchanged b", got)
	}
}

func TestSecondClipboardPasteIsRejectedWhilePending(t *testing.T) {
	ed := testEditor(t, "a", 0, 0)
	v := New()
	feed(t, v, ed, `"+p`)
	v.Handle(input.Event{Kind: input.KindKeyDown, Key: input.KeyApostrophe, Mods: input.ModShift}, ed)
	v.Handle(input.Event{Kind: input.KindKeyDown, Key: input.KeyEqual, Mods: input.ModShift}, ed)
	result := v.Handle(input.Event{Kind: input.KindKeyDown, Key: input.KeyP}, ed)
	if !strings.Contains(result.Notify, "pending") {
		t.Fatalf("second paste notify = %q, want a pending notice", result.Notify)
	}
	v.CompleteClipboardRead("one\n", nil, ed)
	if got := strings.Join(ed.Buffer.Lines, "\n"); got != "a\none" {
		t.Fatalf("buffer = %q, want one paste", got)
	}
}

func TestBatchedTextSuppression(t *testing.T) {
	ed := testEditor(t, "abc", 0, 0)
	v := New()
	v.Handle(input.Event{Kind: input.KindKeyDown, Key: input.KeyI}, ed)
	v.Handle(input.Event{Kind: input.KindKeyDown, Key: input.KeyX}, ed)
	v.Handle(input.Event{Kind: input.KindText, Runes: []rune("ix")}, ed)
	if got := strings.Join(ed.Buffer.Lines, "\n"); got != "xabc" {
		t.Fatalf("buffer = %q, want xabc", got)
	}

	ed = testEditor(t, "abc", 0, 0)
	v = New()
	v.Handle(input.Event{Kind: input.KindKeyDown, Key: input.KeySemicolon, Mods: input.ModShift}, ed)
	v.Handle(input.Event{Kind: input.KindKeyDown, Key: input.KeyW}, ed)
	v.Handle(input.Event{Kind: input.KindText, Runes: []rune(":w")}, ed)
	_, text, _, _ := v.Prompt()
	if text != "w" {
		t.Fatalf("prompt = %q, want w", text)
	}
}

func TestClipboardPasteDoesNotDisturbInsertSession(t *testing.T) {
	ed := testEditor(t, "x", 0, 0)
	v := New()
	feed(t, v, ed, `"+p`)
	feed(t, v, ed, "iZ")
	if v.Mode != ModeInsert {
		t.Fatalf("mode = %s, want INSERT", v.Mode)
	}
	v.CompleteClipboardRead("P\n", nil, ed)
	if v.Mode != ModeInsert {
		t.Fatalf("mode after paste = %s, want INSERT", v.Mode)
	}
	if ed.Cursor.Pos != (editor.Pos{Line: 0, Rune: 1}) {
		t.Fatalf("cursor = %+v, want the insert cursor", ed.Cursor.Pos)
	}
	if got := strings.Join(ed.Buffer.Lines, "\n"); got != "Zx\nP" {
		t.Fatalf("buffer = %q, want Zx\\nP", got)
	}
}
