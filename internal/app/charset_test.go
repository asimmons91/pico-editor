package app

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/asimmons91/pico-editor/internal/font"
	"github.com/asimmons91/pico-editor/internal/input"
	"github.com/asimmons91/pico-editor/internal/render"
)

type fakeSource struct {
	events []input.Event
}

func (f *fakeSource) Poll() []input.Event {
	out := f.events
	f.events = nil
	return out
}

var testClock = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

func newTestGame(t *testing.T) *Game {
	t.Helper()
	game := New(Config{
		Logger:       slog.New(slog.DiscardHandler),
		Now:          func() time.Time { return testClock },
		DebugCharset: true,
	})
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

func TestLogHideBoundary(t *testing.T) {
	cases := []struct {
		selected byte
		wantRow  uint8
	}{
		{0xdf, 0},
		{0xe0, 1},
	}
	for _, tc := range cases {
		charset := NewCharset()
		charset.Background = 1
		charset.Log = []string{"hello"}
		charset.Selected = tc.selected
		surface := render.NewSurface(CanvasWidth, CanvasHeight)
		charset.Draw(surface)
		if got := surface.At(1, 232); got != tc.wantRow {
			t.Errorf("selected %#02x: At(0,232) = %d, want %d", tc.selected, got, tc.wantRow)
		}
	}
}

func TestOverlayCellLabelAndBottomBorder(t *testing.T) {
	charset := NewCharset()
	charset.Background = 1
	charset.Overlay = true
	surface := render.NewSurface(CanvasWidth, CanvasHeight)
	charset.Draw(surface)

	want := render.NewSurface(4, font.CellHeight)
	want.DrawText(0, 0, "0", 10, 1)
	for y := 0; y < font.CellHeight; y++ {
		for x := 0; x < 3; x++ {
			if got, expected := surface.At(12+x, 9+y), want.At(x, y); got != expected {
				t.Errorf("cell label At(%d,%d) = %d, want %d", 12+x, 9+y, got, expected)
			}
		}
	}
	if got := surface.At(15, 11); got != 5 {
		t.Errorf("cell-label right border = %d, want 5", got)
	}
	if got := surface.At(12, 15); got != 5 {
		t.Errorf("bottom border = %d, want 5", got)
	}
}

func TestOverlayLabelsWidthTiles(t *testing.T) {
	charset := NewCharset()
	charset.Background = 1
	charset.Overlay = true
	surface := render.NewSurface(CanvasWidth, CanvasHeight)
	charset.Draw(surface)

	cases := []struct {
		byte  byte
		pixel string
		cell  string
	}{
		{0x41, "4", "1"},
		{0x80, "8", "2"},
	}
	for _, tc := range cases {
		x := int(tc.byte) % charsetTiles * charsetTile
		y := int(tc.byte) / charsetTiles * charsetTile
		assertOverlayLabel(t, surface, x+12, y+2, tc.pixel)
		assertOverlayLabel(t, surface, x+12, y+9, tc.cell)
	}
}

func assertOverlayLabel(t *testing.T, surface *render.Surface, x, y int, label string) {
	t.Helper()
	want := render.NewSurface(4, font.CellHeight)
	want.DrawText(0, 0, label, 10, 1)
	for row := 0; row < font.CellHeight; row++ {
		for col := 0; col < 3; col++ {
			if got, expected := surface.At(x+col, y+row), want.At(col, row); got != expected {
				t.Errorf("label %q At(%d,%d) = %d, want %d", label, x+col, y+row, got, expected)
			}
		}
	}
}
