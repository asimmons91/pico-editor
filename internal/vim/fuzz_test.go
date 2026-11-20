package vim

import (
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/asimmons91/pico-editor/internal/input"
)

func FuzzHandle(f *testing.F) {
	seeds := []string{
		"dw", "dd", "cwX<esc>", "yyjp", "vld", "<c-v>jIX<esc>",
		"/foo<enter>n", ":%s/old/new/g<enter>", ":help<enter>",
		"3d2w", "2Ggg", "f-;,", "diw", "dap", ">j", "~",
		"\"ayy\"Ap", "rZ", "J", "u<c-r>", "x.",
	}
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		ed := testEditor(t, "one two\nthree four\n\nfive", 0, 0)
		v := New()
		for _, b := range data {
			for _, event := range fuzzEvents(b) {
				result := v.Handle(event, ed)
				for _, action := range result.Actions {
					if action.Kind == ActionClipboardRead {
						v.CompleteClipboardRead("one\ntwo", nil, ed)
					}
				}
				if !utf8.ValidString(ed.Buffer.Text()) {
					t.Fatalf("buffer is not valid UTF-8 after byte %#x: %q", b, ed.Buffer.Text())
				}
				normalized := ed.Buffer.NormalizePos(ed.Cursor.Pos)
				if normalized != ed.Cursor.Pos {
					t.Fatalf("cursor %+v is not on a cluster boundary (normalized %+v)", ed.Cursor.Pos, normalized)
				}
				if ed.Cursor.Pos.Line < 0 || ed.Cursor.Pos.Line >= len(ed.Buffer.Lines) {
					t.Fatalf("cursor line %d outside buffer", ed.Cursor.Pos.Line)
				}
			}
		}
	})
}

func fuzzEvents(b byte) []input.Event {
	letters := "abcdefghijklmnopqrstuvwxyz"
	punctuation := []rune(":;,. /?[]{}\\|-_=+~`%#*^$!@&()<>\"'")
	special := []input.Key{
		input.KeyEsc, input.KeyEnter, input.KeyBackspace, input.KeyDelete,
		input.KeyUp, input.KeyDown, input.KeyLeft, input.KeyRight,
		input.KeyHome, input.KeyEnd, input.KeyTab, input.KeySpace,
	}
	switch b % 3 {
	case 0:
		r := rune(letters[int(b)%len(letters)])
		event, _ := keyEventForRune(r)
		text := input.Event{Kind: input.KindText, Runes: []rune{r}}
		return []input.Event{event, text}
	case 1:
		r := punctuation[int(b)%len(punctuation)]
		event, ok := keyEventForRune(r)
		if !ok {
			return nil
		}
		text := input.Event{Kind: input.KindText, Runes: []rune{r}}
		return []input.Event{event, text}
	default:
		key := special[int(b)%len(special)]
		mods := input.Mods(0)
		if key == input.KeyTab {
			mods = input.ModCtrl
		}
		return []input.Event{{Kind: input.KindKeyDown, Key: key, Mods: mods}}
	}
}

func TestRandomUndoRestoresBuffer(t *testing.T) {
	initial := "one two\nthree four\nfive"
	rng := rand.New(rand.NewSource(7))
	safe := []byte("hjklwbe0$GfFtT;,%{}HMLxXsdcypJ~><uiIaAvV")
	for iteration := 0; iteration < 300; iteration++ {
		ed := testEditor(t, initial, 0, 0)
		v := New()
		for step := 0; step < 40; step++ {
			b := safe[rng.Intn(len(safe))]
			event, ok := keyEventForRune(rune(b))
			if !ok {
				continue
			}
			v.Handle(event, ed)
			v.Handle(input.Event{Kind: input.KindText, Runes: []rune{rune(b)}}, ed)
			if v.Mode == ModeInsert {
				v.Handle(input.Event{Kind: input.KindText, Runes: []rune("z")}, ed)
				v.Handle(input.Event{Kind: input.KindKeyDown, Key: input.KeyEsc}, ed)
			}
		}
		if v.Mode == ModeInsert {
			v.Handle(input.Event{Kind: input.KindKeyDown, Key: input.KeyEsc}, ed)
		}
		ed.EndGroup()
		for ed.CanUndo() {
			ed.Undo()
		}
		if got := strings.Join(ed.Buffer.Lines, "\n"); got != initial {
			t.Fatalf("iteration %d: after undo-all = %q, want %q", iteration, got, initial)
		}
		if !utf8.ValidString(ed.Buffer.Text()) {
			t.Fatalf("iteration %d produced invalid UTF-8", iteration)
		}
	}
}
