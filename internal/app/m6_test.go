package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/asimmons91/pico-lsp/pkg/p8scii"

	"github.com/asimmons91/pico-editor/internal/editor"
	"github.com/asimmons91/pico-editor/internal/input"
	"github.com/asimmons91/pico-editor/internal/lsp"
	"github.com/asimmons91/pico-editor/internal/vim"
)

func TestM6ShiftLetterSaveReload(t *testing.T) {
	game, path := newEditorGame(t, "x\n", &recordingClipboard{})
	if err := sendKeys(t, game, "iA<esc>"); err != nil {
		t.Fatal(err)
	}
	if got := game.session.CurrentEditor().Buffer.Lines[0]; got != "█x" {
		t.Fatalf("line = %q, want the Shift+A glyph", got)
	}
	if err := sendKeys(t, game, ":w<enter>"); err != nil {
		t.Fatal(err)
	}
	waitClean(t, game, game.session.CurrentBuffer())
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "█") {
		t.Fatalf("saved file = %q, want the canonical glyph", data)
	}
	if err := sendKeys(t, game, ":e<enter>"); err != nil {
		t.Fatal(err)
	}
	if got := game.session.CurrentEditor().Buffer.Lines[0]; got != "█x" {
		t.Fatalf("reloaded line = %q, want the canonical glyph", got)
	}
}

func TestM6PrintableGlyphCartRoundTrip(t *testing.T) {
	game, _ := newCartGame(t, "pico-8 cartridge // http://www.pico-8.com\nversion 43\n__lua__\n")
	buffer := game.session.CurrentBuffer()
	bytes := make([]byte, 0, 240)
	for b := 0x10; b <= 0xff; b++ {
		bytes = append(bytes, byte(b))
	}
	text := p8scii.Decode(bytes)
	ed := buffer.Editor
	ed.ClearLineRange()
	lastLine := len(ed.Buffer.Lines) - 1
	end := editor.Pos{Line: lastLine, Rune: len([]rune(ed.Buffer.Lines[lastLine]))}
	ed.BeginGroup()
	if _, err := ed.Apply(editor.Edit{Start: editor.Pos{}, End: end, Text: text}); err != nil {
		t.Fatal(err)
	}
	game.session.syncCurrentCart()
	if err := sendKeys(t, game, ":w<enter>"); err != nil {
		t.Fatal(err)
	}
	waitClean(t, game, buffer)
	if err := sendKeys(t, game, ":e<enter>"); err != nil {
		t.Fatal(err)
	}
	reloaded := game.session.CurrentEditor().Buffer.Text()
	if reloaded != text {
		t.Fatalf("reloaded glyph text differs (len %d vs %d)", len(reloaded), len(text))
	}
	if encoded := p8scii.Encode(reloaded); string(encoded) != string(bytes) {
		t.Fatalf("reloaded text did not map back to bytes 0x10-0xff")
	}
}

func TestM6NormalModeShiftCommandsUnaffected(t *testing.T) {
	glyphs := []string{
		"█", "▒", "🐱", "⬇️", "░", "✽", "●", "♥", "☉", "웃", "⌂", "⬅️", "😐",
		"♪", "🅾️", "◆", "…", "➡️", "★", "⧗", "⬆️", "ˇ", "∧", "❎", "▤", "▥",
	}
	for _, keys := range []string{"J", "K", "P", "X", "D", "C", "Y", "gg", "G", "$"} {
		game, _ := newEditorGame(t, "one\ntwo\nthree\n", &recordingClipboard{})
		if err := sendKeys(t, game, keys); err != nil {
			t.Fatalf("%s: %v", keys, err)
		}
		text := game.session.CurrentEditor().Buffer.Text()
		for _, glyph := range glyphs {
			if strings.Contains(text, glyph) {
				t.Fatalf("%s inserted P8SCII glyph %q: %q", keys, glyph, text)
			}
		}
	}
}

func TestM6PunyModeRoundTrip(t *testing.T) {
	game, path := newEditorGame(t, "x\n", &recordingClipboard{})
	if err := sendKeys(t, game, "i<c-p>my_var<esc>"); err != nil {
		t.Fatal(err)
	}
	if got := game.session.CurrentEditor().Buffer.Lines[0]; got != "MY_VARx" {
		t.Fatalf("line = %q, want MY_VARx", got)
	}
	if err := sendKeys(t, game, ":w<enter>"); err != nil {
		t.Fatal(err)
	}
	waitClean(t, game, game.session.CurrentBuffer())
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "MY_VAR") {
		t.Fatalf("saved file = %q, want MY_VAR", data)
	}
}

func TestM6PunyIdentifierIsAnalyzed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "puny_analysis.lua")
	if err := os.WriteFile(path, []byte("print(1)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	game := newLSPGame(t, path)
	waitLSPReady(t, game)
	waitLSPTokens(t, game)
	if err := sendKeys(t, game, "i<c-p>my_var<esc>"); err != nil {
		t.Fatal(err)
	}
	buffer := game.session.CurrentBuffer()
	documentURI := game.session.ActiveDocumentURI()
	eventually(t, func() bool {
		updateGame(t, game)
		return game.session.LSPTokens[documentURI].Version >= buffer.Editor.Buffer.Version
	})
	for _, diagnostic := range game.session.LSPDiagnostics[documentURI] {
		if diagnostic.Severity == lsp.SeverityError {
			t.Fatalf("uppercase identifier produced an error: %+v", diagnostic)
		}
	}
	if got := buffer.Editor.Buffer.Lines[0]; !strings.Contains(got, "MY_VAR") {
		t.Fatalf("line = %q, want the saved uppercase spelling", got)
	}
}

func TestM6PunyAndKanaModesAreExclusive(t *testing.T) {
	game, _ := newEditorGame(t, "x\n", &recordingClipboard{})
	if err := sendKeys(t, game, "i<c-p>"); err != nil {
		t.Fatal(err)
	}
	if game.session.P8.Mode != input.P8ModePuny || game.session.P8.ModeLabel() != "PUNY" {
		t.Fatalf("mode = %v label = %q, want PUNY", game.session.P8.Mode, game.session.P8.ModeLabel())
	}
	if err := sendKeys(t, game, "<c-j>"); err != nil {
		t.Fatal(err)
	}
	if game.session.P8.Mode != input.P8ModeHiragana || game.session.P8.ModeLabel() != "HIRA" {
		t.Fatalf("mode = %v label = %q, want HIRA", game.session.P8.Mode, game.session.P8.ModeLabel())
	}
	if err := sendKeys(t, game, "<c-k>"); err != nil {
		t.Fatal(err)
	}
	if game.session.P8.Mode != input.P8ModeKatakana || game.session.P8.ModeLabel() != "KATA" {
		t.Fatalf("mode = %v label = %q, want KATA", game.session.P8.Mode, game.session.P8.ModeLabel())
	}
	if err := sendKeys(t, game, "<esc>"); err != nil {
		t.Fatal(err)
	}
	if game.session.P8.Mode != input.P8ModeDefault || game.session.P8.ModeLabel() != "" {
		t.Fatalf("Esc left mode = %v label = %q", game.session.P8.Mode, game.session.P8.ModeLabel())
	}
}

func TestM6KanaInsertionAndShiftDigits(t *testing.T) {
	tests := []struct {
		keys string
		want string
	}{
		{"i<c-j>ka<esc>", "か"},
		{"i<c-j>kya<esc>", "きゃ"},
		{"i<c-j>nnka<esc>", "んか"},
		{"i<c-k>ka<esc>", "カ"},
		{"i<c-k>!<esc>", "▮"},
		{"i<c-k>)<esc>", "◝"},
		{"i<c-j>)<esc>", "◝"},
	}
	for _, tc := range tests {
		game, _ := newEditorGame(t, "x\n", &recordingClipboard{})
		if err := sendKeys(t, game, tc.keys); err != nil {
			t.Fatalf("%q: %v", tc.keys, err)
		}
		if got := game.session.CurrentEditor().Buffer.Lines[0]; got != tc.want+"x" {
			t.Errorf("%q -> %q, want %q", tc.keys, got, tc.want+"x")
		}
	}
}

func TestM6GlyphPickerFiltersAndInserts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "picker.lua")
	if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	game := newLSPGame(t, path)
	waitLSPReady(t, game)
	eventually(t, func() bool {
		updateGame(t, game)
		return len(game.session.LSPGlyphs) > 0
	})
	if err := sendKeys(t, game, "<c-S-g>"); err != nil {
		t.Fatal(err)
	}
	picker := game.session.GlyphPicker
	if picker == nil {
		t.Fatal("glyph picker did not open")
	}
	if err := sendKeys(t, game, "heart"); err != nil {
		t.Fatal(err)
	}
	if len(picker.Entries) == 0 {
		t.Fatal("picker filter returned no entries")
	}
	found := false
	for _, entry := range picker.Entries {
		if entry.Name == "heart" {
			found = true
		}
	}
	if !found {
		t.Fatalf("filtered entries = %+v, want heart", picker.Entries)
	}
	if err := sendKeys(t, game, "<enter>"); err != nil {
		t.Fatal(err)
	}
	if game.session.GlyphPicker != nil {
		t.Fatal("picker did not close after insertion")
	}
	if got := game.session.CurrentEditor().Buffer.Lines[0]; got != "♥x" {
		t.Fatalf("line = %q, want the inserted heart", got)
	}
	if game.session.Vim.Mode != vim.ModeInsert {
		t.Fatalf("normal-mode insertion mode = %v, want INSERT", game.session.Vim.Mode)
	}
}

func TestM6GlyphPickerReplacesVisualSelection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "picker_visual.lua")
	if err := os.WriteFile(path, []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	game := newLSPGame(t, path)
	waitLSPReady(t, game)
	eventually(t, func() bool {
		updateGame(t, game)
		return len(game.session.LSPGlyphs) > 0
	})
	if err := sendKeys(t, game, "vll<c-S-g>"); err != nil {
		t.Fatal(err)
	}
	if game.session.GlyphPicker == nil {
		t.Fatal("glyph picker did not open in visual mode")
	}
	if err := sendKeys(t, game, "heart<enter>"); err != nil {
		t.Fatal(err)
	}
	if got := game.session.CurrentEditor().Buffer.Lines[0]; got != "♥" {
		t.Fatalf("line = %q, want the glyph to replace the selection", got)
	}
	if game.session.Vim.Mode != vim.ModeNormal {
		t.Fatalf("mode = %v, want NORMAL after visual insertion", game.session.Vim.Mode)
	}
}

func TestM6GlyphPickerByteAndCategoryFilter(t *testing.T) {
	picker := NewGlyphPicker([]lsp.Glyph{
		{Name: "heart", Text: "♥", Byte: 0x87, Category: "symbol", Aliases: []string{"p8-heart"}},
		{Name: "hiragana_a", Text: "あ", Byte: 0x9a, Category: "kana"},
	})
	picker.Query = "0x87"
	picker.Refilter()
	if len(picker.Entries) != 1 || picker.Entries[0].Name != "heart" {
		t.Fatalf("byte filter = %+v", picker.Entries)
	}
	picker.Query = "kana"
	picker.Refilter()
	if len(picker.Entries) != 1 || picker.Entries[0].Name != "hiragana_a" {
		t.Fatalf("category filter = %+v", picker.Entries)
	}
	picker.Query = "p8-heart"
	picker.Refilter()
	if len(picker.Entries) != 1 || picker.Entries[0].Name != "heart" {
		t.Fatalf("alias filter = %+v", picker.Entries)
	}
}

func TestM6CtrlPTogglesPunyWithCompletionOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "completion_puny.lua")
	if err := os.WriteFile(path, []byte("print(1)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	game := newLSPGame(t, path)
	waitLSPReady(t, game)
	waitLSPTokens(t, game)
	if err := sendKeys(t, game, "1Go p8_le"); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		updateGame(t, game)
		return game.session.Completion != nil
	})
	if err := sendKeys(t, game, "<c-p>"); err != nil {
		t.Fatal(err)
	}
	if game.session.P8.Mode != input.P8ModePuny {
		t.Fatalf("mode = %v, want PUNY", game.session.P8.Mode)
	}
	if game.session.Completion == nil {
		t.Fatal("Ctrl+P closed the completion popup")
	}
	if err := sendKeys(t, game, "<esc>"); err != nil {
		t.Fatal(err)
	}
	if game.session.Completion != nil || game.session.Vim.Mode != vim.ModeInsert {
		t.Fatalf("first Esc should close completion only: completion=%v mode=%v", game.session.Completion, game.session.Vim.Mode)
	}
}

func TestM6GlyphPickerInsertMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "picker_insert.lua")
	if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	game := newLSPGame(t, path)
	waitLSPReady(t, game)
	eventually(t, func() bool {
		updateGame(t, game)
		return len(game.session.LSPGlyphs) > 0
	})
	if err := sendKeys(t, game, "i<c-S-g>heart<enter>"); err != nil {
		t.Fatal(err)
	}
	if got := game.session.CurrentEditor().Buffer.Lines[0]; got != "♥x" {
		t.Fatalf("line = %q, want insert-mode picker glyph", got)
	}
	if game.session.Vim.Mode != vim.ModeInsert {
		t.Fatalf("mode = %v, want INSERT", game.session.Vim.Mode)
	}
}

func TestM6VisualShiftLetterIsNotConverted(t *testing.T) {
	game, _ := newEditorGame(t, "abc\n", &recordingClipboard{})
	if err := sendKeys(t, game, "vA<esc>"); err != nil {
		t.Fatal(err)
	}
	if got := game.session.CurrentEditor().Buffer.Text(); got != "abc\n" {
		t.Fatalf("buffer = %q, want visual Shift+A to stay a Vim command", got)
	}
}

func TestM6CommandLineShiftLetterIsNotConverted(t *testing.T) {
	game, _ := newEditorGame(t, "abc\n", &recordingClipboard{})
	if err := sendKeys(t, game, ":A<esc>"); err != nil {
		t.Fatal(err)
	}
	if got := game.session.CurrentEditor().Buffer.Text(); got != "abc\n" {
		t.Fatalf("buffer = %q, want command-line Shift+A to stay text", got)
	}
}

func TestM6GlyphCatalogCoversPrintableBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "glyph_catalog.lua")
	if err := os.WriteFile(path, []byte("print(1)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	game := newLSPGame(t, path)
	waitLSPReady(t, game)
	eventually(t, func() bool {
		updateGame(t, game)
		return len(game.session.LSPGlyphs) > 0
	})
	seen := make(map[int]int, 240)
	for _, glyph := range game.session.LSPGlyphs {
		if glyph.Context == "string" {
			continue
		}
		seen[glyph.Byte]++
	}
	for b := 0x10; b <= 0xff; b++ {
		if seen[b] != 1 {
			t.Errorf("printable byte 0x%02x has %d catalog entries, want 1", b, seen[b])
		}
	}
}

func TestM6PickerWinsOverFocusedTree(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tree_picker.lua")
	if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	game := newLSPGame(t, path)
	waitLSPReady(t, game)
	eventually(t, func() bool {
		updateGame(t, game)
		return len(game.session.LSPGlyphs) > 0
	})
	game.session.Tree.Visible = true
	game.session.Tree.Focused = true
	if err := sendKeys(t, game, "<c-S-g>"); err != nil {
		t.Fatal(err)
	}
	if game.session.GlyphPicker == nil {
		t.Fatal("picker did not open over the focused tree")
	}
	if game.session.Tree.Focused {
		t.Fatal("tree kept input focus under the picker")
	}
	if err := sendKeys(t, game, "<esc>"); err != nil {
		t.Fatal(err)
	}
	if game.session.GlyphPicker != nil {
		t.Fatal("Esc did not close the picker")
	}
}

func TestM6HelpClearsPendingOperator(t *testing.T) {
	game, _ := newEditorGame(t, "one two\n", &recordingClipboard{})
	if err := sendKeys(t, game, "d"); err != nil {
		t.Fatal(err)
	}
	if err := sendKeys(t, game, "<f1>"); err != nil {
		t.Fatal(err)
	}
	if game.session.Help == nil {
		t.Fatal("F1 did not open help")
	}
	if err := sendKeys(t, game, "<esc>"); err != nil {
		t.Fatal(err)
	}
	if err := sendKeys(t, game, "w"); err != nil {
		t.Fatal(err)
	}
	if got := game.session.CurrentEditor().Buffer.Lines[0]; got != "one two" {
		t.Fatalf("line = %q, pending operator survived help", got)
	}
}

func TestM6PendingGlyphCatalogCancelsOnInput(t *testing.T) {
	game, _ := newEditorGame(t, "x\n", &recordingClipboard{})
	game.session.glyphPickerPending = true
	if err := sendKeys(t, game, "<esc>"); err != nil {
		t.Fatal(err)
	}
	if game.session.glyphPickerPending {
		t.Fatal("input did not cancel the deferred picker open")
	}
}

func TestM6GlyphPickerRequiresLSP(t *testing.T) {
	game, _ := newEditorGame(t, "x\n", &recordingClipboard{})
	if !game.session.OpenGlyphPicker() {
		t.Fatal("OpenGlyphPicker reported no change")
	}
	if game.session.GlyphPicker != nil {
		t.Fatal("picker opened without pico-lsp")
	}
	if !strings.Contains(game.session.Status, "pico-lsp") {
		t.Fatalf("status = %q, want an LSP warning", game.session.Status)
	}
}

func TestM6InputAndGlyphExCommands(t *testing.T) {
	game, _ := newEditorGame(t, "x\n", &recordingClipboard{})
	if err := sendKeys(t, game, ":input puny<enter>"); err != nil {
		t.Fatal(err)
	}
	if game.session.Vim.Mode != vim.ModeInsert || game.session.P8.Mode != input.P8ModePuny {
		t.Fatalf("mode = %v input = %v, want INSERT/PUNY", game.session.Vim.Mode, game.session.P8.Mode)
	}
	if err := sendKeys(t, game, "<esc>:input off<enter>"); err != nil {
		t.Fatal(err)
	}
	if game.session.P8.Mode != input.P8ModeDefault {
		t.Fatalf("input mode = %v after :input off", game.session.P8.Mode)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "ex_picker.lua")
	if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lspGame := newLSPGame(t, path)
	waitLSPReady(t, lspGame)
	eventually(t, func() bool {
		updateGame(t, lspGame)
		return len(lspGame.session.LSPGlyphs) > 0
	})
	if err := sendKeys(t, lspGame, ":glyph<enter>"); err != nil {
		t.Fatal(err)
	}
	if lspGame.session.GlyphPicker == nil {
		t.Fatal(":glyph did not open the picker")
	}
}

func TestM6CommandPaletteListsAndRuns(t *testing.T) {
	game, _ := newEditorGame(t, "x\n", &recordingClipboard{})
	if err := sendKeys(t, game, "<c-S-p>"); err != nil {
		t.Fatal(err)
	}
	if game.palette == nil {
		t.Fatal("palette did not open")
	}
	if err := sendKeys(t, game, "help input"); err != nil {
		t.Fatal(err)
	}
	if err := sendKeys(t, game, "<enter>"); err != nil {
		t.Fatal(err)
	}
	if game.palette != nil {
		t.Fatal("palette did not close")
	}
	if game.session.Help == nil || game.session.Help.Title != "Help: input" {
		t.Fatalf("help = %+v, want Help: input", game.session.Help)
	}
}

func TestM6CommandPaletteOpensGlyphPicker(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "palette_picker.lua")
	if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	game := newLSPGame(t, path)
	waitLSPReady(t, game)
	eventually(t, func() bool {
		updateGame(t, game)
		return len(game.session.LSPGlyphs) > 0
	})
	if err := sendKeys(t, game, "<c-S-p>glyph pick<enter>"); err != nil {
		t.Fatal(err)
	}
	if game.palette != nil {
		t.Fatal("palette did not close after running glyph.pick")
	}
	if game.session.GlyphPicker == nil {
		t.Fatal("glyph.pick palette command did not open the picker")
	}
}

func TestM6PaletteHidesDisabledCommandsUntilID(t *testing.T) {
	game, _ := newEditorGame(t, "x\n", &recordingClipboard{})
	game.openPalette()
	game.palette.Query = "glyph"
	game.refilterPalette()
	if len(game.palette.Matches) != 0 {
		t.Fatalf("disabled glyph command matched a title query: %+v", game.palette.Matches)
	}
	game.palette.Query = "glyph.pick"
	game.refilterPalette()
	if len(game.palette.Matches) != 1 {
		t.Fatalf("disabled glyph command did not match its ID: %+v", game.palette.Matches)
	}
}

func TestM6CommandRegistry(t *testing.T) {
	game, _ := newEditorGame(t, "x\n", &recordingClipboard{})
	seen := make(map[string]bool, len(game.commands))
	for _, command := range game.commands {
		if command.ID == "" || command.Title == "" {
			t.Errorf("incomplete command: %+v", command)
		}
		if seen[command.ID] {
			t.Errorf("duplicate command ID %q", command.ID)
		}
		seen[command.ID] = true
	}
	for _, id := range []string{
		"file.save", "file.close", "file.reload", "buffer.next", "buffer.prev",
		"buffer.list", "cart.tab.next", "cart.tab.prev", "tree.toggle",
		"tree.refresh", "palette.show", "help.open", "help.input", "diag.list",
		"messages.show", "lsp.restart", "glyph.pick", "input.puny",
		"config.reload", "app.quit",
	} {
		if !seen[id] {
			t.Errorf("missing command %q", id)
		}
	}
}

func TestCompletionPrefixIncludesP8Dash(t *testing.T) {
	buffer := editor.New("")
	buffer.Lines = []string{"p8-left"}
	ed := editor.NewEditor(buffer)
	ed.SetCursor(editor.Pos{Line: 0, Rune: len("p8-left")})
	session := &Session{}
	if got := session.completionPrefix(ed); got != "p8-left" {
		t.Fatalf("completionPrefix = %q, want p8-left", got)
	}
}

func TestM6GlyphCompletionInsertsCanonicalText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "glyph_completion.lua")
	if err := os.WriteFile(path, []byte("print(1)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	game := newLSPGame(t, path)
	waitLSPReady(t, game)
	waitLSPTokens(t, game)
	if err := sendKeys(t, game, "1Gop8_le"); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		updateGame(t, game)
		return game.session.Completion != nil
	})
	popup := game.session.Completion
	if popup == nil {
		t.Fatal("completion did not open for p8_le")
	}
	selected := -1
	for index, item := range popup.Items {
		if strings.Contains(item.Label, "p8_left") {
			selected = index
			break
		}
	}
	if selected < 0 {
		t.Fatalf("completion items = %+v, want p8_left", popup.Items)
	}
	popup.Selected = selected
	if err := sendKeys(t, game, "<enter>"); err != nil {
		t.Fatal(err)
	}
	line := game.session.CurrentEditor().Buffer.Lines[1]
	if !strings.Contains(line, "⬅️") {
		t.Fatalf("line = %q, want canonical left glyph", line)
	}
	if strings.Contains(line, "p8_le") {
		t.Fatalf("line = %q, completion trigger was not replaced", line)
	}
}

func TestM6GlyphCompletionAliasAndKana(t *testing.T) {
	tests := []struct {
		name   string
		typed  string
		label  string
		expect string
	}{
		{"alias", "p8-left", "p8_left", "⬅️"},
		{"kana", "p8_hiragana_a", "p8_hiragana_a", "あ"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "glyph_completion.lua")
			if err := os.WriteFile(path, []byte("print(1)\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			game := newLSPGame(t, path)
			waitLSPReady(t, game)
			waitLSPTokens(t, game)
			if err := sendKeys(t, game, "1Go"+tc.typed); err != nil {
				t.Fatal(err)
			}
			if got := game.session.CurrentEditor().Buffer.Lines[1]; got != tc.typed {
				t.Fatalf("typed line = %q, want %q", got, tc.typed)
			}
			eventually(t, func() bool {
				updateGame(t, game)
				return game.session.Completion != nil
			})
			popup := game.session.Completion
			if popup == nil {
				t.Fatal("completion did not open")
			}
			selected := -1
			for index, item := range popup.Items {
				if strings.HasSuffix(item.Label, " "+tc.label) {
					selected = index
					break
				}
			}
			if selected < 0 {
				t.Fatalf("completion items = %+v, want %s", popup.Items, tc.label)
			}
			popup.Selected = selected
			if err := sendKeys(t, game, "<enter>"); err != nil {
				t.Fatal(err)
			}
			line := game.session.CurrentEditor().Buffer.Lines[1]
			if !strings.Contains(line, tc.expect) {
				t.Fatalf("line = %q, want canonical %q", line, tc.expect)
			}
		})
	}
}

func TestM6GlyphCompletionCoverage(t *testing.T) {
	tests := []struct {
		typed string
		label string
		text  string
	}{
		{"p8_left", "p8_left", "⬅️"},
		{"p8_right", "p8_right", "➡️"},
		{"p8_up", "p8_up", "⬆️"},
		{"p8_down", "p8_down", "⬇️"},
		{"p8_heart", "p8_heart", "♥"},
		{"p8_hiragana_a", "p8_hiragana_a", "あ"},
		{"p8-left", "p8_left", "⬅️"},
	}
	for _, tc := range tests {
		t.Run(tc.typed, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "glyph_coverage.lua")
			if err := os.WriteFile(path, []byte("print(1)\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			game := newLSPGame(t, path)
			waitLSPReady(t, game)
			waitLSPTokens(t, game)
			if err := sendKeys(t, game, "1Go"+tc.typed); err != nil {
				t.Fatal(err)
			}
			eventually(t, func() bool {
				updateGame(t, game)
				return game.session.Completion != nil
			})
			popup := game.session.Completion
			selected := -1
			for index, item := range popup.Items {
				if strings.HasSuffix(item.Label, " "+tc.label) {
					selected = index
					break
				}
			}
			if selected < 0 {
				t.Fatalf("completion items = %+v, want %s", popup.Items, tc.label)
			}
			popup.Selected = selected
			if err := sendKeys(t, game, "<enter>"); err != nil {
				t.Fatal(err)
			}
			if line := game.session.CurrentEditor().Buffer.Lines[1]; !strings.Contains(line, tc.text) {
				t.Fatalf("line = %q, want %q", line, tc.text)
			}
		})
	}
}

func TestM6SystemClipboardBindings(t *testing.T) {
	clipboard := &recordingClipboard{text: "pasted\n"}
	game, _ := newEditorGame(t, "one\n", clipboard)
	if err := sendKeys(t, game, "i<c-S-v>"); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		updateGame(t, game)
		return strings.Contains(game.session.CurrentEditor().Buffer.Text(), "pasted")
	})
	if !strings.Contains(game.session.CurrentEditor().Buffer.Text(), "pasted") {
		t.Fatalf("insert paste = %q", game.session.CurrentEditor().Buffer.Text())
	}

	clipboard.text = ""
	game, _ = newEditorGame(t, "one\ntwo\n", clipboard)
	if err := sendKeys(t, game, "vll<c-S-c>"); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		updateGame(t, game)
		return clipboard.Text() == "one"
	})
	if got := clipboard.Text(); got != "one" {
		t.Fatalf("visual copy = %q, want one", got)
	}
}

func TestM6ClipboardBlockAndLinewiseCopy(t *testing.T) {
	clipboard := &recordingClipboard{}
	game, _ := newEditorGame(t, "a\nbbb\ncc\n", clipboard)
	if err := sendKeys(t, game, "Vj\"+y"); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		updateGame(t, game)
		return clipboard.Text() != ""
	})
	if got := clipboard.Text(); got != "a\nbbb\n" {
		t.Fatalf("linewise copy = %q, want a\\nbbb\\n", got)
	}

	clipboard = &recordingClipboard{}
	game, _ = newEditorGame(t, "a\nbbb\ncc\n", clipboard)
	if err := sendKeys(t, game, "<c-v>2jl\"+y"); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		updateGame(t, game)
		return clipboard.Text() != ""
	})
	if got := clipboard.Text(); got != "a \nbb\ncc" {
		t.Fatalf("block copy = %q, want padded a\\nbb\\ncc", got)
	}
}

func TestM6ClipboardStripsBOM(t *testing.T) {
	clipboard := &recordingClipboard{text: "\ufeffpasted\n"}
	game, _ := newEditorGame(t, "x\n", clipboard)
	if err := sendKeys(t, game, "i<c-S-v>"); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		updateGame(t, game)
		return strings.Contains(game.session.CurrentEditor().Buffer.Text(), "pasted")
	})
	if strings.Contains(game.session.CurrentEditor().Buffer.Text(), "\ufeff") {
		t.Fatalf("paste kept the BOM: %q", game.session.CurrentEditor().Buffer.Text())
	}
}

func TestM6ClipboardPasteInCart(t *testing.T) {
	game, path := newCartGame(t, fullCart)
	clipboard := &recordingClipboard{text: "-- pasted\n"}
	game.clipboard = clipboard
	if err := sendKeys(t, game, "i<c-S-v>"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		updateGame(t, game)
		if strings.Contains(game.session.CurrentEditor().Buffer.Text(), "-- pasted") {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if !strings.Contains(game.session.CurrentEditor().Buffer.Text(), "-- pasted") {
		t.Fatalf("paste missing: buffer=%q status=%q reads=%d mode=%v", game.session.CurrentEditor().Buffer.Text(), game.session.Status, clipboard.Reads(), game.session.Vim.Mode)
	}
	if err := sendKeys(t, game, "<esc>:w<enter>"); err != nil {
		t.Fatal(err)
	}
	waitClean(t, game, game.session.CurrentBuffer())
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "-- pasted") {
		t.Fatalf("saved cart = %q, want pasted text", data)
	}
}

func TestM6ClipboardUnknownRuneWarning(t *testing.T) {
	clipboard := &recordingClipboard{text: "a😀b\n"}
	game, _ := newEditorGame(t, "x\n", clipboard)
	if err := sendKeys(t, game, "i<c-S-v>"); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		updateGame(t, game)
		return strings.Contains(game.session.CurrentEditor().Buffer.Text(), "😀")
	})
	if !strings.Contains(game.session.Status, "unknown") {
		t.Fatalf("status = %q, want an unknown-rune warning", game.session.Status)
	}
}
