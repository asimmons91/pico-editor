package app

import (
	"bytes"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/asimmons91/pico-editor/internal/editor"
	"github.com/asimmons91/pico-editor/internal/lsp"
	"github.com/asimmons91/pico-editor/internal/render"
)

func screenshotGame(t *testing.T) *Game {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "game.p8")
	cart := "pico-8 cartridge // pico-8.com\nversion 42\n__lua__\n" +
		"function _update()\n" +
		"  if btn(0) then x-=1 end\n" +
		"  for i=1,3 do sfx(i) end\n" +
		"end\n" +
		"-->8 util\n" +
		"local label = \"héllo\"\n" +
		"__gfx__\n0000\n"
	if err := os.WriteFile(path, []byte(cart), 0o644); err != nil {
		t.Fatal(err)
	}
	game := New(Config{
		Logger:    slog.New(slog.DiscardHandler),
		Clipboard: &recordingClipboard{},
		Now:       func() time.Time { return time.Unix(1700000000, 0) },
		File:      path,
	})
	if err := game.StartupError(); err != nil {
		t.Fatal(err)
	}
	session := game.session
	buffer := session.CurrentBuffer()
	session.LSP = lsp.New(lsp.Options{})
	session.LSPStatus = lsp.StatusReady
	uri := lsp.URIForPath(buffer.Editor.Buffer.Path)
	session.LSPTokens[uri] = lsp.TokenSet{URI: uri, ByLine: map[int][]lsp.Token{
		0: {
			{Line: 0, StartRune: 0, EndRune: 8, Type: "keyword"},
			{Line: 0, StartRune: 9, EndRune: 18, Type: "function"},
		},
		1: {
			{Line: 1, StartRune: 2, EndRune: 4, Type: "keyword"},
			{Line: 1, StartRune: 5, EndRune: 8, Type: "function"},
		},
		2: {
			{Line: 2, StartRune: 2, EndRune: 5, Type: "keyword"},
			{Line: 2, StartRune: 10, EndRune: 11, Type: "number"},
			{Line: 2, StartRune: 13, EndRune: 16, Type: "function"},
		},
		3: {
			{Line: 3, StartRune: 0, EndRune: 3, Type: "keyword"},
		},
		5: {
			{Line: 5, StartRune: 0, EndRune: 5, Type: "keyword"},
			{Line: 5, StartRune: 6, EndRune: 11, Type: "variable"},
			{Line: 5, StartRune: 14, EndRune: 21, Type: "string"},
		},
	}}
	session.LSPDiagnostics[uri] = []lsp.Diagnostic{{
		URI:      uri,
		Range:    lsp.Range{Start: lsp.Position{Line: 3, Character: 2}, End: lsp.Position{Line: 3, Character: 8}},
		Severity: lsp.SeverityWarning,
		Message:  "unused local",
	}}
	session.LSPCartInfo[uri] = lsp.CartInfo{Tokens: 120, TokenLimit: 8192, Chars: 300, CharLimit: 65535, Compressed: 80, CompressedStatus: "ok"}
	buffer.Editor.SetCursor(editor.Pos{Line: 2, Rune: 4})
	session.Tree.Visible = false
	return game
}

func TestDocumentationScreenshots(t *testing.T) {
	game := screenshotGame(t)
	cases := []struct {
		name  string
		setup func()
	}{
		{name: "cart-view", setup: func() {}},
		{name: "sidebar", setup: func() {
			root := game.session.Workspace.Root
			lib := filepath.Join(root, "lib")
			if err := os.MkdirAll(lib, 0o755); err != nil {
				t.Fatal(err)
			}
			for path, content := range map[string]string{
				filepath.Join(lib, "util.lua"):    "function clamp(v, lo, hi)\n  return mid(lo, v, hi)\nend\n",
				filepath.Join(root, "README.md"):  "pico-editor\n",
				filepath.Join(root, "config.lua"): "scale=4\n",
			} {
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := game.session.Workspace.Refresh(); err != nil {
				t.Fatal(err)
			}
			game.session.Tree.Init(game.session)
			game.session.setTreeVisible(true)
			game.session.Tree.Focused = false
			game.session.resizeEditor()
		}},
		{name: "completion", setup: func() {
			game.session.Completion = &CompletionPopup{
				Items: []lsp.CompletionItem{
					{Label: "btn", Detail: "function btn(i)"},
					{Label: "btnp", Detail: "function btnp(i)"},
					{Label: "camera", Detail: "function camera()"},
				},
				Selected: 0,
			}
		}},
		{name: "picker", setup: func() {
			game.session.GlyphPicker = NewGlyphPicker([]lsp.Glyph{
				{Name: "left", Text: "⬅️", Byte: 0x8b, Category: "arrows", Description: "left arrow"},
				{Name: "right", Text: "➡️", Byte: 0x91, Category: "arrows", Description: "right arrow"},
				{Name: "heart", Text: "♥", Byte: 0x87, Category: "symbols", Description: "heart"},
				{Name: "cat", Text: "🐱", Byte: 0x82, Category: "symbols", Description: "cat"},
			})
		}},
		{name: "palette", setup: func() {
			game.openPalette()
		}},
		{name: "error-dialog", setup: func() {
			game.session.ShowErrorDetail("Save failed", "save", "/projects/game.p8", os.ErrPermission)
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			// Rebuild for each screen so overlays and dialogs do not leak.
			game = screenshotGame(t)
			test.setup()
			game.redraw()
			path := filepath.Join("..", "..", "docs", "images", test.name+".png")
			compareScreenshot(t, path, &game.renderer.Surface)
		})
	}
}

func compareScreenshot(t *testing.T, path string, surface *render.Surface) {
	t.Helper()
	img := scaleSurface(surface, 2)
	var encoded bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	if err := encoder.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, encoded.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read screenshot (run UPDATE_GOLDEN=1 go test ./internal/app): %v", err)
	}
	if !bytes.Equal(encoded.Bytes(), want) {
		t.Fatalf("%s is stale; run UPDATE_GOLDEN=1 go test ./internal/app", path)
	}
}
