package app

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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

type fakeClipboard struct {
	text  string
	err   error
	reads int
}

func (f *fakeClipboard) ReadText() (string, error) {
	f.reads++
	return f.text, f.err
}
func (f *fakeClipboard) WriteText(string) error { return nil }

type blockingClipboard struct {
	started chan struct{}
	release chan struct{}
}

func (b *blockingClipboard) ReadText() (string, error) {
	close(b.started)
	<-b.release
	return "ok", nil
}
func (b *blockingClipboard) WriteText(string) error { return nil }

var testClock = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

func newTestGame(t *testing.T) *Game {
	t.Helper()
	game := New(Config{
		Logger: slog.New(slog.DiscardHandler),
		Now:    func() time.Time { return testClock },
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
	finishPaste(t, game)
	if got := strings.Join(game.charset.Log, "\n"); !strings.Contains(got, `paste "a\nb"`) {
		t.Errorf("log = %q, want normalized pasted text", got)
	}

	game.clipboard = &fakeClipboard{err: errors.New("no clipboard")}
	source.events = []input.Event{{Kind: input.KindKeyDown, Key: input.KeyV, Mods: input.ModCtrl}}
	if err := game.Update(); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	finishPaste(t, game)
	if got := strings.Join(game.charset.Log, "\n"); !strings.Contains(got, "paste error") {
		t.Errorf("log = %q, want a paste error", got)
	}
}

func TestPasteResultPolledByUpdate(t *testing.T) {
	game := newTestGame(t)
	game.clipboardResults <- clipboardResult{text: "x\r\ny"}
	if err := game.Update(); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got := strings.Join(game.charset.Log, "\n"); !strings.Contains(got, `paste "x\ny"`) {
		t.Errorf("log = %q, want the polled paste", got)
	}
	if game.clipboardBusy {
		t.Error("clipboardBusy remained true after polling a result")
	}
}

func TestPasteDoesNotBlockUpdate(t *testing.T) {
	game := newTestGame(t)
	blocking := &blockingClipboard{started: make(chan struct{}), release: make(chan struct{})}
	game.clipboard = blocking
	source := game.source.(*fakeSource)
	source.events = []input.Event{{Kind: input.KindKeyDown, Key: input.KeyV, Mods: input.ModCtrl}}

	done := make(chan error, 1)
	go func() { done <- game.Update() }()
	select {
	case <-blocking.started:
	case <-time.After(time.Second):
		t.Fatal("clipboard read did not start")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Update() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Update blocked on the clipboard read")
	}

	close(blocking.release)
	if result := waitPasteResult(t, game); result.err != nil || result.text != "ok" {
		t.Fatalf("paste result = %+v, want ok", result)
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

func finishPaste(t *testing.T, game *Game) {
	t.Helper()
	result := waitPasteResult(t, game)
	game.clipboardResults <- result
	if err := game.Update(); err != nil {
		t.Fatalf("Update() after paste = %v", err)
	}
}

func waitPasteResult(t *testing.T, game *Game) clipboardResult {
	t.Helper()
	select {
	case result := <-game.clipboardResults:
		return result
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the clipboard result")
		return clipboardResult{}
	}
}

func TestRepeatDoesNotTriggerCommands(t *testing.T) {
	game := newTestGame(t)
	clipboard := &fakeClipboard{text: "x"}
	game.clipboard = clipboard
	source := game.source.(*fakeSource)
	source.events = []input.Event{
		{Kind: input.KindKeyDown, Key: input.KeyW, Repeat: true},
		{Kind: input.KindKeyDown, Key: input.KeyP, Repeat: true},
		{Kind: input.KindKeyDown, Key: input.Key1, Repeat: true},
		{Kind: input.KindKeyDown, Key: input.KeyV, Mods: input.ModCtrl, Repeat: true},
		{Kind: input.KindKeyDown, Key: input.KeyQ, Mods: input.ModCtrl, Repeat: true},
	}
	if err := game.Update(); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if game.charset.Overlay {
		t.Error("repeated W toggled the overlay")
	}
	if game.charset.Background != 0 {
		t.Error("repeated P cycled the background")
	}
	if game.scale != DefaultScale {
		t.Error("repeated 1 changed the scale")
	}
	if clipboard.reads != 0 {
		t.Errorf("repeated Ctrl+V read the clipboard %d times", clipboard.reads)
	}
}

func TestMouseMoveDoesNotRedraw(t *testing.T) {
	game := newTestGame(t)
	game.renderer.dirty = false
	source := game.source.(*fakeSource)
	source.events = []input.Event{{Kind: input.KindMouse, X: 10, Y: 20}}
	if err := game.Update(); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if game.renderer.dirty {
		t.Error("mouse movement marked the surface dirty")
	}
}

func TestUpdateRedrawsOnVisibleChange(t *testing.T) {
	game := newTestGame(t)
	game.renderer.dirty = false
	source := game.source.(*fakeSource)
	source.events = []input.Event{{Kind: input.KindText, Runes: []rune("x")}}
	if err := game.Update(); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !game.renderer.dirty {
		t.Error("text input did not redraw the charset surface")
	}
}

func TestBlinkThroughUpdate(t *testing.T) {
	current := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	game := New(Config{
		Logger: slog.New(slog.DiscardHandler),
		Now:    func() time.Time { return current },
	})
	game.source = &fakeSource{}
	if !game.charset.CursorVisible {
		t.Fatal("cursor is not visible at startup")
	}

	current = current.Add(BlinkInterval)
	if err := game.Update(); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if game.charset.CursorVisible {
		t.Fatal("cursor did not hide after one blink interval")
	}

	current = current.Add(time.Millisecond)
	game.source.(*fakeSource).events = []input.Event{{Kind: input.KindText, Runes: []rune("x")}}
	if err := game.Update(); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !game.charset.CursorVisible {
		t.Fatal("input did not reset the cursor to visible")
	}
}

func TestScaleKeysResizeWindow(t *testing.T) {
	game := newTestGame(t)
	source := game.source.(*fakeSource)
	for digit, key := range []input.Key{input.Key1, input.Key2, input.Key3, input.Key4} {
		source.events = []input.Event{{Kind: input.KindKeyDown, Key: key}}
		if err := game.Update(); err != nil {
			t.Fatalf("Update() error = %v", err)
		}
		want := digit + 1
		width, height := ebiten.WindowSize()
		if width != CanvasWidth*want || height != CanvasHeight*want {
			t.Errorf("WindowSize after %v = %dx%d, want %dx%d", key, width, height, CanvasWidth*want, CanvasHeight*want)
		}
	}
}

func TestLogPanelDoesNotCoverBottomSelection(t *testing.T) {
	charset := NewCharset()
	charset.Background = 1
	charset.Log = []string{"hello"}
	charset.Selected = 0xF0
	surface := render.NewSurface(CanvasWidth, CanvasHeight)
	charset.Draw(surface)
	if got := surface.At(1, 232); got != 1 {
		t.Errorf("bottom background = %d, want 1 (log panel hidden)", got)
	}
	if got := surface.At(0, 240); got != 10 {
		t.Errorf("selected bottom tile border = %d, want 10", got)
	}

	charset.Selected = 0x41
	surface = render.NewSurface(CanvasWidth, CanvasHeight)
	charset.Draw(surface)
	if got := surface.At(1, 232); got != 0 {
		t.Errorf("log panel background = %d, want 0", got)
	}
}

func TestWidthLabels(t *testing.T) {
	cases := []struct {
		width int
		pix   string
		cells string
	}{
		{0, "0", "0"},
		{4, "4", "1"},
		{8, "8", "2"},
	}
	for _, tc := range cases {
		pix, cells := widthLabels(tc.width)
		if pix != tc.pix || cells != tc.cells {
			t.Errorf("widthLabels(%d) = (%q, %q), want (%q, %q)", tc.width, pix, cells, tc.pix, tc.cells)
		}
	}
}

func TestOverlayKeepsTileBorder(t *testing.T) {
	charset := NewCharset()
	charset.Background = 1
	charset.Overlay = true
	surface := render.NewSurface(CanvasWidth, CanvasHeight)
	charset.Draw(surface)
	if got := surface.At(15, 2); got != 5 {
		t.Errorf("right border = %d, want 5", got)
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		text  string
		limit int
		want  string
	}{
		{"abc", 3, `"abc"`},
		{"abcd", 3, `"abc"...`},
		{"héllo", 2, `"hé"...`},
		{"abc", 0, `""...`},
		{"", 0, `""`},
	}
	for _, tc := range cases {
		if got := truncate(tc.text, tc.limit); got != tc.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.text, tc.limit, got, tc.want)
		}
	}
}

func TestPastePendingWhileBusy(t *testing.T) {
	game := newTestGame(t)
	blocking := &blockingClipboard{started: make(chan struct{}), release: make(chan struct{})}
	game.clipboard = blocking
	source := game.source.(*fakeSource)
	source.events = []input.Event{{Kind: input.KindKeyDown, Key: input.KeyV, Mods: input.ModCtrl}}
	if err := game.Update(); err != nil {
		t.Fatalf("first Update() error = %v", err)
	}
	select {
	case <-blocking.started:
	case <-time.After(time.Second):
		t.Fatal("clipboard read did not start")
	}

	source.events = []input.Event{{Kind: input.KindKeyDown, Key: input.KeyV, Mods: input.ModCtrl}}
	if err := game.Update(); err != nil {
		t.Fatalf("second Update() error = %v", err)
	}
	if got := strings.Join(game.charset.Log, "\n"); !strings.Contains(got, "paste pending") {
		t.Errorf("log = %q, want a pending paste", got)
	}

	close(blocking.release)
	finishPaste(t, game)
	if got := strings.Join(game.charset.Log, "\n"); !strings.Contains(got, "paste \"ok\"") {
		t.Errorf("log = %q, want the completed paste", got)
	}
}

func TestPastePollRedraws(t *testing.T) {
	game := newTestGame(t)
	game.renderer.dirty = false
	game.clipboardResults <- clipboardResult{text: "x"}
	if err := game.Update(); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !game.renderer.dirty {
		t.Error("polled paste did not redraw the surface")
	}
}

func TestPasteTimeout(t *testing.T) {
	game := newTestGame(t)
	game.clipboardBusy = true
	game.clipboardStarted = testClock.Add(-pasteTimeout - time.Second)
	game.clipboardGeneration = 1
	if !game.pollPaste(testClock) {
		t.Fatal("pollPaste did not report the timeout")
	}
	if game.clipboardBusy {
		t.Fatal("clipboardBusy remained true after timeout")
	}
	if got := strings.Join(game.charset.Log, "\n"); !strings.Contains(got, "paste timed out") {
		t.Errorf("log = %q, want a timeout", got)
	}
}

func TestPasteTypedUnavailable(t *testing.T) {
	game := newTestGame(t)
	game.clipboard = &fakeClipboard{err: fmt.Errorf("%w: no display", input.ErrUnavailable)}
	source := game.source.(*fakeSource)
	source.events = []input.Event{{Kind: input.KindKeyDown, Key: input.KeyV, Mods: input.ModCtrl}}
	if err := game.Update(); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	finishPaste(t, game)
	if got := strings.Join(game.charset.Log, "\n"); !strings.Contains(got, "clipboard unavailable") {
		t.Errorf("log = %q, want the typed unavailable message", got)
	}
}

type panicClipboard struct{}

func (panicClipboard) ReadText() (string, error) { panic("boom") }
func (panicClipboard) WriteText(string) error    { return nil }

func TestPastePanicRecovered(t *testing.T) {
	game := newTestGame(t)
	game.clipboard = panicClipboard{}
	source := game.source.(*fakeSource)
	source.events = []input.Event{{Kind: input.KindKeyDown, Key: input.KeyV, Mods: input.ModCtrl}}
	if err := game.Update(); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	finishPaste(t, game)
	if got := strings.Join(game.charset.Log, "\n"); !strings.Contains(got, "clipboard panic") {
		t.Errorf("log = %q, want the recovered panic", got)
	}
}

func TestRepeatArrowMovesSelection(t *testing.T) {
	game := newTestGame(t)
	source := game.source.(*fakeSource)
	source.events = []input.Event{
		{Kind: input.KindKeyDown, Key: input.KeyRight, Repeat: true},
		{Kind: input.KindKeyDown, Key: input.KeyDown, Repeat: true},
	}
	if err := game.Update(); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if want := byte(0x41 + 1 + 16); game.charset.Selected != want {
		t.Errorf("selected = %#02x, want %#02x", game.charset.Selected, want)
	}
}

func TestPreviewPrefix(t *testing.T) {
	if got := previewPrefix("abc", 10); got != "abc" {
		t.Errorf("short prefix = %q, want abc", got)
	}
	if got := previewPrefix("abc", 0); got != "" {
		t.Errorf("zero prefix = %q, want empty", got)
	}
	long := strings.Repeat("é", 3000)
	got := previewPrefix(long, 100)
	if len(got) > 100 {
		t.Errorf("prefix length = %d, want <= 100", len(got))
	}
	if !utf8.ValidString(got) {
		t.Error("prefix split a UTF-8 sequence")
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
