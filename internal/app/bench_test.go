package app

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/asimmons91/pico-editor/internal/editor"
	"github.com/asimmons91/pico-editor/internal/lsp"
	"github.com/asimmons91/pico-editor/internal/render"
)

func benchmarkGame(b *testing.B) *Game {
	b.Helper()
	root := b.TempDir()
	path := filepath.Join(root, "bench.lua")
	if err := os.WriteFile(path, []byte("local x = 1\nprint(x)\n"), 0o644); err != nil {
		b.Fatal(err)
	}
	game := New(Config{Logger: slog.New(slog.DiscardHandler), File: path})
	if err := game.StartupError(); err != nil {
		b.Fatal(err)
	}
	game.source = &fakeSource{}
	return game
}

func BenchmarkPaletteFilter(b *testing.B) {
	game := benchmarkGame(b)
	game.openPalette()
	game.palette.Query = "file"
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		game.refilterPalette()
	}
}

func BenchmarkSessionDraw(b *testing.B) {
	game := benchmarkGame(b)
	surface := render.NewSurface(CanvasWidth, CanvasHeight)
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		game.session.Draw(surface, true)
	}
}

func BenchmarkTokenApply(b *testing.B) {
	buffer := editor.New("")
	buffer.Lines = make([]string, 200)
	for index := range buffer.Lines {
		buffer.Lines[index] = strings.Repeat("x", 40)
	}
	data := make([]uint32, 0, 5*200)
	for line := 0; line < 200; line++ {
		data = append(data, 0, 0, 40, 0, 0)
	}
	types := []string{"keyword"}
	mods := []string{"declaration"}
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := lsp.DecodeTokens(data, types, mods, buffer, 0, "file:///bench.lua", 1); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBufferInsert(b *testing.B) {
	buffer := editor.New("")
	buffer.Lines = []string{""}
	ed := editor.NewEditor(buffer)
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := ed.Insert("x"); err != nil {
			b.Fatal(err)
		}
	}
}

func make64KBCart(tb testing.TB) string {
	tb.Helper()
	root := tb.TempDir()
	path := filepath.Join(root, "large.p8")
	line := "a=1 -- padding padding padding\n"
	body := strings.Repeat(line, 65536/len(line)+1)
	body = body[:65536]
	data := "pico-8 cartridge // pico-8.com\nversion 42\n__lua__\n" + body
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		tb.Fatal(err)
	}
	return path
}

func Test64KBCartOpenAndEdit(t *testing.T) {
	path := make64KBCart(t)
	openBudget := 200 * time.Millisecond
	editBudget := 8 * time.Millisecond
	if os.Getenv("CI") != "" {
		// Shared runners are noisy; keep the budget check for the reference
		// machine and use a smoke threshold in CI.
		openBudget = 2 * time.Second
		editBudget = 2 * time.Second
	}
	start := time.Now()
	game := New(Config{Logger: slog.New(slog.DiscardHandler), File: path, Now: time.Now})
	open := time.Since(start)
	if err := game.StartupError(); err != nil {
		t.Fatal(err)
	}
	if open > openBudget {
		t.Fatalf("64KB cart open took %s, want < %s", open, openBudget)
	}
	game.source = &fakeSource{}
	start = time.Now()
	if err := sendKeys(t, game, "ix<esc>"); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > editBudget {
		t.Fatalf("64KB cart edit took %s, want < %s", elapsed, editBudget)
	}
	if got := len(game.session.CurrentEditor().Buffer.Text()); got < 65536 {
		t.Fatalf("edited cart text = %d bytes, want >= 64KB", got)
	}
	t.Logf("64KB cart: open %s (budget %s), edit %s (budget %s)", open, openBudget, time.Since(start), editBudget)
}
