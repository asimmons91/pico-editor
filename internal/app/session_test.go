package app

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/asimmons91/pico-editor/internal/editor"
	"github.com/asimmons91/pico-editor/internal/input"
	"github.com/asimmons91/pico-editor/internal/render"
	"github.com/asimmons91/pico-editor/internal/vim"
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

func TestTabCommandsReportNoCartTabs(t *testing.T) {
	game, _ := newEditorGame(t, "abc\n", &recordingClipboard{})
	if err := sendKeys(t, game, ":tabn<enter>"); err != nil {
		t.Fatalf("Update tabn: %v", err)
	}
	if !strings.Contains(game.session.Status, "E85") {
		t.Fatalf("status = %q, want E85 for a .lua buffer", game.session.Status)
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
	case "c-space":
		key, mods = input.KeySpace, input.ModCtrl
	case "c-n":
		key, mods = input.KeyN, input.ModCtrl
	case "c-S-n":
		key, mods = input.KeyN, input.ModCtrl|input.ModShift
	case "c-]":
		key, mods = input.KeyRightBracket, input.ModCtrl
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

func TestMultiBufferQuitAndCanQuitAll(t *testing.T) {
	game, _ := newEditorGame(t, "one\n", &recordingClipboard{})
	second := filepath.Join(filepath.Dir(game.session.CurrentEditor().Buffer.Path), "second.lua")
	if err := os.WriteFile(second, []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sendKeys(t, game, ":e second.lua<enter>x"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if game.session.CanQuitAll() {
		t.Fatal("CanQuitAll returned true with a dirty buffer")
	}
	if quit, _ := game.session.RequestQuit(false); quit || !strings.Contains(game.session.Status, "E37") {
		t.Fatalf("dirty RequestQuit = quit %t status %q", quit, game.session.Status)
	}
	if quit, _ := game.session.RequestQuit(true); quit || len(game.session.Buffers) != 1 {
		t.Fatalf("force RequestQuit = quit %t buffers %d", quit, len(game.session.Buffers))
	}
	if !game.session.CanQuitAll() {
		t.Fatal("CanQuitAll returned false after discarding the dirty buffer")
	}
	if quit, _ := game.session.RequestQuit(false); !quit || !game.session.Quitting() {
		t.Fatal("last clean RequestQuit did not quit")
	}
}

func TestCtrlQRefusesDirtySessionThenForceQuits(t *testing.T) {
	game, _ := newEditorGame(t, "abc\n", &recordingClipboard{})
	if err := sendKeys(t, game, "x"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	game.source.(*fakeSource).events = []input.Event{{Kind: input.KindKeyDown, Key: input.KeyQ, Mods: input.ModCtrl}}
	if err := game.Update(); err != nil {
		t.Fatalf("Update ctrl+q: %v", err)
	}
	if game.session.Dialog == nil || game.session.Dialog.Kind != DialogQuit {
		t.Fatalf("dialog = %+v, want quit dialog", game.session.Dialog)
	}
	if err := sendKeys(t, game, "c"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if err := sendKeys(t, game, ":q!<enter>"); err != ebiten.Termination {
		t.Fatalf("force quit = %v, want Termination", err)
	}
}

func TestAsyncSaveFailureAndEditDuringSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.lua")
	if err := os.WriteFile(path, []byte("abc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	session, err := NewSession(path, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	ed := session.CurrentEditor()
	if _, err := ed.Insert("X"); err != nil {
		t.Fatal(err)
	}
	job, err := session.PrepareSave(vim.Action{Kind: vim.ActionSave, QuitAfter: true})
	if err != nil {
		t.Fatalf("PrepareSave: %v", err)
	}
	quit, _ := session.CompleteSave(job, errors.New("disk full"))
	if quit {
		t.Fatal("failed save quit the session")
	}
	if !ed.Buffer.Dirty() {
		t.Fatal("buffer became clean after a failed save")
	}
	if !session.StatusError || session.Status == "" {
		t.Fatalf("status = %q error=%t, want the save error", session.Status, session.StatusError)
	}

	job, err = session.PrepareSave(vim.Action{Kind: vim.ActionSave, QuitAfter: true})
	if err != nil {
		t.Fatalf("PrepareSave: %v", err)
	}
	if err := job.Snapshot.Write(); err != nil {
		t.Fatalf("snapshot write: %v", err)
	}
	if _, err := ed.Insert("Y"); err != nil {
		t.Fatal(err)
	}
	quit, _ = session.CompleteSave(job, nil)
	if quit {
		t.Fatal("save-quit exited while the buffer had newer edits")
	}
	if !ed.Buffer.Dirty() {
		t.Fatal("buffer lost its newer edit during save commit")
	}
	if !strings.Contains(session.Status, "still has changes") {
		t.Fatalf("status = %q, want the still-dirty notice", session.Status)
	}
}

func TestCleanXQuitsWithoutWriting(t *testing.T) {
	game, path := newEditorGame(t, "abc\n", &recordingClipboard{})
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := sendKeys(t, game, ":x<enter>"); err != ebiten.Termination {
		t.Fatalf(":x = %v, want Termination", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("clean :x changed the file from %q to %q", before, after)
	}
}

func TestClipboardFailureIsSurfaced(t *testing.T) {
	clipboard := &recordingClipboard{err: errors.New("no clipboard")}
	game, _ := newEditorGame(t, "x\n", clipboard)
	if err := sendKeys(t, game, "\"+p"); err != nil {
		t.Fatalf("Update paste: %v", err)
	}
	eventually(t, func() bool {
		_ = game.Update()
		return strings.Contains(game.session.Status, "clipboard unavailable")
	})
	if got := strings.Join(game.session.CurrentEditor().Buffer.Lines, "\n"); got != "x" {
		t.Fatalf("buffer = %q, want unchanged", got)
	}

	game, _ = newEditorGame(t, "x\n", clipboard)
	if err := sendKeys(t, game, "\"+yy"); err != nil {
		t.Fatalf("Update yank: %v", err)
	}
	eventually(t, func() bool {
		_ = game.Update()
		return strings.Contains(game.session.Status, "clipboard:")
	})
}

func TestSelectionAndSearchBackgroundsArePainted(t *testing.T) {
	game, _ := newEditorGame(t, "   \n   \n", &recordingClipboard{})
	base := render.NewSurface(CanvasWidth, CanvasHeight)
	game.session.Draw(base, true)
	if countPalette(base, selectionBackground) != 0 {
		t.Fatal("baseline surface already contains selection pixels")
	}
	if err := sendKeys(t, game, "V"); err != nil {
		t.Fatalf("Update V: %v", err)
	}
	selected := render.NewSurface(CanvasWidth, CanvasHeight)
	game.session.Draw(selected, true)
	if countPalette(selected, selectionBackground) == 0 {
		t.Fatal("visual line selection painted no background")
	}

	game, _ = newEditorGame(t, "one two\n", &recordingClipboard{})
	base = render.NewSurface(CanvasWidth, CanvasHeight)
	game.session.Draw(base, true)
	if countPalette(base, searchBackground) != 0 {
		t.Fatal("baseline surface already contains search pixels")
	}
	if err := sendKeys(t, game, "/two<enter>"); err != nil {
		t.Fatalf("Update search: %v", err)
	}
	searched := render.NewSurface(CanvasWidth, CanvasHeight)
	game.session.Draw(searched, true)
	if countPalette(searched, searchBackground) == 0 {
		t.Fatal("search matches painted no background")
	}
}

func countPalette(surface *render.Surface, index uint8) int {
	count := 0
	for _, pixel := range surface.Pix {
		if pixel == index {
			count++
		}
	}
	return count
}

func TestWindowCloseRefusesDirtyBuffers(t *testing.T) {
	game, _ := newEditorGame(t, "one\n", &recordingClipboard{})
	second := filepath.Join(filepath.Dir(game.session.CurrentEditor().Buffer.Path), "second.lua")
	if err := os.WriteFile(second, []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sendKeys(t, game, ":e second.lua<enter>x"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	quit, changed := game.handleWindowClose(true)
	if quit || !changed || game.session.Dialog == nil || game.session.Dialog.Kind != DialogQuit {
		t.Fatalf("dirty close = quit %t changed %t dialog %+v", quit, changed, game.session.Dialog)
	}
	if err := sendKeys(t, game, "c"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if err := sendKeys(t, game, ":q!<enter>:q!<enter>"); err != ebiten.Termination {
		t.Fatalf("force quits = %v, want Termination", err)
	}
}

func TestClusterSelectionAndSearchPredicates(t *testing.T) {
	ed := editor.NewEditor(editor.New(""))
	ed.Buffer.Lines = []string{"abcd", "efgh"}
	ed.Buffer.MarkSaved()
	ed.Selection = editor.Selection{Anchor: editor.Pos{Line: 0, Rune: 1}, Cursor: editor.Pos{Line: 0, Rune: 2}, Kind: editor.SelectionChar}
	if !clusterSelected(ed, 0, 1) || clusterSelected(ed, 0, 0) {
		t.Fatal("character selection predicate is wrong")
	}
	ed.Selection = editor.Selection{Anchor: editor.Pos{Line: 0, Rune: 3}, Cursor: editor.Pos{Line: 1, Rune: 0}, Kind: editor.SelectionLine}
	if !clusterSelected(ed, 1, 2) {
		t.Fatal("line selection predicate is wrong")
	}
	ed.Selection = editor.Selection{Anchor: editor.Pos{Line: 0, Rune: 1}, Cursor: editor.Pos{Line: 0, Rune: 2}, Kind: editor.SelectionBlock}
	if !clusterSelected(ed, 0, 1) || clusterSelected(ed, 0, 3) {
		t.Fatal("block selection predicate is wrong")
	}
	matches := []editor.Match{{Start: editor.Pos{Line: 0, Rune: 1}, End: editor.Pos{Line: 0, Rune: 3}}}
	if !clusterSearched(matches, 0, 1) || clusterSearched(matches, 0, 3) {
		t.Fatal("search predicate is wrong")
	}
}

func TestSaveQuitClosesSavedBufferNotCurrent(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a.lua")
	pathB := filepath.Join(dir, "b.lua")
	if err := os.WriteFile(pathA, []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pathB, []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	session, err := NewSession(pathA, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if session.OpenPath(pathB, false) != true {
		t.Fatal("failed to open second buffer")
	}
	if _, err := session.Buffers[1].Editor.Insert("Y"); err != nil {
		t.Fatal(err)
	}
	job, err := session.PrepareSave(vim.Action{Kind: vim.ActionSave, QuitAfter: true})
	if err != nil {
		t.Fatalf("PrepareSave: %v", err)
	}
	if err := job.Snapshot.Write(); err != nil {
		t.Fatalf("snapshot write: %v", err)
	}
	if !session.SwitchBuffer(1, 0) {
		t.Fatal("failed to switch back to the first buffer")
	}
	if _, err := session.Buffers[0].Editor.Insert("X"); err != nil {
		t.Fatal(err)
	}
	quit, _ := session.CompleteSave(job, nil)
	if quit {
		t.Fatal("save-quit exited while another buffer was current")
	}
	if len(session.Buffers) != 1 || session.CurrentEditor().Buffer.Path != pathA {
		t.Fatalf("buffers = %d current = %q, want only a.lua", len(session.Buffers), session.CurrentEditor().Buffer.Path)
	}
	if !session.CurrentEditor().Buffer.Dirty() {
		t.Fatal("the unrelated current buffer lost its dirty state")
	}
}

func TestPendingQuitWaitsForSave(t *testing.T) {
	game, _ := newEditorGame(t, "abc\n", &recordingClipboard{})
	game.saveBusy = true
	game.pendingQuit = true
	if err := game.Update(); err != nil {
		t.Fatalf("Update while saving = %v, want nil", err)
	}
	game.saveBusy = false
	if err := game.Update(); err != ebiten.Termination {
		t.Fatalf("Update after save = %v, want Termination", err)
	}
}

func TestDebugCharsetWindowCloseStillWorks(t *testing.T) {
	game := New(Config{Logger: slog.New(slog.DiscardHandler), DebugCharset: true})
	if quit, _ := game.handleWindowClose(true); !quit {
		t.Fatal("debug charset close request did not quit")
	}
}

func TestClipboardCRLFIsNormalizedOnPaste(t *testing.T) {
	clipboard := &recordingClipboard{text: "a\r\nb\r\n"}
	game, _ := newEditorGame(t, "x\n", clipboard)
	if err := sendKeys(t, game, "\"+p"); err != nil {
		t.Fatalf("Update paste: %v", err)
	}
	eventually(t, func() bool {
		_ = game.Update()
		return strings.Join(game.session.CurrentEditor().Buffer.Lines, "\n") == "x\na\nb"
	})
	for _, line := range game.session.CurrentEditor().Buffer.Lines {
		if strings.ContainsRune(line, '\r') {
			t.Fatalf("pasted line contains CR: %q", line)
		}
	}
}

func TestForceEditReloadsAlreadyOpenPath(t *testing.T) {
	game, path := newEditorGame(t, "abc\n", &recordingClipboard{})
	if err := sendKeys(t, game, "iX<esc>"); err != nil {
		t.Fatalf("Update edit: %v", err)
	}
	if err := os.WriteFile(path, []byte("disk\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sendKeys(t, game, ":e! test.lua<enter>"); err != nil {
		t.Fatalf("Update e!: %v", err)
	}
	if got := game.session.CurrentEditor().Buffer.Text(); got != "disk\n" {
		t.Fatalf("buffer = %q, want disk", got)
	}
	if game.session.CurrentEditor().Buffer.Dirty() {
		t.Fatal("forced reload left the buffer dirty")
	}
}
