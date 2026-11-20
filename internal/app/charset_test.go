package app

import (
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/asimmons91/pico-editor/internal/input"
)

type fakeSource struct {
	events []input.Event
}

func (f *fakeSource) Poll() []input.Event {
	out := f.events
	f.events = nil
	return out
}

type fakeClipboard struct {
	text string
	err  error
}

func (f *fakeClipboard) ReadText() (string, error) { return f.text, f.err }
func (f *fakeClipboard) WriteText(string) error    { return nil }

func newTestGame(t *testing.T) *Game {
	t.Helper()
	game := New(Config{Logger: slog.New(slog.DiscardHandler)})
	game.source = &fakeSource{}
	return game
}

func TestCharsetScreenIsDrawnAtStartup(t *testing.T) {
	game := newTestGame(t)
	ink := 0
	for _, pixel := range game.renderer.Surface.Pix {
		if pixel != 0 {
			ink++
		}
	}
	if ink == 0 {
		t.Fatal("startup surface is blank")
	}
	if game.scale != DefaultScale {
		t.Errorf("scale = %d, want %d", game.scale, DefaultScale)
	}
}

func TestCharsetInputControls(t *testing.T) {
	game := newTestGame(t)
	source := game.source.(*fakeSource)

	source.events = []input.Event{
		{Kind: input.KindKeyDown, Key: input.KeyW},
		{Kind: input.KindKeyDown, Key: input.KeyP},
		{Kind: input.KindKeyDown, Key: input.KeyRight},
		{Kind: input.KindKeyDown, Key: input.KeyDown},
	}
	if err := game.Update(); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !game.charset.Overlay {
		t.Error("W did not enable the width overlay")
	}
	if game.charset.Background != 1 {
		t.Errorf("background = %d, want 1", game.charset.Background)
	}
	if game.charset.Selected != 0x41+1+16 {
		t.Errorf("selected = %#02x, want %#02x", game.charset.Selected, 0x41+1+16)
	}
	if len(game.charset.Log) == 0 {
		t.Error("input events were not logged")
	}
}

func TestCharsetTextAndRepeatLogs(t *testing.T) {
	game := newTestGame(t)
	source := game.source.(*fakeSource)
	source.events = []input.Event{
		{Kind: input.KindText, Runes: []rune("hé")},
		{Kind: input.KindKeyDown, Key: input.KeyJ, Mods: input.ModShift, Repeat: true},
	}
	if err := game.Update(); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	joined := strings.Join(game.charset.Log, "\n")
	if !strings.Contains(joined, `text "hé"`) {
		t.Errorf("log = %q, want the committed text", joined)
	}
	if !strings.Contains(joined, "key shift+j repeat") {
		t.Errorf("log = %q, want the shifted repeat", joined)
	}
}

func TestCharsetPaste(t *testing.T) {
	game := newTestGame(t)
	game.clipboard = &fakeClipboard{text: "a\r\nb"}
	source := game.source.(*fakeSource)
	source.events = []input.Event{{Kind: input.KindKeyDown, Key: input.KeyV, Mods: input.ModCtrl}}
	if err := game.Update(); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got := strings.Join(game.charset.Log, "\n"); !strings.Contains(got, `paste "a\nb"`) {
		t.Errorf("log = %q, want normalized pasted text", got)
	}

	game.clipboard = &fakeClipboard{err: errors.New("no clipboard")}
	source.events = []input.Event{{Kind: input.KindKeyDown, Key: input.KeyV, Mods: input.ModCtrl}}
	if err := game.Update(); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got := strings.Join(game.charset.Log, "\n"); !strings.Contains(got, "paste error") {
		t.Errorf("log = %q, want a paste error", got)
	}
}

func TestCtrlQQuits(t *testing.T) {
	game := newTestGame(t)
	source := game.source.(*fakeSource)
	source.events = []input.Event{{Kind: input.KindKeyDown, Key: input.KeyQ, Mods: input.ModCtrl}}
	if err := game.Update(); err != ebiten.Termination {
		t.Fatalf("Update() error = %v, want ebiten.Termination", err)
	}
}

func TestScaleKeys(t *testing.T) {
	game := newTestGame(t)
	source := game.source.(*fakeSource)
	for digit, key := range []input.Key{input.Key1, input.Key2, input.Key3, input.Key4} {
		source.events = []input.Event{{Kind: input.KindKeyDown, Key: key}}
		if err := game.Update(); err != nil {
			t.Fatalf("Update() error = %v", err)
		}
		want := digit + 1
		if game.scale != want {
			t.Errorf("scale after %v = %d, want %d", key, game.scale, want)
		}
	}
}

func TestFocusAndMouseEvents(t *testing.T) {
	game := newTestGame(t)
	source := game.source.(*fakeSource)
	source.events = []input.Event{
		{Kind: input.KindFocus, Focused: false},
		{Kind: input.KindMouse, X: 10, Y: 20, Buttons: input.MouseLeft},
	}
	if err := game.Update(); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got := strings.Join(game.charset.Log, "\n"); !strings.Contains(got, "focus false") {
		t.Errorf("log = %q, want the focus event", got)
	}
}

func TestDebugCharsetStartsWithOverlay(t *testing.T) {
	game := New(Config{Logger: slog.New(slog.DiscardHandler), DebugCharset: true})
	if !game.charset.Overlay {
		t.Error("DebugCharset did not start with the width overlay")
	}
}
