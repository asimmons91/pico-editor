package app

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/asimmons91/pico-editor/internal/input"
)

type recordingClipboard struct {
	mu    sync.Mutex
	text  string
	reads int
	err   error
}

func (c *recordingClipboard) ReadText() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reads++
	return c.text, c.err
}

func (c *recordingClipboard) WriteText(text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	c.text = text
	return nil
}

func (c *recordingClipboard) Text() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.text
}

func (c *recordingClipboard) Reads() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reads
}

func newEditorGame(t *testing.T, content string, clipboard input.Clipboard) (*Game, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.lua")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	game := New(Config{
		Logger:    slog.New(slog.DiscardHandler),
		Clipboard: clipboard,
		Now:       func() time.Time { return testClock },
		File:      path,
	})
	if err := game.StartupError(); err != nil {
		t.Fatalf("StartupError: %v", err)
	}
	game.source = &fakeSource{}
	return game, path
}

func sendKeys(t *testing.T, game *Game, keys string) error {
	t.Helper()
	game.source.(*fakeSource).events = appParseKeys(t, keys)
	return game.Update()
}

func eventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("condition did not become true")
}

func TestNoFileStartsEmptyNormalBuffer(t *testing.T) {
	game := New(Config{Logger: slog.New(slog.DiscardHandler)})
	if game.StartupError() != nil {
		t.Fatalf("StartupError: %v", game.StartupError())
	}
	if game.session == nil {
		t.Fatal("no file did not create an editor session")
	}
	if got := game.session.Vim.Mode; got != 0 {
		t.Fatalf("mode = %v, want NORMAL", got)
	}
	if got := game.session.CurrentEditor().Buffer.Text(); got != "" {
		t.Fatalf("buffer = %q, want empty", got)
	}
}

func TestNormalModeShiftCommandIsNotInserted(t *testing.T) {
	game, _ := newEditorGame(t, "abc\n", &recordingClipboard{})
	if err := sendKeys(t, game, "D"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := game.session.CurrentEditor().Buffer.Lines[0]; got != "" {
		t.Fatalf("line = %q, want empty after D", got)
	}
}

func TestInsertAndAsyncSave(t *testing.T) {
	game, path := newEditorGame(t, "abc\n", &recordingClipboard{})
	if err := sendKeys(t, game, "iX<esc>"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := game.session.CurrentEditor().Buffer.Text(); got != "Xabc\n" {
		t.Fatalf("buffer = %q, want Xabc", got)
	}
	if err := sendKeys(t, game, ":w<enter>"); err != nil {
		t.Fatalf("Update write: %v", err)
	}
	eventually(t, func() bool {
		if err := game.Update(); err != nil {
			t.Fatalf("Update poll: %v", err)
		}
		return !game.session.CurrentEditor().Buffer.Dirty()
	})
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(data) != "Xabc\n" {
		t.Fatalf("saved file = %q, want Xabc", data)
	}
}

func TestQuitRefusesDirtyAndForceQuits(t *testing.T) {
	game, _ := newEditorGame(t, "abc\n", &recordingClipboard{})
	if err := sendKeys(t, game, "x:q<enter>"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !strings.Contains(game.session.Status, "E37") {
		t.Fatalf("status = %q, want E37", game.session.Status)
	}
	if game.session.Quitting() {
		t.Fatal("session quit despite unsaved changes")
	}
	if err := sendKeys(t, game, ":q!<enter>"); err != ebiten.Termination {
		t.Fatalf("Update quit = %v, want Termination", err)
	}
}

func TestClipboardWriteAndReadRegisters(t *testing.T) {
	clipboard := &recordingClipboard{}
	game, _ := newEditorGame(t, "one\n", clipboard)
	if err := sendKeys(t, game, "\"+yy"); err != nil {
		t.Fatalf("Update yank: %v", err)
	}
	eventually(t, func() bool {
		_ = game.Update()
		return clipboard.Text() == "one\n"
	})

	game, _ = newEditorGame(t, "x\n", clipboard)
	if err := sendKeys(t, game, "\"+p"); err != nil {
		t.Fatalf("Update paste: %v", err)
	}
	eventually(t, func() bool {
		_ = game.Update()
		return strings.Join(game.session.CurrentEditor().Buffer.Lines, "\n") == "x\none"
	})
	if clipboard.Reads() == 0 {
		t.Error("clipboard paste did not read the system clipboard")
	}
}

func TestMultiBufferOpenListAndSwitch(t *testing.T) {
	game, _ := newEditorGame(t, "one\n", &recordingClipboard{})
	second := filepath.Join(filepath.Dir(game.session.CurrentEditor().Buffer.Path), "second.lua")
	if err := os.WriteFile(second, []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sendKeys(t, game, ":e second.lua<enter>"); err != nil {
		t.Fatalf("Update edit: %v", err)
	}
	if len(game.session.Buffers) != 2 {
		t.Fatalf("buffers = %d, want 2", len(game.session.Buffers))
	}
	if got := game.session.CurrentEditor().Buffer.Text(); got != "two\n" {
		t.Fatalf("current buffer = %q, want two", got)
	}
	if err := sendKeys(t, game, ":bn<enter>"); err != nil {
		t.Fatalf("Update bn: %v", err)
	}
	if got := game.session.CurrentEditor().Buffer.Text(); got != "one\n" {
		t.Fatalf("after :bn = %q, want one", got)
	}
	if err := sendKeys(t, game, ":b 2<enter>"); err != nil {
		t.Fatalf("Update b 2: %v", err)
	}
	if got := game.session.CurrentEditor().Buffer.Text(); got != "two\n" {
		t.Fatalf("after :b 2 = %q, want two", got)
	}
	if err := sendKeys(t, game, ":ls<enter>"); err != nil {
		t.Fatalf("Update ls: %v", err)
	}
	if game.session.Help == nil || game.session.Help.Title != "Buffers" {
		t.Fatalf("buffer list overlay = %+v", game.session.Help)
	}
	if err := sendKeys(t, game, "<esc>"); err != nil {
		t.Fatalf("Update esc: %v", err)
	}
	if game.session.Help != nil {
		t.Fatal("escape did not close the buffer list")
	}
}

func TestHelpAndCommandLinePrompt(t *testing.T) {
	game, _ := newEditorGame(t, "abc\n", &recordingClipboard{})
	if err := sendKeys(t, game, ":help input<enter>"); err != nil {
		t.Fatalf("Update help: %v", err)
	}
	if game.session.Help == nil || !strings.Contains(game.session.Help.Title, "input") {
		t.Fatalf("help overlay = %+v", game.session.Help)
	}
	if err := sendKeys(t, game, "<esc>"); err != nil {
		t.Fatalf("Update esc: %v", err)
	}
	if err := sendKeys(t, game, ":"); err != nil {
		t.Fatalf("Update colon: %v", err)
	}
	if prefix, _, _, active := game.session.Vim.Prompt(); !active || prefix != ":" {
		t.Fatalf("prompt = %q active=%t, want colon prompt", prefix, active)
	}
	if err := sendKeys(t, game, "<esc>"); err != nil {
		t.Fatalf("Update esc prompt: %v", err)
	}
}

func TestTabCommandsReportSingleTab(t *testing.T) {
	game, _ := newEditorGame(t, "abc\n", &recordingClipboard{})
	if err := sendKeys(t, game, ":tabn<enter>"); err != nil {
		t.Fatalf("Update tabn: %v", err)
	}
	if !strings.Contains(game.session.Status, "M4") {
		t.Fatalf("status = %q, want the M4 single-tab message", game.session.Status)
	}
}

func TestEditorSurfaceDrawsChromeAndCursor(t *testing.T) {
	game, _ := newEditorGame(t, "abc\n", &recordingClipboard{})
	ink := 0
	for _, pixel := range game.renderer.Surface.Pix {
		if pixel != 0 {
			ink++
		}
	}
	if ink == 0 {
		t.Fatal("editor surface is blank")
	}
}

func TestSessionHelpScrollAndClose(t *testing.T) {
	game, _ := newEditorGame(t, "abc\n", &recordingClipboard{})
	if err := sendKeys(t, game, ":help<enter><down><up><esc>"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if game.session.Help != nil {
		t.Fatal("help did not close")
	}
}

// appParseKeys is the input-event parser used by the app tests. It mirrors the
// normalized events produced by internal/input: printable keys emit a key
// event and a committed-text event.
func appParseKeys(t *testing.T, keys string) []input.Event {
	t.Helper()
	var events []input.Event
	for index := 0; index < len(keys); {
		if keys[index] == '<' {
			if end := strings.IndexByte(keys[index:], '>'); end > 0 {
				name := strings.ToLower(keys[index+1 : index+end])
				if event, ok := appNamedKey(name); ok {
					events = append(events, event)
					index += end + 1
					continue
				}
			}
		}
		r, size := appDecodeRune(keys[index:])
		event, ok := appKeyForRune(r)
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

func appNamedKey(name string) (input.Event, bool) {
	var key input.Key
	mods := input.Mods(0)
	switch name {
	case "esc":
		key = input.KeyEsc
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
	case "c-r":
		key, mods = input.KeyR, input.ModCtrl
	case "c-o":
		key, mods = input.KeyO, input.ModCtrl
	case "c-i":
		key, mods = input.KeyI, input.ModCtrl
	case "c-v":
		key, mods = input.KeyV, input.ModCtrl
	default:
		return input.Event{}, false
	}
	return input.Event{Kind: input.KindKeyDown, Key: key, Mods: mods}, true
}

func appKeyForRune(r rune) (input.Event, bool) {
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

func appDecodeRune(text string) (rune, int) {
	for _, r := range text {
		return r, len(string(r))
	}
	return 0, 0
}

func TestWriteQuitWaitsForAsyncSave(t *testing.T) {
	game, path := newEditorGame(t, "abc\n", &recordingClipboard{})
	if err := sendKeys(t, game, "iX<esc>:wq<enter>"); err != nil {
		t.Fatalf("Update write-quit: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		err := game.Update()
		if err == ebiten.Termination {
			break
		}
		if err != nil {
			t.Fatalf("Update poll: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatal("write-quit did not terminate")
		}
		time.Sleep(2 * time.Millisecond)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(data) != "Xabc\n" {
		t.Fatalf("saved file = %q, want Xabc", data)
	}
}
