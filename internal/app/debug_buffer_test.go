package app

import (
	"bytes"
	"errors"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/asimmons91/pico-editor/internal/editor"
	"github.com/asimmons91/pico-editor/internal/font"
	"github.com/asimmons91/pico-editor/internal/input"
	"github.com/asimmons91/pico-editor/internal/render"
)

func newDebugGame(t *testing.T, content string) (*Game, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.lua")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	game := New(Config{
		Logger: slog.New(slog.DiscardHandler),
		Now:    func() time.Time { return testClock },
		File:   path,
	})
	if err := game.StartupError(); err != nil {
		t.Fatalf("StartupError: %v", err)
	}
	if game.debug == nil {
		t.Fatal("file argument did not create a debug buffer")
	}
	game.source = &fakeSource{}
	return game, path
}

func TestDebugBufferOpensFile(t *testing.T) {
	t.Parallel()

	game, _ := newDebugGame(t, "print(1)\n")
	if got := game.debug.Editor.Buffer.Text(); got != "print(1)\n" {
		t.Errorf("buffer text = %q", got)
	}
	ink := 0
	for _, pixel := range game.renderer.Surface.Pix {
		if pixel != 0 {
			ink++
		}
	}
	if ink == 0 {
		t.Error("debug screen rendered a blank surface")
	}
}

func TestDebugBufferRejectsCart(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "cart.p8")
	if err := os.WriteFile(path, []byte("__lua__\nprint(1)\n"), 0o644); err != nil {
		t.Fatalf("write cart: %v", err)
	}
	game := New(Config{Logger: slog.New(slog.DiscardHandler), File: path})
	if !errors.Is(game.StartupError(), editor.ErrUnsupportedFile) {
		t.Fatalf("StartupError = %v, want ErrUnsupportedFile", game.StartupError())
	}
}

func TestDebugBufferTextAndSave(t *testing.T) {
	t.Parallel()

	game, path := newDebugGame(t, "print(1)\n")
	source := game.source.(*fakeSource)
	source.events = []input.Event{{Kind: input.KindText, Runes: []rune("-- hello\n")}}
	if err := game.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := game.debug.Editor.Buffer.Text(); got != "-- hello\nprint(1)\n" {
		t.Errorf("text = %q", got)
	}
	if !game.debug.Editor.Buffer.Dirty() {
		t.Error("buffer is not dirty after typing")
	}

	source.events = []input.Event{{Kind: input.KindKeyDown, Key: input.KeyS, Mods: input.ModCtrl}}
	if err := game.Update(); err != nil {
		t.Fatalf("Update save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(data) != "-- hello\nprint(1)\n" {
		t.Errorf("saved file = %q", data)
	}
	if game.debug.Editor.Buffer.Dirty() {
		t.Error("buffer is dirty after save")
	}
}

func TestDebugBufferUndoRedoKeys(t *testing.T) {
	t.Parallel()

	game, _ := newDebugGame(t, "one\n")
	source := game.source.(*fakeSource)
	source.events = []input.Event{{Kind: input.KindText, Runes: []rune("two")}}
	if err := game.Update(); err != nil {
		t.Fatalf("Update insert: %v", err)
	}
	source.events = []input.Event{{Kind: input.KindKeyDown, Key: input.KeyZ, Mods: input.ModCtrl}}
	if err := game.Update(); err != nil {
		t.Fatalf("Update undo: %v", err)
	}
	if got := game.debug.Editor.Buffer.Text(); got != "one\n" {
		t.Errorf("after undo text = %q", got)
	}
	source.events = []input.Event{{Kind: input.KindKeyDown, Key: input.KeyY, Mods: input.ModCtrl}}
	if err := game.Update(); err != nil {
		t.Fatalf("Update redo: %v", err)
	}
	if got := game.debug.Editor.Buffer.Text(); got != "twoone\n" {
		t.Errorf("after redo text = %q", got)
	}
}

func TestDebugBufferArrowSelectionAndReplacement(t *testing.T) {
	t.Parallel()

	game, _ := newDebugGame(t, "abc\n")
	source := game.source.(*fakeSource)
	source.events = []input.Event{
		{Kind: input.KindKeyDown, Key: input.KeyRight, Mods: input.ModShift},
	}
	if err := game.Update(); err != nil {
		t.Fatalf("Update selection: %v", err)
	}
	if got := game.debug.Editor.Selection.Kind; got != editor.SelectionChar {
		t.Errorf("selection kind = %v, want char", got)
	}
	if text, ok := game.debug.Editor.SelectedText(); !ok || text != "ab" {
		t.Errorf("selected text = %q, %t, want ab", text, ok)
	}
	source.events = []input.Event{{Kind: input.KindText, Runes: []rune("X")}}
	if err := game.Update(); err != nil {
		t.Fatalf("Update replace: %v", err)
	}
	if got := game.debug.Editor.Buffer.Text(); got != "Xc\n" {
		t.Errorf("replacement text = %q, want Xc\\n", got)
	}
}

func TestDebugBufferCtrlQQuits(t *testing.T) {
	t.Parallel()

	game, _ := newDebugGame(t, "print(1)\n")
	source := game.source.(*fakeSource)
	source.events = []input.Event{{Kind: input.KindKeyDown, Key: input.KeyQ, Mods: input.ModCtrl}}
	if err := game.Update(); !errors.Is(err, ebiten.Termination) {
		t.Fatalf("Update = %v, want ebiten.Termination", err)
	}
}

func TestDebugBufferPaste(t *testing.T) {
	t.Parallel()

	game, _ := newDebugGame(t, "\n")
	clipboard := &fakeClipboard{text: "one\r\ntwo"}
	game.clipboard = clipboard
	source := game.source.(*fakeSource)
	source.events = []input.Event{{Kind: input.KindKeyDown, Key: input.KeyV, Mods: input.ModCtrl}}
	if err := game.Update(); err != nil {
		t.Fatalf("Update paste request: %v", err)
	}
	finishPaste(t, game)
	if got := game.debug.Editor.Buffer.Text(); got != "one\ntwo\n" {
		t.Errorf("pasted text = %q, want canonical LF text", got)
	}
}

func TestDebugBufferGolden(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.lua")
	content := "print(\"♥⬅️😐️\")\n\n\tlocal value = 42\nfunction _init()\n\treturn value\nend\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	buffer, err := editor.Open(path)
	if err != nil {
		t.Fatalf("Open fixture: %v", err)
	}
	buffer.Path = "fixture.lua"

	edit := editor.NewEditor(buffer)
	edit.Viewport.Width = GridColumns
	edit.Viewport.Height = debugRows
	edit.Viewport.ScrollOff = 2
	edit.SetCursor(editor.Pos{Line: 2, Rune: 7})
	edit.Selection = editor.Selection{
		Anchor: editor.Pos{Line: 0},
		Cursor: editor.Pos{Line: 0, Rune: 10},
		Kind:   editor.SelectionChar,
	}
	debug := &DebugBuffer{Editor: edit, Status: "saved"}

	surface := render.NewSurface(CanvasWidth, CanvasHeight)
	debug.Draw(surface, true)
	img := scaleSurface(surface, 4)

	var encoded bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	if err := encoder.Encode(&encoded, img); err != nil {
		t.Fatalf("encode golden: %v", err)
	}
	goldenPath := filepath.Join("testdata", "debug_4x.png")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(goldenPath, encoded.Bytes(), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run UPDATE_GOLDEN=1 go test ./internal/app): %v", err)
	}
	if !bytes.Equal(encoded.Bytes(), want) {
		t.Error("debug_4x.png is stale; run UPDATE_GOLDEN=1 go test ./internal/app")
	}
}

func TestDebugStatusIncludesError(t *testing.T) {
	t.Parallel()

	game, _ := newDebugGame(t, "print(1)\n")
	game.debug.SetStatus("save failed", true)
	if !strings.Contains(game.debug.Status, "save failed") || !game.debug.ErrorStatus {
		t.Errorf("status = %q error=%t", game.debug.Status, game.debug.ErrorStatus)
	}
}

func TestDebugEditingKeys(t *testing.T) {
	t.Parallel()

	key := func(k input.Key) input.Event {
		return input.Event{Kind: input.KindKeyDown, Key: k}
	}
	shiftKey := func(k input.Key) input.Event {
		return input.Event{Kind: input.KindKeyDown, Key: k, Mods: input.ModShift}
	}

	tests := []struct {
		name    string
		content string
		events  []input.Event
		want    string
		cursor  editor.Pos
	}{
		{
			name:    "backspace",
			content: "abc\n",
			events:  []input.Event{key(input.KeyEnd), key(input.KeyBackspace)},
			want:    "ab\n",
			cursor:  editor.Pos{Line: 0, Rune: 2},
		},
		{
			name:    "delete",
			content: "abc\n",
			events:  []input.Event{key(input.KeyRight), key(input.KeyDelete)},
			want:    "ac\n",
			cursor:  editor.Pos{Line: 0, Rune: 1},
		},
		{
			name:    "enter",
			content: "ab\n",
			events:  []input.Event{key(input.KeyEnd), key(input.KeyEnter)},
			want:    "ab\n\n",
			cursor:  editor.Pos{Line: 1, Rune: 0},
		},
		{
			name:    "tab",
			content: "a\n",
			events:  []input.Event{key(input.KeyEnd), key(input.KeyTab)},
			want:    "a \n",
			cursor:  editor.Pos{Line: 0, Rune: 2},
		},
		{
			name:    "backspace replaces selection",
			content: "abc\n",
			events:  []input.Event{shiftKey(input.KeyRight), key(input.KeyBackspace)},
			want:    "c\n",
			cursor:  editor.Pos{Line: 0, Rune: 0},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			game, _ := newDebugGame(t, test.content)
			source := game.source.(*fakeSource)
			source.events = test.events
			if err := game.Update(); err != nil {
				t.Fatalf("Update: %v", err)
			}
			if got := game.debug.Editor.Buffer.Text(); got != test.want {
				t.Errorf("text = %q, want %q", got, test.want)
			}
			if got := game.debug.Editor.Cursor.Pos; got != test.cursor {
				t.Errorf("cursor = %v, want %v", got, test.cursor)
			}
		})
	}
}

func TestDebugNavigationAndSelectAll(t *testing.T) {
	t.Parallel()

	game, _ := newDebugGame(t, "one\ntwo\nthree\n")
	source := game.source.(*fakeSource)
	handle := func(event input.Event) {
		t.Helper()
		source.events = []input.Event{event}
		if err := game.Update(); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}
	key := func(k input.Key) input.Event {
		return input.Event{Kind: input.KindKeyDown, Key: k}
	}

	handle(key(input.KeyEnd))
	if got := game.debug.Editor.Cursor.Pos; got != (editor.Pos{Line: 0, Rune: 3}) {
		t.Fatalf("End cursor = %v, want 0,3", got)
	}
	handle(key(input.KeyDown))
	if got := game.debug.Editor.Cursor.Pos; got != (editor.Pos{Line: 1, Rune: 3}) {
		t.Fatalf("Down cursor = %v, want 1,3", got)
	}
	handle(key(input.KeyHome))
	if got := game.debug.Editor.Cursor.Pos; got != (editor.Pos{Line: 1, Rune: 0}) {
		t.Fatalf("Home cursor = %v, want 1,0", got)
	}
	handle(key(input.KeyPageDown))
	if got := game.debug.Editor.Cursor.Pos; got != (editor.Pos{Line: 2, Rune: 0}) {
		t.Fatalf("PageDown cursor = %v, want the last line", got)
	}
	handle(input.Event{Kind: input.KindKeyDown, Key: input.KeyA, Mods: input.ModCtrl})
	if got := game.debug.Editor.Cursor.Pos; got != (editor.Pos{Line: 2, Rune: 5}) {
		t.Fatalf("Ctrl+A cursor = %v, want the buffer end", got)
	}
	if game.debug.Editor.Selection.Cursor != game.debug.Editor.Cursor.Pos {
		t.Fatal("Ctrl+A left the selection cursor and editor cursor out of sync")
	}
	handle(input.Event{Kind: input.KindKeyDown, Key: input.KeyLeft, Mods: input.ModShift})
	if got := game.debug.Editor.Selection.Cursor; got != (editor.Pos{Line: 2, Rune: 4}) {
		t.Errorf("Shift+Left selection cursor = %v, want 2,4", got)
	}
	handle(key(input.KeyEsc))
	if got := game.debug.Editor.Selection.Kind; got != editor.SelectionNone {
		t.Errorf("Esc selection kind = %v, want none", got)
	}
}

func TestDebugScrolledRender(t *testing.T) {
	t.Parallel()

	var content strings.Builder
	for i := 0; i < 100; i++ {
		content.WriteString("line\n")
	}
	game, _ := newDebugGame(t, content.String())
	game.debug.Editor.MoveBufferEnd()
	surface := render.NewSurface(CanvasWidth, CanvasHeight)
	game.debug.Draw(surface, true)
	if game.debug.Editor.Viewport.TopLine == 0 {
		t.Fatal("debug viewport did not scroll to the buffer end")
	}
	ink := func(y0, y1 int) int {
		count := 0
		for y := y0; y < y1; y++ {
			for x := 0; x < surface.Width; x++ {
				if surface.At(x, y) != 0 {
					count++
				}
			}
		}
		return count
	}
	if ink(0, font.CellHeight) == 0 {
		t.Error("first visible row is blank after scrolling")
	}
	if ink(debugStatusRow*font.CellHeight, CanvasHeight) == 0 {
		t.Error("status row is blank after scrolling")
	}
}
