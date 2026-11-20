package app

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asimmons91/pico-editor/internal/config"
	"github.com/asimmons91/pico-editor/internal/input"
	"github.com/asimmons91/pico-editor/internal/lsp"
)

func newProjectGame(t *testing.T, configText string) *Game {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "game.lua")
	if err := os.WriteFile(path, []byte("abc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if configText != "" {
		if err := os.WriteFile(filepath.Join(root, ".pico-editor.toml"), []byte(configText), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	game := New(Config{
		Logger:    slog.New(slog.DiscardHandler),
		File:      path,
		EnableLSP: true,
		Version:   "test",
	})
	if err := game.StartupError(); err != nil {
		t.Fatal(err)
	}
	game.source = &fakeSource{}
	return game
}

func TestWorkspaceConfigAppliesEditorAndLSP(t *testing.T) {
	game := newProjectGame(t, `
[editor]
tab_width = 8
scrolloff = 0
[lsp]
enabled = false
`)
	if got := game.session.CurrentEditor().Buffer.TabWidth; got != 8 {
		t.Errorf("tab width = %d, want 8", got)
	}
	if got := game.session.CurrentEditor().Viewport.ScrollOff; got != 0 {
		t.Errorf("scrolloff = %d, want 0", got)
	}
	if game.session.LSP != nil {
		t.Error("lsp.enabled=false still started pico-lsp")
	}
}

func TestKeymapRebindCtrlS(t *testing.T) {
	game := newProjectGame(t, `
[keys.normal]
"ctrl+s" = "file.close"
[keys.insert]
"ctrl+s" = "file.close"
`)
	if err := sendKeys(t, game, "ix<esc>"); err != nil {
		t.Fatal(err)
	}
	game.source.(*fakeSource).events = []input.Event{{Kind: input.KindKeyDown, Key: input.KeyS, Mods: input.ModCtrl}}
	if err := game.Update(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(game.session.Status, "E37") {
		t.Fatalf("status = %q, want the dirty-close refusal", game.session.Status)
	}
	if !game.session.CurrentEditor().Buffer.Dirty() {
		t.Fatal("the rebound ctrl+s saved the buffer")
	}
}

func TestKeymapUnbindTreeToggle(t *testing.T) {
	game := newProjectGame(t, `
[keys.normal]
"ctrl+shift+b" = ""
`)
	before := game.session.Tree.Visible
	game.source.(*fakeSource).events = []input.Event{{
		Kind: input.KindKeyDown, Key: input.KeyB, Mods: input.ModCtrl | input.ModShift,
	}}
	if err := game.Update(); err != nil {
		t.Fatal(err)
	}
	if game.session.Tree.Visible != before {
		t.Fatal("unbound ctrl+shift+b toggled the tree")
	}
}

func TestConfigReloadOnFocus(t *testing.T) {
	game := newProjectGame(t, "[editor]\ntab_width = 2\n")
	path := filepath.Join(game.session.Workspace.Root, ".pico-editor.toml")
	if err := os.WriteFile(path, []byte("[editor]\ntab_width = 6\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !game.checkConfigOnFocus() {
		t.Fatal("checkConfigOnFocus = false after the config changed")
	}
	if got := game.session.CurrentEditor().Buffer.TabWidth; got != 6 {
		t.Fatalf("tab width = %d, want 6", got)
	}
}

func TestScaleChangeNotifiesRestart(t *testing.T) {
	game := newProjectGame(t, "")
	settings := game.settings.Clone()
	settings.Editor.Scale = 2
	game.applySettings(settings, nil, false)
	if !strings.Contains(historyText(game), "restart") {
		t.Fatalf("history = %q, want a restart notice", historyText(game))
	}
}

func TestConfigErrorDialogChoices(t *testing.T) {
	game := newProjectGame(t, "")
	game.settingsErr = errors.New("config.toml: toml: line 1: expected = after a key")
	game.showConfigError()
	if game.session.Dialog == nil || game.session.Dialog.Kind != DialogConfigError {
		t.Fatalf("dialog = %+v, want DialogConfigError", game.session.Dialog)
	}
	game.session.Dialog.Selected = 0
	game.session.Dialog.Chosen = true
	game.handleDialogResult()
	if game.settingsErr != nil {
		t.Fatal("Use defaults did not clear the error")
	}
	if game.session.Dialog != nil {
		t.Fatal("Use defaults left the dialog open")
	}

	game.settingsErr = errors.New("bad")
	game.showConfigError()
	game.session.Dialog.Selected = 1
	game.session.Dialog.Chosen = true
	game.handleDialogResult()
	if !game.pendingQuit || !game.forceQuit {
		t.Fatalf("Quit = pending %t force %t, want both true", game.pendingQuit, game.forceQuit)
	}
}

func TestLSPConfigMappingCarriesGlyphs(t *testing.T) {
	byteValue := 128
	settings := config.LSPSettings{
		Enabled:      true,
		AutoComplete: false,
		DebounceMS:   200,
		Glyphs: []config.Glyph{{
			Name:        "logo",
			Byte:        &byteValue,
			Context:     "both",
			Aliases:     []string{"mark"},
			Description: "project mark",
		}},
	}
	mapped := lspSettingsFromConfig(settings)
	if len(mapped.Glyphs) != 1 {
		t.Fatalf("glyphs = %+v", mapped.Glyphs)
	}
	if mapped.Glyphs[0].Name != "logo" || mapped.Glyphs[0].Byte == nil || *mapped.Glyphs[0].Byte != 128 {
		t.Fatalf("glyph = %+v", mapped.Glyphs[0])
	}
	if mapped.TypeChecking != lsp.DefaultSettings().TypeChecking {
		t.Errorf("type checking = %q", mapped.TypeChecking)
	}
}

func TestDetachLSPHidesLanguageUI(t *testing.T) {
	game := newProjectGame(t, "")
	game.session.LSP = lsp.New(lsp.Options{})
	game.session.LSPStatus = lsp.StatusReady
	game.session.LSPTokens["x"] = lsp.TokenSet{}
	game.session.DetachLSP()
	if game.session.LSP != nil || game.session.LSPStatus != lsp.StatusOff {
		t.Fatal("DetachLSP left a live service or ready status")
	}
	if len(game.session.LSPTokens) != 0 {
		t.Fatal("DetachLSP kept token caches")
	}
}

func historyText(game *Game) string {
	var text strings.Builder
	for _, entry := range game.notifications.HistoryEntries() {
		text.WriteString(entry.Message)
		text.WriteByte('\n')
	}
	return text.String()
}

func TestAutoCompleteSettingDisablesTypingTrigger(t *testing.T) {
	game := newProjectGame(t, "")
	game.settings.LSP.AutoComplete = false
	game.session.Settings.LSP.AutoComplete = false
	game.session.scheduleCompletion(game.now(), "x", false)
	if !game.session.completionDue.IsZero() {
		t.Fatal("auto_complete=false still scheduled completion")
	}
	game.session.scheduleCompletion(game.now(), "", true)
	if game.session.completionDue.IsZero() {
		t.Fatal("manual completion was not scheduled")
	}
}

func TestCommandPanicBecomesNotification(t *testing.T) {
	game := newProjectGame(t, "")
	command := Command{ID: "test.panic", Title: "Test: panic", Run: func(*Game) (bool, bool) {
		panic("boom")
	}}
	quit, changed := game.runCommand(command)
	if quit || !changed {
		t.Fatalf("runCommand = quit %t changed %t", quit, changed)
	}
	if !strings.Contains(historyText(game), "test.panic") {
		t.Fatalf("history = %q, want the failed command", historyText(game))
	}
}

func TestExMessagesAndReloadActions(t *testing.T) {
	game := newProjectGame(t, "[editor]\ntab_width = 2\n")
	if err := sendKeys(t, game, ":messages<enter>"); err != nil {
		t.Fatal(err)
	}
	if game.session.Help == nil || game.session.Help.Title != "Messages" {
		t.Fatalf("help = %+v, want messages", game.session.Help)
	}
	game.session.Help = nil
	path := filepath.Join(game.session.Workspace.Root, ".pico-editor.toml")
	if err := os.WriteFile(path, []byte("[editor]\ntab_width = 7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sendKeys(t, game, ":reload<enter>"); err != nil {
		t.Fatal(err)
	}
	if got := game.session.CurrentEditor().Buffer.TabWidth; got != 7 {
		t.Fatalf("tab width = %d, want 7", got)
	}
}

func TestPaletteShowsActiveModeBinding(t *testing.T) {
	game := newProjectGame(t, `
[keys.normal]
"ctrl+s" = "file.close"
"ctrl+shift+b" = ""
`)
	var closeCommand Command
	for _, command := range game.commands {
		if command.ID == "file.close" {
			closeCommand = command
			break
		}
	}
	if got := game.displayKey(closeCommand); got != "ctrl+s" {
		t.Fatalf("displayKey(file.close) = %q, want ctrl+s", got)
	}
	unbound := Command{ID: "tree.toggle", Key: "ctrl+shift+b"}
	if got := game.displayKey(unbound); got != "" {
		t.Fatalf("displayKey(unbound) = %q, want empty", got)
	}
}

func TestKeymapQuitActuallyTerminates(t *testing.T) {
	game := newProjectGame(t, "")
	game.source.(*fakeSource).events = []input.Event{{Kind: input.KindKeyDown, Key: input.KeyQ, Mods: input.ModCtrl}}
	if err := game.Update(); err == nil {
		t.Fatal("ctrl+q did not terminate")
	}
}

func TestDefaultKeymapCtrlSSaves(t *testing.T) {
	game, path := newEditorGame(t, "abc\n", &recordingClipboard{})
	if err := sendKeys(t, game, "ix<esc>"); err != nil {
		t.Fatal(err)
	}
	game.source.(*fakeSource).events = []input.Event{{Kind: input.KindKeyDown, Key: input.KeyS, Mods: input.ModCtrl}}
	if err := game.Update(); err != nil {
		t.Fatal(err)
	}
	waitClean(t, game, game.session.CurrentBuffer())
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "xabc\n" {
		t.Fatalf("saved = %q, want xabc", data)
	}
}

func TestDefaultKeymapCtrlShiftPOpensPalette(t *testing.T) {
	game := newProjectGame(t, "")
	game.source.(*fakeSource).events = []input.Event{{
		Kind: input.KindKeyDown, Key: input.KeyP, Mods: input.ModCtrl | input.ModShift,
	}}
	if err := game.Update(); err != nil {
		t.Fatal(err)
	}
	if game.palette == nil {
		t.Fatal("ctrl+shift+p did not open the palette")
	}
}
