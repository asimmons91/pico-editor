package input

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/asimmons91/pico-lsp/pkg/p8scii"
)

func TestP8ShiftLetterTable(t *testing.T) {
	in := NewP8Input(nil)
	want := []string{
		"█", "▒", "🐱", "⬇️", "░", "✽", "●", "♥", "☉", "웃", "⌂", "⬅️", "😐",
		"♪", "🅾️", "◆", "…", "➡️", "★", "⧗", "⬆️", "ˇ", "∧", "❎", "▤", "▥",
	}
	for i, key := range allLetterKeys() {
		if handled := in.HandleKey(key, ModShift); handled {
			t.Fatalf("%s: HandleKey consumed the key", KeyName(key))
		}
		text, handled := in.TransformText([]rune{rune('A' + i)}, ModShift)
		if handled {
			t.Fatalf("%s: TransformText consumed the glyph", KeyName(key))
		}
		if got := string(text); got != want[i] {
			t.Errorf("%s = %q, want %q", KeyName(key), got, want[i])
		}
	}
}

func TestP8ShiftLetterSurvivesShiftKeydownOrder(t *testing.T) {
	in := NewP8Input(nil)
	in.HandleKey(KeyA, ModShift)
	// Ebitengine may drain the physical Shift keydown after the letter.
	in.HandleKey(KeyShift, ModShift)
	text, handled := in.TransformText([]rune("A"), ModShift)
	if handled || string(text) != "█" {
		t.Fatalf("text = %q handled=%t, want █", string(text), handled)
	}
}

func TestP8BatchedShiftLetters(t *testing.T) {
	in := NewP8Input(nil)
	in.HandleKey(KeyA, ModShift)
	in.HandleKey(KeyB, ModShift)
	in.HandleKey(KeyShift, ModShift)
	text, handled := in.TransformText([]rune("AB"), ModShift)
	if handled || string(text) != "█▒" {
		t.Fatalf("text = %q handled=%t, want █▒", string(text), handled)
	}
}

func TestP8BatchedKanaShiftDigits(t *testing.T) {
	in := NewP8Input(nil)
	in.HandleKey(KeyJ, ModCtrl)
	in.HandleKey(Key1, ModShift)
	in.HandleKey(Key2, ModShift)
	text, handled := in.TransformText([]rune("!@"), ModShift)
	if handled || string(text) != "▮■" {
		t.Fatalf("text = %q handled=%t, want ▮■", string(text), handled)
	}
}

func TestP8ShiftLetterKeepsNonUSLayout(t *testing.T) {
	in := NewP8Input(nil)
	in.HandleKey(KeyA, ModShift)
	text, handled := in.TransformText([]rune("Ä"), ModShift)
	if handled || string(text) != "Ä" {
		t.Fatalf("fallback text = %q handled=%t, want Ä", string(text), handled)
	}
}

func TestP8NormalAndVisualStateUntouched(t *testing.T) {
	// P8Input has no mode changes for unshifted keys and does not synthesize
	// glyphs without a Shift+letter key.
	in := NewP8Input(nil)
	if in.HandleKey(KeyJ, 0) {
		t.Fatal("plain J was consumed")
	}
	if in.HandleKey(KeyP, 0) {
		t.Fatal("plain P was consumed")
	}
	if text, handled := in.TransformText([]rune("J"), 0); handled || string(text) != "J" {
		t.Fatalf("plain J text = %q handled=%t", string(text), handled)
	}
}

func TestP8ModeToggleRepeatDoesNotFlip(t *testing.T) {
	in := NewP8Input(nil)
	in.HandleKey(KeyJ, ModCtrl)
	if in.Mode != P8ModeHiragana {
		t.Fatalf("mode = %v", in.Mode)
	}
	if !in.HandleRepeat(KeyJ, ModCtrl) {
		t.Fatal("repeat was not consumed")
	}
	if in.Mode != P8ModeHiragana {
		t.Fatalf("repeat toggled mode to %v", in.Mode)
	}
}

func TestP8ModeTogglesAreExclusive(t *testing.T) {
	in := NewP8Input(nil)
	if !in.HandleKey(KeyJ, ModCtrl) || in.Mode != P8ModeHiragana {
		t.Fatalf("Ctrl+J mode = %v", in.Mode)
	}
	if in.ModeLabel() != "HIRA" {
		t.Fatalf("label = %q, want HIRA", in.ModeLabel())
	}
	if !in.HandleKey(KeyK, ModCtrl) || in.Mode != P8ModeKatakana {
		t.Fatalf("Ctrl+K mode = %v", in.Mode)
	}
	if in.ModeLabel() != "KATA" {
		t.Fatalf("label = %q, want KATA", in.ModeLabel())
	}
	if !in.HandleKey(KeyP, ModCtrl) || in.Mode != P8ModePuny {
		t.Fatalf("Ctrl+P mode = %v", in.Mode)
	}
	if in.ModeLabel() != "PUNY" {
		t.Fatalf("label = %q, want PUNY", in.ModeLabel())
	}
	if !in.HandleKey(KeyJ, ModCtrl) || in.Mode != P8ModeHiragana {
		t.Fatalf("Ctrl+J from puny mode = %v", in.Mode)
	}
	if !in.HandleKey(KeyJ, ModCtrl) || in.Mode != P8ModeDefault {
		t.Fatalf("second Ctrl+J did not turn the mode off: %v", in.Mode)
	}
	in.Reset()
	if in.Mode != P8ModeDefault || in.Pending != "" {
		t.Fatalf("Reset left mode=%v pending=%q", in.Mode, in.Pending)
	}
}

func TestP8PunyText(t *testing.T) {
	in := NewP8Input(nil)
	in.HandleKey(KeyP, ModCtrl)
	text, handled := in.TransformText([]rune("my_var"), 0)
	if handled || string(text) != "MY_VAR" {
		t.Fatalf("puny unshifted = %q handled=%t, want MY_VAR", string(text), handled)
	}
	text, handled = in.TransformText([]rune("MY_VAR"), ModShift)
	if handled || string(text) != "my_var" {
		t.Fatalf("puny shifted = %q handled=%t, want my_var", string(text), handled)
	}
	// Puny mode never converts Shift+letter into a glyph.
	in.HandleKey(KeyA, ModShift)
	text, handled = in.TransformText([]rune("A"), ModShift)
	if handled || string(text) != "a" {
		t.Fatalf("puny Shift+A = %q handled=%t, want a", string(text), handled)
	}
}

func TestP8KanaSequences(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"a", "あ"},
		{"ka", "か"},
		{"kya", "きゃ"},
		{"ga", "か゛"},
		{"shi", "し"},
		{"sha", "しゃ"},
		{"ja", "し゛ゃ"},
		{"cha", "ちゃ"},
		{"tsu", "つ"},
		{"fu", "ふ"},
		{"nya", "にゃ"},
		{"nn", "ん"},
		{"nna", "んあ"},
		{"kka", "っか"},
		{"kkya", "っきゃ"},
		{"xtsu", "っ"},
		{"xya", "ゃ"},
		{"ltsu", "っ"},
		{"lya", "ゃ"},
	}
	for _, tc := range tests {
		in := NewP8Input(nil)
		in.HandleKey(KeyJ, ModCtrl)
		text, handled := in.TransformText([]rune(tc.input), 0)
		if handled {
			t.Fatalf("%q was dropped", tc.input)
		}
		if got := string(text); got != tc.want {
			t.Errorf("%q = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestP8KanaInvalidSequences(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"nka", "あ"},
		{"nta", "あ"},
		{"tya", "あ"},
		{"wya", "あ"},
		{"fa", ""},
		{"wi", ""},
		{"we", ""},
		{"xa", ""},
		{"la", ""},
	}
	for _, tc := range tests {
		in := NewP8Input(nil)
		in.HandleKey(KeyJ, ModCtrl)
		text, handled := in.TransformText([]rune(tc.input), 0)
		if got := string(text); got != tc.want {
			t.Errorf("%q = %q handled=%t, want %q", tc.input, got, handled, tc.want)
		}
	}
}

func TestP8KanaPendingAcrossTextEvents(t *testing.T) {
	in := NewP8Input(nil)
	in.HandleKey(KeyJ, ModCtrl)
	text, handled := in.TransformText([]rune("k"), 0)
	if !handled || len(text) != 0 || in.Pending != "k" {
		t.Fatalf("pending k = %q handled=%t pending=%q", string(text), handled, in.Pending)
	}
	text, handled = in.TransformText([]rune("a"), 0)
	if handled || string(text) != "か" || in.Pending != "" {
		t.Fatalf("ka = %q handled=%t pending=%q", string(text), handled, in.Pending)
	}
}

func TestP8KanaNonLetterFlushesPending(t *testing.T) {
	in := NewP8Input(nil)
	in.HandleKey(KeyJ, ModCtrl)
	in.TransformText([]rune("k"), 0)
	text, handled := in.TransformText([]rune(" "), 0)
	if handled || string(text) != " " || in.Pending != "" {
		t.Fatalf("space = %q handled=%t pending=%q", string(text), handled, in.Pending)
	}
}

func TestP8KanaShiftDigits(t *testing.T) {
	hira := NewP8Input(nil)
	hira.HandleKey(KeyJ, ModCtrl)
	wantHira := []string{"▮", "■", "□", "⁙", "⁘", "‖", "◀", "▶", "◜", "◝"}
	for i, key := range []Key{Key1, Key2, Key3, Key4, Key5, Key6, Key7, Key8, Key9, Key0} {
		hira.HandleKey(key, ModShift)
		text, _ := hira.TransformText([]rune{'!'}, ModShift)
		if got := string(text); got != wantHira[i] {
			t.Errorf("hiragana Shift+%d = %q, want %q", (i+1)%10, got, wantHira[i])
		}
	}

	kata := NewP8Input(nil)
	kata.HandleKey(KeyK, ModCtrl)
	wantKata := wantHira
	for i, key := range []Key{Key1, Key2, Key3, Key4, Key5, Key6, Key7, Key8, Key9, Key0} {
		kata.HandleKey(key, ModShift)
		text, _ := kata.TransformText([]rune{'!'}, ModShift)
		if got := string(text); got != wantKata[i] {
			t.Errorf("katakana Shift+%d = %q, want %q", (i+1)%10, got, wantKata[i])
		}
	}
}

func TestP8KanaCoversEveryByte(t *testing.T) {
	seen := make(map[byte]bool, 100)
	for _, text := range kanaTable {
		for _, b := range p8scii.Encode(text) {
			if b >= 0x9a && b <= 0xcb {
				seen[b] = true
			}
		}
	}
	for b := 0x9a; b <= 0xcb; b++ {
		if !seen[byte(b)] {
			t.Errorf("hiragana byte 0x%02x is not produced by any romaji sequence", b)
		}
	}

	kataSeen := make(map[byte]bool, 100)
	for _, text := range kanaTable {
		in := NewP8Input(nil)
		in.Mode = P8ModeKatakana
		for _, b := range p8scii.Encode(in.kanaModeText(text)) {
			if b >= 0xcc && b <= 0xfd {
				kataSeen[b] = true
			}
		}
	}
	for b := 0xcc; b <= 0xfd; b++ {
		if !kataSeen[byte(b)] {
			t.Errorf("katakana byte 0x%02x is not produced by any romaji sequence", b)
		}
	}
}

func TestP8KanaMatchesCapture(t *testing.T) {
	tests := []struct {
		name    string
		mode    P8Mode
		keys    string
		capture string
	}{
		{"hiragana", P8ModeHiragana, "../../testdata/pico8/input/hiragana-keys.txt", "../../testdata/pico8/input/hiragana-capture.txt"},
		{"katakana", P8ModeKatakana, "../../testdata/pico8/input/katakana-keys.txt", "../../testdata/pico8/input/katakana-capture.txt"},
		{"gemination", P8ModeHiragana, "../../testdata/pico8/input/gemination-keys.txt", "../../testdata/pico8/input/gemination-capture.txt"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			keyData, err := os.ReadFile(tc.keys)
			if err != nil {
				t.Fatal(err)
			}
			captureData, err := os.ReadFile(tc.capture)
			if err != nil {
				t.Fatal(err)
			}
			keys := strings.Split(strings.TrimRight(string(keyData), "\n"), "\n")
			capture := strings.Split(string(captureData), "\n")
			if index := slices.Index(capture, "__lua__"); index >= 0 {
				capture = capture[index+1:]
			}
			if len(capture) < len(keys) {
				t.Fatalf("capture has %d lines, want at least %d", len(capture), len(keys))
			}
			for i, key := range keys {
				in := NewP8Input(nil)
				in.Mode = tc.mode
				text, _ := in.TransformText([]rune(key), 0)
				got := strings.TrimSpace(string(text))
				want := strings.TrimSpace(capture[i])
				if got != want {
					t.Errorf("%q = %q, capture says %q", key, got, want)
				}
			}
		})
	}
}

func TestP8KanaDigitsMatchCapture(t *testing.T) {
	keyData, err := os.ReadFile("../../testdata/pico8/input/shift-digits-keys.txt")
	if err != nil {
		t.Fatal(err)
	}
	captureData, err := os.ReadFile("../../testdata/pico8/input/shift-digits-capture.txt")
	if err != nil {
		t.Fatal(err)
	}
	keys := strings.Split(strings.TrimRight(string(keyData), "\n"), "\n")
	capture := strings.Split(string(captureData), "\n")
	if index := slices.Index(capture, "__lua__"); index >= 0 {
		capture = capture[index+1:]
	}
	for _, mode := range []P8Mode{P8ModeHiragana, P8ModeKatakana} {
		for i, key := range keys {
			if len(key) != 1 || key[0] < '0' || key[0] > '9' {
				continue
			}
			in := NewP8Input(nil)
			in.Mode = mode
			in.HandleKey(Key0+Key(key[0]-'0'), ModShift)
			text, _ := in.TransformText([]rune{'!'}, ModShift)
			got := strings.TrimSpace(string(text))
			want := strings.TrimSpace(capture[i])
			if got != want {
				t.Errorf("mode %v Shift+%s = %q, capture says %q", mode, key, got, want)
			}
		}
	}
}

func TestP8KanaNoDuplicateSequences(t *testing.T) {
	for key, text := range kanaTable {
		if strings.TrimSpace(key) != key || key == "" {
			t.Errorf("invalid sequence %q", key)
		}
		if text == "" {
			t.Errorf("sequence %q has empty text", key)
		}
	}
}

func allLetterKeys() []Key {
	keys := make([]Key, 0, 26)
	for key := KeyA; key <= KeyZ; key++ {
		keys = append(keys, key)
	}
	return keys
}
