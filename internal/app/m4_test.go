package app

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/asimmons91/pico-editor/internal/editor"
	"github.com/asimmons91/pico-editor/internal/input"
	"github.com/asimmons91/pico-editor/internal/vim"
)

const fullCart = "pico-8 cartridge // pico-8.com\n" +
	"version 42\n" +
	"__lua__\n" +
	"a=1\n" +
	"b=2\n" +
	"-->8 second\n" +
	"a=3\n" +
	"c=3\n" +
	"__gfx__\n" +
	"0000\n" +
	"__label__\n" +
	"label\n"

func newCartGame(t *testing.T, content string) (*Game, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "game.p8")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	game := New(Config{
		Logger: slog.New(slog.DiscardHandler),
		Now:    func() time.Time { return testClock },
		File:   path,
	})
	if err := game.StartupError(); err != nil {
		t.Fatalf("StartupError: %v", err)
	}
	game.source = &fakeSource{}
	return game, path
}

func waitClean(t *testing.T, game *Game, buffer *SessionBuffer) {
	t.Helper()
	eventually(t, func() bool {
		if err := game.Update(); err != nil && err != ebiten.Termination {
			t.Fatalf("Update poll: %v", err)
		}
		return !buffer.Editor.Buffer.Dirty()
	})
}

func TestCartOpenTabsAndSave(t *testing.T) {
	game, path := newCartGame(t, fullCart)
	buffer := game.session.CurrentBuffer()
	if buffer.Cart == nil {
		t.Fatal("cart did not open through the cart adapter")
	}
	if len(buffer.Cart.Tabs) != 2 {
		t.Fatalf("tabs = %+v, want 2", buffer.Cart.Tabs)
	}
	if got := buffer.Editor.Viewport.Height; got != cartCodeRows {
		t.Fatalf("cart viewport height = %d, want %d", got, cartCodeRows)
	}
	if err := sendKeys(t, game, "GiX<esc>"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := buffer.Editor.Cursor.Pos.Line; got != 1 {
		t.Fatalf("G landed on line %d, want the last tab-0 line", got)
	}
	if got := buffer.Editor.Buffer.Lines[1]; got != "Xb=2" {
		t.Fatalf("line = %q, want Xb=2", got)
	}
	if err := sendKeys(t, game, ":w<enter>"); err != nil {
		t.Fatalf("Update save: %v", err)
	}
	waitClean(t, game, buffer)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "Xb=2") {
		t.Fatalf("saved cart missing edit: %q", text)
	}
	if !strings.HasSuffix(text, "__gfx__\n0000\n__label__\nlabel\n") {
		t.Fatalf("non-code suffix changed: %q", text)
	}
	if !strings.HasPrefix(text, "pico-8 cartridge // pico-8.com\nversion 42\n__lua__\n") {
		t.Fatalf("header changed: %q", text)
	}
}

func TestCartTabSwitchPreservesView(t *testing.T) {
	game, _ := newCartGame(t, fullCart)
	buffer := game.session.CurrentBuffer()
	if err := sendKeys(t, game, "j:tabn<enter>"); err != nil {
		t.Fatalf("Update tabn: %v", err)
	}
	if buffer.ActiveTab != 1 {
		t.Fatalf("active tab = %d, want 1", buffer.ActiveTab)
	}
	if got := buffer.Editor.Cursor.Pos.Line; got != 3 {
		t.Fatalf("tab 1 cursor line = %d, want 3", got)
	}
	if err := sendKeys(t, game, ":tabp<enter>"); err != nil {
		t.Fatalf("Update tabp: %v", err)
	}
	if buffer.ActiveTab != 0 {
		t.Fatalf("active tab = %d, want 0", buffer.ActiveTab)
	}
	if got := buffer.Editor.Cursor.Pos.Line; got != 1 {
		t.Fatalf("restored cursor line = %d, want 1", got)
	}
}

func TestCartTabConfinement(t *testing.T) {
	game, _ := newCartGame(t, fullCart)
	buffer := game.session.CurrentBuffer()
	if err := sendKeys(t, game, "G"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	before := buffer.Editor.Buffer.Text()
	if err := sendKeys(t, game, "J"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := buffer.Editor.Buffer.Text(); got != before {
		t.Fatalf("J joined across the tab boundary: %q", got)
	}
	if err := sendKeys(t, game, "onew<esc>"); err != nil {
		t.Fatalf("Update open line: %v", err)
	}
	lines := buffer.Editor.Buffer.Lines
	if len(lines) < 4 || lines[2] != "new" || lines[3] != "-->8 second" {
		t.Fatalf("lines = %q, want new before separator", lines)
	}
	if got := buffer.Cart.Tabs[0].EndLine; got != 3 {
		t.Fatalf("tab 0 end = %d, want 3", got)
	}
	if err := sendKeys(t, game, "ggdG"); err != nil {
		t.Fatalf("Update delete: %v", err)
	}
	text := buffer.Editor.Buffer.Text()
	if strings.Contains(text, "a=1") {
		t.Fatalf("tab 0 was not deleted: %q", text)
	}
	if !strings.Contains(text, "-->8 second\na=3\nc=3\n") {
		t.Fatalf("separator or tab 1 was damaged: %q", text)
	}
	if err := sendKeys(t, game, "/c=3<enter>"); err != nil {
		t.Fatalf("Update search: %v", err)
	}
	if !strings.Contains(game.session.Status, "E486") {
		t.Fatalf("search crossed into tab 1; status = %q", game.session.Status)
	}
	if err := sendKeys(t, game, ":tabn<enter>"); err != nil {
		t.Fatalf("Update tabn: %v", err)
	}
	if got := buffer.Editor.Cursor.Pos.Line; got != 1 {
		t.Fatalf("tab 1 cursor = %d, want its first line", got)
	}
	if err := sendKeys(t, game, "Gdddd"); err != nil {
		t.Fatalf("Update delete last tab: %v", err)
	}
	text = buffer.Editor.Buffer.Text()
	if strings.Contains(text, "a=3") || strings.Contains(text, "c=3") {
		t.Fatalf("tab 1 content survived deletion: %q", text)
	}
	if !strings.Contains(text, "-->8 second\n") {
		t.Fatalf("separator was removed: %q", text)
	}
}

func TestCartEmptyTabEditing(t *testing.T) {
	const cart = "pico-8 cartridge // pico-8.com\nversion 42\n__lua__\n-->8 first\na=1\n"
	game, path := newCartGame(t, cart)
	buffer := game.session.CurrentBuffer()
	if len(buffer.Cart.Tabs) != 2 || buffer.Cart.Tabs[0].StartLine != buffer.Cart.Tabs[0].EndLine {
		t.Fatalf("tabs = %+v, want an empty first tab", buffer.Cart.Tabs)
	}
	if err := sendKeys(t, game, "ihello<esc>:w<enter>"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	waitClean(t, game, buffer)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "__lua__\nhello\n-->8 first\na=1\n") {
		t.Fatalf("empty-tab edit damaged the separator: %q", data)
	}
}

func TestEmptyTabCommandsDoNotEditMarker(t *testing.T) {
	const cart = "pico-8 cartridge // pico-8.com\nversion 42\n__lua__\n-->8 first\na=1\n"
	game, _ := newCartGame(t, cart)
	buffer := game.session.CurrentBuffer()
	before := buffer.Editor.Buffer.Text()
	if err := sendKeys(t, game, "xrZJdd>>~"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := buffer.Editor.Buffer.Text(); got != before {
		t.Fatalf("empty-tab commands changed the marker:\n got %q\nwant %q", got, before)
	}
	if err := sendKeys(t, game, "o<esc>"); err != nil {
		t.Fatalf("Update open line: %v", err)
	}
	lines := buffer.Editor.Buffer.Lines
	if len(lines) != 3 || lines[0] != "" || lines[1] != "-->8 first" || lines[2] != "a=1" {
		t.Fatalf("o on an empty tab produced %q", lines)
	}
}

func TestSubstituteStaysInActiveTab(t *testing.T) {
	game, _ := newCartGame(t, fullCart)
	buffer := game.session.CurrentBuffer()
	if err := sendKeys(t, game, ":%s/a/X/g<enter>"); err != nil {
		t.Fatal(err)
	}
	lines := buffer.Editor.Buffer.Lines
	if lines[0] != "X=1" {
		t.Fatalf("tab 0 line = %q, want X=1", lines[0])
	}
	if lines[3] != "a=3" {
		t.Fatalf("tab 1 line changed to %q", lines[3])
	}
}

func TestTrailingEmptyTabEditing(t *testing.T) {
	const cart = "pico-8 cartridge // pico-8.com\nversion 42\n__lua__\na=1\n-->8\n"
	game, path := newCartGame(t, cart)
	buffer := game.session.CurrentBuffer()
	if err := sendKeys(t, game, ":tabn<enter>ihello<esc>:w<enter>"); err != nil {
		t.Fatal(err)
	}
	waitClean(t, game, buffer)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "a=1\n-->8\nhello\n") {
		t.Fatalf("trailing empty-tab edit = %q", data)
	}
}

func TestTrailingEmptyTabOpenLineSavesOneBlank(t *testing.T) {
	const cart = "pico-8 cartridge // pico-8.com\nversion 42\n__lua__\na=1\n-->8\n"
	game, path := newCartGame(t, cart)
	buffer := game.session.CurrentBuffer()
	if err := sendKeys(t, game, ":tabn<enter>o<esc>:w<enter>"); err != nil {
		t.Fatal(err)
	}
	waitClean(t, game, buffer)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "a=1\n-->8\n\n") {
		t.Fatalf("o in trailing empty tab = %q, want one blank line", data)
	}
	if strings.HasSuffix(string(data), "\n\n\n") {
		t.Fatalf("o saved an extra blank line: %q", data)
	}
}

func TestTrailingEmptyTabWithoutFinalNewline(t *testing.T) {
	const cart = "pico-8 cartridge // pico-8.com\nversion 42\n__lua__\na=1\n-->8"
	game, path := newCartGame(t, cart)
	buffer := game.session.CurrentBuffer()
	if err := sendKeys(t, game, ":tabn<enter>o<esc>:w<enter>"); err != nil {
		t.Fatal(err)
	}
	waitClean(t, game, buffer)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "a=1\n-->8\n\n") {
		t.Fatalf("o without a final newline = %q, want one blank line", data)
	}
}

func TestIncludedLuaShowsOwningCart(t *testing.T) {
	root := t.TempDir()
	cartPath := filepath.Join(root, "game.p8")
	includePath := filepath.Join(root, "lib", "util.lua")
	if err := os.MkdirAll(filepath.Dir(includePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(includePath, []byte("util=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cart := "pico-8 cartridge // pico-8.com\nversion 42\n__lua__\n#include lib/util.lua\nprint(1)\n"
	if err := os.WriteFile(cartPath, []byte(cart), 0o644); err != nil {
		t.Fatal(err)
	}
	game := New(Config{Logger: slog.New(slog.DiscardHandler), Now: func() time.Time { return testClock }, File: cartPath})
	if err := game.StartupError(); err != nil {
		t.Fatal(err)
	}
	game.source = &fakeSource{}
	if err := sendKeys(t, game, ":e lib/util.lua<enter>"); err != nil {
		t.Fatal(err)
	}
	if got := game.session.CurrentBuffer().Owner; got != cartPath {
		t.Fatalf("owner = %q, want %q", got, cartPath)
	}
}

func TestQuitDialogCancelAndDiscard(t *testing.T) {
	game, _ := newCartGame(t, fullCart)
	if err := sendKeys(t, game, "iX<esc>"); err != nil {
		t.Fatal(err)
	}
	game.source.(*fakeSource).events = []input.Event{{Kind: input.KindKeyDown, Key: input.KeyQ, Mods: input.ModCtrl}}
	if err := game.Update(); err != nil {
		t.Fatalf("Ctrl+Q: %v", err)
	}
	if game.session.Dialog == nil || game.session.Dialog.Kind != DialogQuit {
		t.Fatalf("dialog = %+v, want quit dialog", game.session.Dialog)
	}
	if err := sendKeys(t, game, "c"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if game.session.Dialog != nil || game.pendingQuit {
		t.Fatal("cancel did not leave the session running")
	}
	game.source.(*fakeSource).events = []input.Event{{Kind: input.KindKeyDown, Key: input.KeyQ, Mods: input.ModCtrl}}
	if err := game.Update(); err != nil {
		t.Fatal(err)
	}
	if err := sendKeys(t, game, "d"); err != ebiten.Termination {
		t.Fatalf("discard = %v, want Termination", err)
	}
}

func TestSaveConflictOverwriteCreatesBackup(t *testing.T) {
	game, path := newCartGame(t, fullCart)
	buffer := game.session.CurrentBuffer()
	if err := sendKeys(t, game, "iX<esc>"); err != nil {
		t.Fatal(err)
	}
	external := "external bytes\n"
	if err := os.WriteFile(path, []byte(external), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sendKeys(t, game, ":w<enter>"); err != nil {
		t.Fatalf("save: %v", err)
	}
	if game.session.Dialog == nil || game.session.Dialog.Kind != DialogConflict {
		t.Fatalf("dialog = %+v, want conflict dialog", game.session.Dialog)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != external {
		t.Fatalf("conflict wrote before confirmation: %q %v", data, err)
	}
	if err := sendKeys(t, game, "o"); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	waitClean(t, game, buffer)
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Xa=1") {
		t.Fatalf("saved cart = %q", data)
	}
	backup, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != external {
		t.Fatalf("backup = %q, want conflicting bytes", backup)
	}
}

func TestTreeFilterTextIsNotSeparatorBlocked(t *testing.T) {
	game, _ := newCartGame(t, fullCart)
	buffer := game.session.CurrentBuffer()
	buffer.Editor.Buffer.Lines[0] = "-->"
	buffer.Editor.SetCursor(editor.Pos{Line: 0, Rune: 3})
	game.session.Vim.Mode = vim.ModeInsert
	game.session.Tree.Visible = true
	game.session.Tree.Focused = true
	game.session.Tree.Filtering = true
	game.session.Tree.Filter = ""
	_, changed, _ := game.session.HandleEvent(input.Event{Kind: input.KindText, Runes: []rune{'8'}})
	if !changed || game.session.Tree.Filter != "8" {
		t.Fatalf("filter = %q changed=%t, want tree text", game.session.Tree.Filter, changed)
	}
	if strings.Contains(game.session.Status, "cannot create") {
		t.Fatalf("tree text was blocked as a separator: %q", game.session.Status)
	}
}

func TestSectionHeaderKeepsHiddenSeparatorSafe(t *testing.T) {
	game, path := newCartGame(t, fullCart)
	buffer := game.session.CurrentBuffer()
	if err := sendKeys(t, game, "ggO__gfx__<esc>"); err != nil {
		t.Fatal(err)
	}
	if len(buffer.Cart.Tabs) < 2 {
		t.Fatalf("tabs = %+v", buffer.Cart.Tabs)
	}
	tab := buffer.Cart.Tabs[1]
	if tab.MarkerLine < 0 || tab.StartLine != tab.MarkerLine+1 {
		t.Fatalf("stale tab range after failed sync: %+v", tab)
	}
	if line := buffer.Editor.Buffer.Lines[tab.MarkerLine]; !strings.HasPrefix(line, "-->8") {
		t.Fatalf("marker line = %q", line)
	}
	if err := sendKeys(t, game, ":tabn<enter>dd:tabp<enter>ggdd:w<enter>"); err != nil {
		t.Fatal(err)
	}
	waitClean(t, game, buffer)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "-->8 second") {
		t.Fatalf("hidden separator was lost: %q", data)
	}
	if strings.Contains(string(data), "__gfx__\nb=2") {
		t.Fatalf("section header remained in code: %q", data)
	}
}

func TestSectionHeaderCrossTabUndoKeepsSeparator(t *testing.T) {
	game, path := newCartGame(t, fullCart)
	buffer := game.session.CurrentBuffer()
	if err := sendKeys(t, game, "ggO__gfx__<esc>:tabn<enter>Go<esc>:tabp<enter>u:tabn<enter>ggdd:tabp<enter>ggdd:w<enter>"); err != nil {
		t.Fatal(err)
	}
	waitClean(t, game, buffer)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "-->8 second") {
		t.Fatalf("hidden separator was lost after cross-tab undo: %q", data)
	}
	if !strings.Contains(string(data), "-->8 second\nc=3\n") {
		t.Fatalf("tab 1 content is wrong: %q", data)
	}
	if strings.Contains(string(data), "__gfx__\nb=2") {
		t.Fatalf("section header remained in code: %q", data)
	}
}

func TestFailedSyncDoesNotShiftEarlierTabViews(t *testing.T) {
	const cart = "pico-8 cartridge // pico-8.com\nversion 42\n__lua__\na=1\nb=2\n-->8 second\nc=3\n-->8 third\nd=4\n"
	game, _ := newCartGame(t, cart)
	buffer := game.session.CurrentBuffer()
	if err := sendKeys(t, game, "j:tabn<enter>ggO__gfx__<esc>:tabp<enter>"); err != nil {
		t.Fatal(err)
	}
	if got := buffer.Editor.Cursor.Pos.Line; got != 1 {
		t.Fatalf("earlier tab cursor = %d, want preserved line 1", got)
	}
}

func TestLongDirtyFileTabKeepsMarker(t *testing.T) {
	game, _ := newCartGame(t, fullCart)
	long := filepath.Join(filepath.Dir(game.session.CurrentEditor().Buffer.Path), strings.Repeat("x", 40)+".lua")
	if err := os.WriteFile(long, []byte("x=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sendKeys(t, game, ":e "+filepath.Base(long)+"<enter>iX<esc>"); err != nil {
		t.Fatal(err)
	}
	labels := game.session.fileTabLabels()
	if len(labels) == 0 || !strings.Contains(labels[len(labels)-1], "*") {
		t.Fatalf("labels = %q, want a dirty marker", labels)
	}
}

func TestClampTreeTopKeepsSelectionVisible(t *testing.T) {
	if got := clampTreeTop(0, 80, 100, 40); got != 41 {
		t.Fatalf("top = %d, want 41", got)
	}
	if got := clampTreeTop(80, 2, 100, 40); got != 2 {
		t.Fatalf("top = %d, want 2", got)
	}
}

func TestDirectoryStartupFocusesTreeAndOpensFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "game.p8"), []byte(fullCart), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "lib.lua"), []byte("lib=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	game := New(Config{Logger: slog.New(slog.DiscardHandler), Now: func() time.Time { return testClock }, File: root})
	if err := game.StartupError(); err != nil {
		t.Fatal(err)
	}
	game.source = &fakeSource{}
	if !game.session.Tree.Visible || !game.session.Tree.Focused {
		t.Fatal("directory startup did not focus the tree")
	}
	if got := game.session.CurrentEditor().Buffer.Text(); got != "" {
		t.Fatalf("startup buffer = %q, want empty", got)
	}
	if err := sendKeys(t, game, "<enter>"); err != nil {
		t.Fatal(err)
	}
	if game.session.CurrentBuffer().Cart == nil {
		t.Fatal("Enter did not open the cart")
	}
}

func TestExternalFocusReloadsCleanAndReportsDirty(t *testing.T) {
	game, path := newCartGame(t, fullCart)
	game.session.CurrentEditor().SetCursor(editor.Pos{Line: 1})
	reloaded := strings.Replace(fullCart, "a=1", "a=9", 1)
	if err := os.WriteFile(path, []byte(reloaded), 0o644); err != nil {
		t.Fatal(err)
	}
	if conflict := game.session.RefreshFromDisk(); conflict != nil {
		t.Fatalf("clean buffer conflict = %+v", conflict)
	}
	current := game.session.CurrentBuffer()
	if got := current.Editor.Buffer.Text(); !strings.Contains(got, "a=9") {
		t.Fatalf("clean buffer did not reload: %q", got)
	}
	if got := current.Editor.Cursor.Pos.Line; got != 1 {
		t.Fatalf("reloaded cursor line = %d, want preserved line 1", got)
	}
	if _, err := current.Editor.Buffer.Apply(editor.Edit{Start: current.Editor.Cursor.Pos, End: current.Editor.Cursor.Pos, Text: "dirty"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(reloaded, "a=9", "a=7", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	conflict := game.session.RefreshFromDisk()
	if conflict == nil || conflict.Deleted {
		t.Fatalf("dirty conflict = %+v, want changed-file conflict", conflict)
	}
}

func TestForceEditAnotherPathDiscardsCurrentChanges(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "a.lua")
	second := filepath.Join(root, "b.lua")
	if err := os.WriteFile(first, []byte("a=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("b=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	game := New(Config{Logger: slog.New(slog.DiscardHandler), Now: func() time.Time { return testClock }, File: first})
	if err := game.StartupError(); err != nil {
		t.Fatal(err)
	}
	game.source = &fakeSource{}
	if err := sendKeys(t, game, "iX<esc>:e! b.lua<enter>"); err != nil {
		t.Fatal(err)
	}
	if len(game.session.Buffers) != 1 {
		t.Fatalf("buffers = %d, want 1 after :e!", len(game.session.Buffers))
	}
	if got := game.session.CurrentEditor().Buffer.Path; got != second {
		t.Fatalf("current = %q, want %q", got, second)
	}
	if !game.session.CanQuitAll() {
		t.Fatal("discarded buffer remained dirty")
	}
}

func TestReadOnlySaveShowsErrorDialog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "readonly.lua")
	if err := os.WriteFile(path, []byte("a=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	game := New(Config{Logger: slog.New(slog.DiscardHandler), Now: func() time.Time { return testClock }, File: path})
	if err := game.StartupError(); err != nil {
		t.Fatal(err)
	}
	game.source = &fakeSource{}
	if err := sendKeys(t, game, "iX<esc>:w<enter>"); err != nil {
		t.Fatal(err)
	}
	if game.session.Dialog == nil || game.session.Dialog.Kind != DialogError {
		t.Fatalf("dialog = %+v, want read-only error dialog", game.session.Dialog)
	}
	if !game.session.CurrentEditor().Buffer.Dirty() {
		t.Fatal("read-only save cleared the dirty flag")
	}
}

func TestConflictReloadThenWriteQuit(t *testing.T) {
	game, path := newCartGame(t, fullCart)
	if err := sendKeys(t, game, "iX<esc>"); err != nil {
		t.Fatal(err)
	}
	external := strings.Replace(fullCart, "a=1", "a=5", 1)
	if err := os.WriteFile(path, []byte(external), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sendKeys(t, game, ":wq<enter>"); err != nil {
		t.Fatal(err)
	}
	if game.session.Dialog == nil || game.session.Dialog.Kind != DialogConflict {
		t.Fatalf("dialog = %+v, want conflict", game.session.Dialog)
	}
	if err := sendKeys(t, game, "r"); err != ebiten.Termination {
		t.Fatalf("reload then quit = %v, want Termination", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != external {
		t.Fatalf("reload wrote the file: %q", data)
	}
}

func TestDialogAcceleratorDoesNotInsertText(t *testing.T) {
	game, path := newCartGame(t, fullCart)
	if err := sendKeys(t, game, "iX"); err != nil {
		t.Fatal(err)
	}
	external := strings.Replace(fullCart, "a=1", "a=5", 1)
	if err := os.WriteFile(path, []byte(external), 0o644); err != nil {
		t.Fatal(err)
	}
	game.source.(*fakeSource).events = []input.Event{{Kind: input.KindKeyDown, Key: input.KeyS, Mods: input.ModCtrl}}
	if err := game.Update(); err != nil {
		t.Fatal(err)
	}
	if game.session.Dialog == nil || game.session.Dialog.Kind != DialogConflict {
		t.Fatalf("dialog = %+v, want conflict", game.session.Dialog)
	}
	if err := sendKeys(t, game, "o"); err != nil {
		t.Fatal(err)
	}
	waitClean(t, game, game.session.CurrentBuffer())
	buffer := game.session.CurrentBuffer()
	if got := buffer.Editor.Buffer.Lines[0]; got != "Xa=1" {
		t.Fatalf("line = %q, want the accelerator text suppressed", got)
	}
}

func TestTypingTabSeparatorIsRejected(t *testing.T) {
	game, _ := newCartGame(t, fullCart)
	buffer := game.session.CurrentBuffer()
	if err := sendKeys(t, game, "ggcc-->8 new<esc>"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(game.session.Status, "cannot create") {
		t.Fatalf("status = %q, want separator rejection", game.session.Status)
	}
	text := buffer.Editor.Buffer.Text()
	if got := strings.Count(text, "-->8"); got != 1 {
		t.Fatalf("separator count = %d, want the original only: %q", got, text)
	}
	if got := buffer.Editor.Buffer.Lines[0]; got != "--> new" {
		t.Fatalf("line 0 = %q, want the typed text kept as code", got)
	}
}

func TestEmptyTabVisualOperatorsDoNotCopySeparator(t *testing.T) {
	const cart = "pico-8 cartridge // pico-8.com\nversion 42\n__lua__\n-->8 first\na=1\n"
	game, _ := newCartGame(t, cart)
	buffer := game.session.CurrentBuffer()
	before := buffer.Editor.Buffer.Text()
	if err := sendKeys(t, game, "VU"); err != nil {
		t.Fatal(err)
	}
	if got := buffer.Editor.Buffer.Text(); got != before {
		t.Fatalf("visual operator copied the separator:\n got %q\nwant %q", got, before)
	}
	game.source.(*fakeSource).events = []input.Event{
		{Kind: input.KindKeyDown, Key: input.KeyV, Mods: input.ModCtrl},
		{Kind: input.KindKeyDown, Key: input.KeyD},
	}
	if err := game.Update(); err != nil {
		t.Fatal(err)
	}
	if got := buffer.Editor.Buffer.Text(); got != before {
		t.Fatalf("block delete changed the empty tab:\n got %q\nwant %q", got, before)
	}
}

func TestSwitchingTabsDuringCountedInsertEndsInsert(t *testing.T) {
	game, _ := newCartGame(t, fullCart)
	buffer := game.session.CurrentBuffer()
	if err := sendKeys(t, game, "2iZ"); err != nil {
		t.Fatal(err)
	}
	game.source.(*fakeSource).events = eventKeyTab
	if err := game.Update(); err != nil {
		t.Fatal(err)
	}
	if buffer.ActiveTab != 1 {
		t.Fatalf("active tab = %d, want 1", buffer.ActiveTab)
	}
	text := buffer.Editor.Buffer.Text()
	if strings.Count(text, "-->8") != 1 {
		t.Fatalf("separator duplicated: %q", text)
	}
	if !strings.Contains(text, "ZZa=1") {
		t.Fatalf("counted insert was not completed in the original tab: %q", text)
	}
}

var eventKeyTab = []input.Event{{Kind: input.KindKeyDown, Key: input.KeyTab, Mods: input.ModCtrl}}

func TestTabViewStateFollowsInsertedSeparator(t *testing.T) {
	game, _ := newCartGame(t, fullCart)
	buffer := game.session.CurrentBuffer()
	if err := sendKeys(t, game, ":tabn<enter>G:tabp<enter>:%s/^a=1/-->8 new/<enter>o<esc>:tabn<enter>:tabn<enter>"); err != nil {
		t.Fatal(err)
	}
	if len(buffer.Cart.Tabs) != 3 {
		t.Fatalf("tabs = %+v, want 3 after the inserted separator", buffer.Cart.Tabs)
	}
	if buffer.ActiveTab != 2 {
		t.Fatalf("active tab = %d, want the shifted original tab", buffer.ActiveTab)
	}
	if got := buffer.Editor.Cursor.Pos.Line; got != 5 {
		t.Fatalf("shifted cursor line = %d, want 5", got)
	}
}

func TestTypingTabSeparatorPrefixIsRejected(t *testing.T) {
	game, _ := newCartGame(t, fullCart)
	buffer := game.session.CurrentBuffer()
	if err := sendKeys(t, game, "ggcc>8 x<esc>I--<esc>"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(game.session.Status, "cannot create") {
		t.Fatalf("status = %q, want separator rejection", game.session.Status)
	}
	if got := strings.Count(buffer.Editor.Buffer.Text(), "-->8"); got != 1 {
		t.Fatalf("separator count = %d, want the original only", got)
	}
}

func TestSwitchingTabsInSameBatchKeepsInsertText(t *testing.T) {
	game, _ := newCartGame(t, fullCart)
	buffer := game.session.CurrentBuffer()
	if err := sendKeys(t, game, "2i"); err != nil {
		t.Fatal(err)
	}
	game.source.(*fakeSource).events = []input.Event{
		{Kind: input.KindKeyDown, Key: input.KeyZ},
		{Kind: input.KindKeyDown, Key: input.KeyTab, Mods: input.ModCtrl},
		{Kind: input.KindText, Runes: []rune{'Z'}},
	}
	if err := game.Update(); err != nil {
		t.Fatal(err)
	}
	if buffer.ActiveTab != 1 {
		t.Fatalf("active tab = %d, want 1", buffer.ActiveTab)
	}
	if !strings.Contains(buffer.Editor.Buffer.Text(), "ZZa=1") {
		t.Fatalf("insert text was lost with the deferred switch: %q", buffer.Editor.Buffer.Text())
	}
	if strings.Count(buffer.Editor.Buffer.Text(), "-->8") != 1 {
		t.Fatalf("separator duplicated: %q", buffer.Editor.Buffer.Text())
	}
}

func TestDeferredTabSwitchDoesNotOverrideSameBatchClick(t *testing.T) {
	game, _ := newCartGame(t, fullCart)
	buffer := game.session.CurrentBuffer()
	if err := sendKeys(t, game, "i"); err != nil {
		t.Fatal(err)
	}
	hits := fitTabs(game.session.cartTabLabels(buffer), buffer.ActiveTab, GridColumns)
	if len(hits) < 2 {
		t.Fatalf("cart hits = %+v", hits)
	}
	x := hits[1].StartCell*CellWidth + 1
	game.source.(*fakeSource).events = []input.Event{
		{Kind: input.KindKeyDown, Key: input.KeyTab, Mods: input.ModCtrl},
		{Kind: input.KindMouse, X: x, Y: cartTabRow * CellHeight, Buttons: input.MouseLeft},
	}
	if err := game.Update(); err != nil {
		t.Fatal(err)
	}
	if buffer.ActiveTab != 1 {
		t.Fatalf("active tab = %d, want the same-batch click to win", buffer.ActiveTab)
	}
}

func TestSaveAllRequeuesEditsDuringSave(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.lua")
	if err := os.WriteFile(path, []byte("a=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	game := New(Config{Logger: slog.New(slog.DiscardHandler), Now: func() time.Time { return testClock }, File: path})
	if err := game.StartupError(); err != nil {
		t.Fatal(err)
	}
	game.source = &fakeSource{}
	if err := sendKeys(t, game, "iX<esc>"); err != nil {
		t.Fatal(err)
	}
	buffer := game.session.CurrentBuffer()
	game.startSaveAll()
	if _, err := buffer.Editor.Insert("Y"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		err := game.Update()
		if err == ebiten.Termination {
			break
		}
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatal("save all did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "YXa=1\n" {
		t.Fatalf("saved = %q, want the edit made during save-all", data)
	}
}

func TestDeferredQuitKeepsLateEdits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.lua")
	if err := os.WriteFile(path, []byte("a=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	game := New(Config{Logger: slog.New(slog.DiscardHandler), Now: func() time.Time { return testClock }, File: path})
	if err := game.StartupError(); err != nil {
		t.Fatal(err)
	}
	game.source = &fakeSource{}
	game.source.(*fakeSource).events = []input.Event{{Kind: input.KindKeyDown, Key: input.KeyS, Mods: input.ModCtrl}}
	if err := game.Update(); err != nil {
		t.Fatal(err)
	}
	if !game.saveBusy {
		t.Fatal("Ctrl+S did not start an asynchronous save")
	}
	if quit, changed := game.requestQuitAll(); quit || !changed || !game.pendingQuit {
		t.Fatalf("deferred quit = quit %t changed %t pending %t", quit, changed, game.pendingQuit)
	}
	buffer := game.session.CurrentBuffer()
	if _, err := buffer.Editor.Insert("X"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		err := game.Update()
		if err == ebiten.Termination {
			t.Fatal("deferred quit discarded an edit made during the save")
		}
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if !game.saveBusy && !game.pendingQuit {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("save did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	if !buffer.Editor.Buffer.Dirty() || !strings.Contains(buffer.Editor.Buffer.Text(), "Xa=1") {
		t.Fatalf("late edit was lost: %q", buffer.Editor.Buffer.Text())
	}
}

func TestQuitGateRequeuesLateEdits(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.lua")
	if err := os.WriteFile(path, []byte("a=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	game := New(Config{Logger: slog.New(slog.DiscardHandler), Now: func() time.Time { return testClock }, File: path})
	if err := game.StartupError(); err != nil {
		t.Fatal(err)
	}
	game.source = &fakeSource{}
	buffer := game.session.CurrentBuffer()
	if _, err := buffer.Editor.Insert("X"); err != nil {
		t.Fatal(err)
	}
	game.saveAllQuit = true
	game.pendingQuit = true
	if game.shouldTerminate() {
		t.Fatal("quit gate ignored a late dirty edit")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		err := game.Update()
		if err == ebiten.Termination {
			break
		}
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatal("late-edit save did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "Xa=1\n" {
		t.Fatalf("saved = %q, want the late edit", data)
	}
}

func TestQuitSaveAllSavesEveryDirtyBuffer(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "a.lua")
	second := filepath.Join(root, "b.lua")
	if err := os.WriteFile(first, []byte("a=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("b=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	game := New(Config{Logger: slog.New(slog.DiscardHandler), Now: func() time.Time { return testClock }, File: first})
	if err := game.StartupError(); err != nil {
		t.Fatal(err)
	}
	game.source = &fakeSource{}
	if err := sendKeys(t, game, ":e b.lua<enter>:bp<enter>iX<esc>:bn<enter>iY<esc>"); err != nil {
		t.Fatal(err)
	}
	game.source.(*fakeSource).events = []input.Event{{Kind: input.KindKeyDown, Key: input.KeyQ, Mods: input.ModCtrl}}
	if err := game.Update(); err != nil {
		t.Fatal(err)
	}
	if game.session.Dialog == nil || game.session.Dialog.Kind != DialogQuit {
		t.Fatalf("dialog = %+v", game.session.Dialog)
	}
	game.source.(*fakeSource).events = appParseKeys(t, "s")
	for {
		err := game.Update()
		if err == ebiten.Termination {
			break
		}
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
	}
	for path, want := range map[string]string{first: "Xa=1\n", second: "Yb=1\n"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != want {
			t.Fatalf("%s = %q, want %q", path, data, want)
		}
	}
}

func TestSymlinkOutsideWorkspaceNeedsConfirmation(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "target.lua")
	if err := os.WriteFile(target, []byte("x=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.lua")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink: %v", err)
	}
	game := New(Config{Logger: slog.New(slog.DiscardHandler), Now: func() time.Time { return testClock }, File: link})
	if err := game.StartupError(); err != nil {
		t.Fatal(err)
	}
	game.source = &fakeSource{}
	if err := sendKeys(t, game, "iX<esc>:w<enter>"); err != nil {
		t.Fatal(err)
	}
	if game.session.Dialog == nil || game.session.Dialog.Kind != DialogOutsideRoot {
		t.Fatalf("dialog = %+v, want outside-root confirmation", game.session.Dialog)
	}
	if err := sendKeys(t, game, "s"); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		_ = game.Update()
		return !game.session.CurrentEditor().Buffer.Dirty()
	})
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Xx=1") {
		t.Fatalf("target = %q, want the edit", data)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink was replaced: %v %v", info, err)
	}
}

func TestTreeToggleFilterAndOpen(t *testing.T) {
	root := t.TempDir()
	cartPath := filepath.Join(root, "game.p8")
	libPath := filepath.Join(root, "lib.lua")
	if err := os.WriteFile(cartPath, []byte(fullCart), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(libPath, []byte("lib=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	game := New(Config{Logger: slog.New(slog.DiscardHandler), Now: func() time.Time { return testClock }, File: root})
	if err := game.StartupError(); err != nil {
		t.Fatal(err)
	}
	game.source = &fakeSource{}
	toggle := []input.Event{
		{Kind: input.KindKeyDown, Key: input.KeyB, Mods: input.ModCtrl | input.ModShift},
		{Kind: input.KindText, Mods: input.ModCtrl | input.ModShift, Runes: []rune{'B'}},
	}
	game.source.(*fakeSource).events = toggle
	if err := game.Update(); err != nil {
		t.Fatal(err)
	}
	if game.session.Tree.Visible {
		t.Fatal("Ctrl+Shift+B did not hide the tree")
	}
	game.source.(*fakeSource).events = toggle
	if err := game.Update(); err != nil {
		t.Fatal(err)
	}
	if !game.session.Tree.Visible || !game.session.Tree.Focused {
		t.Fatal("Ctrl+Shift+B did not show and focus the tree")
	}
	if err := sendKeys(t, game, "/lib<enter>"); err != nil {
		t.Fatal(err)
	}
	if game.session.Tree.Filter != "lib" {
		t.Fatalf("filter = %q, want lib (slash must not be captured)", game.session.Tree.Filter)
	}
	if err := sendKeys(t, game, "<enter>"); err != nil {
		t.Fatal(err)
	}
	if got := game.session.CurrentEditor().Buffer.Path; got != libPath {
		t.Fatalf("opened %q, want %q", got, libPath)
	}
}

func TestMouseClicksSwitchFileAndCartTabs(t *testing.T) {
	game, _ := newCartGame(t, fullCart)
	buffer := game.session.CurrentBuffer()
	cartHits := fitTabs(game.session.cartTabLabels(buffer), buffer.ActiveTab, GridColumns)
	if len(cartHits) != 2 {
		t.Fatalf("cart hits = %+v", cartHits)
	}
	x := (cartHits[1].StartCell + 1) * CellWidth
	if !game.session.ClickCartTabs(x, cartTabRow*CellHeight) {
		t.Fatal("cart tab click was not handled")
	}
	if buffer.ActiveTab != 1 {
		t.Fatalf("active tab = %d, want 1", buffer.ActiveTab)
	}

	second := filepath.Join(filepath.Dir(buffer.Editor.Buffer.Path), "other.lua")
	if err := os.WriteFile(second, []byte("y=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sendKeys(t, game, ":e other.lua<enter>"); err != nil {
		t.Fatal(err)
	}
	fileHits := fitTabs(game.session.fileTabLabels(), game.session.Current, GridColumns)
	if len(fileHits) < 2 {
		t.Fatalf("file hits = %+v", fileHits)
	}
	if !game.session.ClickFileTabs(fileHits[0].StartCell*CellWidth, 0) {
		t.Fatal("file tab click was not handled")
	}
	if game.session.Current != 0 {
		t.Fatalf("current buffer = %d, want 0", game.session.Current)
	}
}
