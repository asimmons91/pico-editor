package app

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/asimmons91/pico-editor/internal/cart"
	"github.com/asimmons91/pico-editor/internal/config"
	"github.com/asimmons91/pico-editor/internal/editor"
	"github.com/asimmons91/pico-editor/internal/input"
)

func dirtyBuffer(path, text string) *SessionBuffer {
	buffer := editor.New(path)
	buffer.Lines = strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	buffer.TrailingNewline = strings.HasSuffix(text, "\n")
	buffer.MarkSavedText("")
	return &SessionBuffer{Editor: editor.NewEditor(buffer)}
}

func TestRecoveryStoreSaveLoadDelete(t *testing.T) {
	dir := t.TempDir()
	store := newRecoveryStore(dir, slog.New(slog.DiscardHandler), time.Now)
	path := filepath.Join(t.TempDir(), "game.lua")
	buffer := dirtyBuffer(path, "print(1)\n")
	if err := store.Save(buffer); err != nil {
		t.Fatal(err)
	}
	entries, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].Path != path || entries[0].Kind != "lua" {
		t.Fatalf("entry = %+v", entries[0])
	}
	text, err := store.Load(entries[0])
	if err != nil {
		t.Fatal(err)
	}
	if text != "print(1)\n" {
		t.Fatalf("text = %q", text)
	}
	if err := store.Delete(path); err != nil {
		t.Fatal(err)
	}
	entries, err = store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries after delete = %+v", entries)
	}
}

func TestRecoveryStoreRejectsOversizeAndBadDirectory(t *testing.T) {
	dir := t.TempDir()
	store := newRecoveryStore(dir, slog.New(slog.DiscardHandler), time.Now)
	buffer := dirtyBuffer(filepath.Join(t.TempDir(), "big.lua"), strings.Repeat("x", recoveryMaxBytes+1))
	if err := store.Save(buffer); err == nil {
		t.Fatal("Save accepted an oversize entry")
	}

	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	bad := newRecoveryStore(blocker, slog.New(slog.DiscardHandler), time.Now)
	if err := bad.Save(dirtyBuffer(filepath.Join(t.TempDir(), "x.lua"), "x\n")); err == nil {
		t.Fatal("Save succeeded with a file as the journal directory")
	}
}

func TestRecoveryStorePrunesOldest(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(1000, 0)
	store := newRecoveryStore(dir, slog.New(slog.DiscardHandler), func() time.Time { return now })
	for index := 0; index < recoveryMaxEntries+1; index++ {
		path := filepath.Join(t.TempDir(), "file.lua")
		if err := store.Save(dirtyBuffer(path, "x\n")); err != nil {
			t.Fatal(err)
		}
		now = now.Add(time.Second)
	}
	entries, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != recoveryMaxEntries {
		t.Fatalf("entries = %d, want %d", len(entries), recoveryMaxEntries)
	}
	if store.TakePruned() == 0 {
		t.Fatal("TakePruned = 0, want a pruned entry")
	}
}

func TestSessionRecoverPlainTextIsDirty(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.lua")
	if err := os.WriteFile(path, []byte("abc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	session, err := NewSessionAtOptions(root, path, slog.New(slog.DiscardHandler), SessionOptions{Settings: config.DefaultSettings()})
	if err != nil {
		t.Fatal(err)
	}
	entry := RecoveryEntry{Path: path, Kind: "lua", Timestamp: time.Now()}
	if err := session.Recover(entry, "abcd\n"); err != nil {
		t.Fatal(err)
	}
	buffer := session.CurrentBuffer()
	if got := buffer.Editor.Buffer.Text(); got != "abcd\n" {
		t.Fatalf("text = %q", got)
	}
	if !buffer.Editor.Buffer.Dirty() {
		t.Fatal("recovered buffer is not dirty")
	}
	if err := buffer.Editor.Buffer.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "abcd\n" {
		t.Fatalf("saved = %q", data)
	}
}

func TestSessionRecoverCartPreservesSuffix(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "game.p8")
	original := "pico-8 cartridge // pico-8.com\nversion 42\n__lua__\na=1\n__gfx__\n0000\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	session, err := NewSessionAtOptions(root, path, slog.New(slog.DiscardHandler), SessionOptions{Settings: config.DefaultSettings()})
	if err != nil {
		t.Fatal(err)
	}
	recovered := "pico-8 cartridge // pico-8.com\nversion 42\n__lua__\na=2\n__gfx__\n0000\n"
	entry := RecoveryEntry{Path: path, Kind: "cart", Timestamp: time.Now()}
	if err := session.Recover(entry, recovered); err != nil {
		t.Fatal(err)
	}
	buffer := session.CurrentBuffer()
	if buffer.Cart == nil {
		t.Fatal("recovered cart lost its cart document")
	}
	if got := buffer.Editor.Buffer.Text(); !strings.Contains(got, "a=2") {
		t.Fatalf("code = %q, want a=2", got)
	}
	if !buffer.Editor.Buffer.Dirty() {
		t.Fatal("recovered cart is not dirty")
	}
	if err := buffer.Cart.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "a=2") || !strings.Contains(string(data), "__gfx__\n0000") {
		t.Fatalf("saved cart = %q", data)
	}
}

func TestMaybeScheduleRecoveryWritesAfterInterval(t *testing.T) {
	game, _ := newEditorGame(t, "abc\n", &recordingClipboard{})
	if err := sendKeys(t, game, "ix<esc>"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	game.recovery = newRecoveryStore(dir, slog.New(slog.DiscardHandler), game.now)
	game.settings.Recovery.Enabled = true
	game.settings.Recovery.IntervalMS = 2000
	start := game.now()
	game.maybeScheduleRecovery(start)
	if entries, _ := game.recovery.List(); len(entries) != 0 {
		t.Fatalf("entries written before the interval: %+v", entries)
	}
	game.maybeScheduleRecovery(start.Add(2 * time.Second))
	entries, err := game.recovery.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %+v, want one", entries)
	}
}

func TestRecoverySkipsInvalidCartText(t *testing.T) {
	game, _ := newCartGame(t, fullCart)
	dir := t.TempDir()
	store := newRecoveryStore(dir, slog.New(slog.DiscardHandler), time.Now)
	buffer := game.session.CurrentBuffer()
	if err := buffer.Editor.Buffer.SetText("__gfx__\n"); err != nil {
		t.Fatal(err)
	}
	buffer.Editor.Buffer.MarkSavedText("")
	if err := store.Save(buffer); err == nil {
		t.Fatal("Save accepted a cart whose code would become a section header")
	}
	if entries, _ := store.List(); len(entries) != 0 {
		t.Fatalf("entries = %+v, want none", entries)
	}
}

func TestStartupRecoveryListAndActions(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.lua")
	if err := os.WriteFile(path, []byte("abc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	session, err := NewSessionAtOptions(root, "", slog.New(slog.DiscardHandler), SessionOptions{Settings: config.DefaultSettings()})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	store := newRecoveryStore(dir, slog.New(slog.DiscardHandler), time.Now)
	if err := store.Save(dirtyBuffer(path, "abcd\n")); err != nil {
		t.Fatal(err)
	}
	game := &Game{
		logger:        slog.New(slog.DiscardHandler),
		session:       session,
		settings:      config.DefaultSettings(),
		notifications: newNotificationCenter(time.Now),
		recovery:      store,
		knownBuffers:  map[string]bool{},
	}
	game.initRecovery()
	if game.pendingRecovery == nil || len(game.pendingRecovery.Entries) != 1 {
		t.Fatalf("pending recovery = %+v", game.pendingRecovery)
	}
	game.activateRecovery()
	if game.recoveryList == nil {
		t.Fatal("activateRecovery did not open the list")
	}
	key := func(k input.Key) input.Event { return input.Event{Kind: input.KindKeyDown, Key: k} }
	if _, _ = game.handleRecoveryList(key(input.KeyL)); game.recoveryList != nil && len(game.recoveryList.Entries) != 0 {
		t.Fatal("Later did not remove the row")
	}
	entries, _ := store.List()
	if len(entries) != 1 {
		t.Fatal("Later deleted the journal entry")
	}
	game.recoveryList = &RecoveryList{Entries: entries}
	game.handleRecoveryList(key(input.KeyR))
	if game.session.CurrentBuffer() == nil || !game.session.CurrentBuffer().Editor.Buffer.Dirty() {
		t.Fatal("Recover did not open a dirty buffer")
	}
	if entries, _ := store.List(); len(entries) != 0 {
		t.Fatal("Recover did not delete the journal entry")
	}
}

func TestRecoveryCartDocumentOpenData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "game.p8")
	data := []byte("pico-8 cartridge // pico-8.com\nversion 42\n__lua__\na=1\n")
	document, err := cart.OpenData(path, data)
	if err != nil {
		t.Fatal(err)
	}
	if document.Code.Text() != "a=1\n" {
		t.Fatalf("code = %q", document.Code.Text())
	}
	document.MarkSavedCodeText("")
	if !document.Dirty() {
		t.Fatal("MarkSavedCodeText did not dirty the recovered cart")
	}
}
