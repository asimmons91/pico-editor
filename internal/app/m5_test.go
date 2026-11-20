package app

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/asimmons91/pico-editor/internal/lsp"
	"github.com/asimmons91/pico-editor/internal/render"
)

func newLSPGame(t *testing.T, path string) *Game {
	t.Helper()
	game := New(Config{
		Logger:    slog.New(slog.DiscardHandler),
		File:      path,
		EnableLSP: true,
		Version:   "test",
	})
	if err := game.StartupError(); err != nil {
		t.Fatalf("StartupError: %v", err)
	}
	game.source = &fakeSource{}
	t.Cleanup(game.stopLSP)
	return game
}

func updateGame(t *testing.T, game *Game) {
	t.Helper()
	if err := game.Update(); err != nil && err != ebiten.Termination {
		t.Fatalf("Update: %v", err)
	}
}

func waitLSPReady(t *testing.T, game *Game) {
	t.Helper()
	eventually(t, func() bool {
		updateGame(t, game)
		return game.session.LSPStatus == lsp.StatusReady
	})
}

func waitLSPTokens(t *testing.T, game *Game) lsp.TokenSet {
	t.Helper()
	documentURI := game.session.ActiveDocumentURI()
	eventually(t, func() bool {
		updateGame(t, game)
		return len(game.session.LSPTokens[documentURI].Tokens) > 0
	})
	return game.session.LSPTokens[documentURI]
}

func TestLSPSemanticTokensAndCartStatus(t *testing.T) {
	dir := t.TempDir()
	path := copyFixtureFile(t, "smoke.p8", dir, "smoke.p8")
	game := newLSPGame(t, path)
	waitLSPReady(t, game)
	tokens := waitLSPTokens(t, game)
	seen := make(map[string]bool)
	for _, token := range tokens.Tokens {
		seen[token.Type] = true
	}
	for _, want := range []string{"keyword", "string", "number", "comment", "variable", "function"} {
		if !seen[want] {
			t.Errorf("missing semantic token type %q in %v", want, seen)
		}
	}
	documentURI := game.session.ActiveDocumentURI()
	eventually(t, func() bool {
		updateGame(t, game)
		info, ok := game.session.LSPCartInfo[documentURI]
		return ok && !info.Unavailable()
	})
	segments := game.session.lspStatusSegments()
	joined := ""
	for _, segment := range segments {
		joined += segment.text + " "
	}
	if !strings.Contains(joined, "T") || !strings.Contains(joined, "LSP") {
		t.Fatalf("status segments = %q", joined)
	}
}

func TestLSPCompletionAccept(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "completion.lua")
	if err := os.WriteFile(path, []byte("function foo()\nend\nfoo()\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	game := newLSPGame(t, path)
	waitLSPReady(t, game)
	waitLSPTokens(t, game)
	if err := sendKeys(t, game, "3Gipri<c-space>"); err != nil {
		t.Fatalf("Update completion: %v", err)
	}
	eventually(t, func() bool {
		updateGame(t, game)
		return game.session.Completion != nil
	})
	popup := game.session.Completion
	if popup == nil || len(popup.Items) == 0 {
		t.Fatal("completion popup did not open")
	}
	selected := -1
	for index, item := range popup.Items {
		if strings.HasPrefix(item.Label, "print") {
			selected = index
			break
		}
	}
	if selected < 0 {
		t.Fatalf("completion items missing print: %+v", popup.Items)
	}
	popup.Selected = selected
	if err := sendKeys(t, game, "<enter><esc>"); err != nil {
		t.Fatalf("Update accept: %v", err)
	}
	if got := game.session.CurrentEditor().Buffer.Lines[2]; !strings.HasPrefix(got, "print") {
		t.Fatalf("line = %q, want a print completion", got)
	}
}

func TestLSPHoverAndDefinition(t *testing.T) {
	dir := copyFixtureDir(t, "navigation")
	path := filepath.Join(dir, "cart.p8")
	game := newLSPGame(t, path)
	waitLSPReady(t, game)
	waitLSPTokens(t, game)
	if err := sendKeys(t, game, "3GK"); err != nil {
		t.Fatalf("Update hover: %v", err)
	}
	eventually(t, func() bool {
		updateGame(t, game)
		return game.session.Hover != nil
	})
	if game.session.Hover.Text == "" {
		t.Fatal("hover popup was empty")
	}
	if err := sendKeys(t, game, "<esc>"); err != nil {
		t.Fatalf("Update close hover: %v", err)
	}
	if game.session.Hover != nil {
		t.Fatal("hover popup did not close")
	}

	if err := sendKeys(t, game, "3G0fmgd"); err != nil {
		t.Fatalf("Update definition: %v", err)
	}
	eventually(t, func() bool {
		updateGame(t, game)
		current := game.session.CurrentBuffer()
		return current != nil && filepath.Base(current.Editor.Buffer.Path) == "player.lua"
	})
	if got := game.session.CurrentEditor().Cursor.Pos.Line; got != 0 {
		t.Fatalf("definition cursor line = %d, want 0", got)
	}
	if err := sendKeys(t, game, "<c-o>"); err != nil {
		t.Fatalf("Update jump back: %v", err)
	}
	eventually(t, func() bool {
		updateGame(t, game)
		current := game.session.CurrentBuffer()
		return current != nil && filepath.Base(current.Editor.Buffer.Path) == "cart.p8"
	})
}

func TestLSPDiagnosticsAndDiagList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.p8")
	const content = "pico-8 cartridge // pico-8.com\nversion 42\n__lua__\n---@type number\nlocal value = \"bad\"\nprint(value)\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	game := newLSPGame(t, path)
	waitLSPReady(t, game)
	documentURI := game.session.ActiveDocumentURI()
	eventually(t, func() bool {
		updateGame(t, game)
		errors, warnings := game.session.DiagnosticCounts(documentURI)
		return errors+warnings > 0
	})
	if err := sendKeys(t, game, ":diag<enter>"); err != nil {
		t.Fatalf("Update diag: %v", err)
	}
	if game.session.DiagList == nil || len(game.session.DiagList.Items) == 0 {
		t.Fatal(":diag did not open the diagnostic list")
	}
	if err := sendKeys(t, game, "<enter>"); err != nil {
		t.Fatalf("Update diag jump: %v", err)
	}
	if game.session.DiagList != nil {
		t.Fatal("diagnostic list did not close after Enter")
	}
}

func TestLSPDrawnHighlightingAndSignature(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "signature.lua")
	if err := os.WriteFile(path, []byte("local x = 1\nprint(x)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	game := newLSPGame(t, path)
	waitLSPReady(t, game)
	waitLSPTokens(t, game)
	surface := newTestSurface()
	game.session.Draw(surface, true)
	if !surfaceHasColor(surface, 14) {
		t.Fatal("surface has no keyword-colored semantic token")
	}
	if err := sendKeys(t, game, "2G0f(i"); err != nil {
		t.Fatalf("Update signature: %v", err)
	}
	game.session.requestSignature()
	eventually(t, func() bool {
		updateGame(t, game)
		return game.session.Signature != nil
	})
	if len(game.session.Signature.Help.Signatures) == 0 {
		t.Fatal("signature help had no signatures")
	}
	game.session.Draw(surface, true)
}

func TestLSPDiagnosticUnderlineIsDrawn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.p8")
	const content = "pico-8 cartridge // pico-8.com\nversion 42\n__lua__\n---@type number\nlocal value = \"bad\"\nprint(value)\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	game := newLSPGame(t, path)
	waitLSPReady(t, game)
	documentURI := game.session.ActiveDocumentURI()
	eventually(t, func() bool {
		updateGame(t, game)
		errors, warnings := game.session.DiagnosticCounts(documentURI)
		return errors+warnings > 0
	})
	surface := newTestSurface()
	game.session.Draw(surface, true)
	if !surfaceHasColor(surface, 8) && !surfaceHasColor(surface, 9) {
		t.Fatal("surface has no diagnostic underline color")
	}
}

func TestLSPDefinitionAcrossCartTabs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tabs.p8")
	const content = "pico-8 cartridge // pico-8.com\nversion 42\n__lua__\nfunction helper()\n return 1\nend\n-->8 second\nhelper()\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	game := newLSPGame(t, path)
	waitLSPReady(t, game)
	waitLSPTokens(t, game)
	if err := sendKeys(t, game, ":tabn<enter>Ggd"); err != nil {
		t.Fatalf("Update definition: %v", err)
	}
	buffer := game.session.CurrentBuffer()
	eventually(t, func() bool {
		updateGame(t, game)
		buffer = game.session.CurrentBuffer()
		return buffer.ActiveTab == 0 && buffer.Editor.Cursor.Pos.Line == 0
	})
}

func TestLSPRestartKeepsEditorUsable(t *testing.T) {
	dir := copyFixtureDir(t, "navigation")
	path := filepath.Join(dir, "cart.p8")
	game := newLSPGame(t, path)
	waitLSPReady(t, game)
	waitLSPTokens(t, game)
	if err := sendKeys(t, game, ":lsp restart<enter>"); err != nil {
		t.Fatalf("Update restart: %v", err)
	}
	eventually(t, func() bool {
		updateGame(t, game)
		return game.session.LSPStatus == lsp.StatusReady
	})
	waitLSPTokens(t, game)
	before := game.session.CurrentEditor().Buffer.Text()
	if err := sendKeys(t, game, "x"); err != nil {
		t.Fatalf("Update edit after restart: %v", err)
	}
	if game.session.CurrentEditor().Buffer.Text() == before {
		t.Fatal("editor did not remain usable after restart")
	}
}

func newTestSurface() *render.Surface {
	return render.NewSurface(CanvasWidth, CanvasHeight)
}

func surfaceHasColor(surface *render.Surface, color uint8) bool {
	for _, pixel := range surface.Pix {
		if pixel == color {
			return true
		}
	}
	return false
}

func copyFixtureFile(t *testing.T, name, dir, target string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "lsp", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, target)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func copyFixtureDir(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join("..", "lsp", "testdata", name)
	err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dir, relative)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
