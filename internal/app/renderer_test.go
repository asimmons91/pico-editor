package app

import (
	"image/color"
	"log/slog"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/asimmons91/pico-editor/internal/render"
)

func TestRendererUpload(t *testing.T) {
	renderer := NewRenderer(2, 2, slog.New(slog.DiscardHandler))
	renderer.Surface.Set(0, 0, 0)
	renderer.Surface.Set(1, 0, 1)
	renderer.Surface.Set(0, 1, 2)
	renderer.Surface.Set(1, 1, 3)
	renderer.MarkDirty()
	renderer.Upload()

	for i, index := range []uint8{0, 1, 2, 3} {
		want := render.Palette[index]
		got := color.RGBA{
			R: renderer.pixels[i*4+0],
			G: renderer.pixels[i*4+1],
			B: renderer.pixels[i*4+2],
			A: renderer.pixels[i*4+3],
		}
		if got != want {
			t.Errorf("pixel %d = %#v, want %#v", i, got, want)
		}
	}
}

func TestRendererUploadsOnlyWhenDirty(t *testing.T) {
	renderer := NewRenderer(2, 2, slog.New(slog.DiscardHandler))
	renderer.Surface.Set(0, 0, 7)
	renderer.MarkDirty()
	renderer.Upload()

	renderer.Surface.Set(0, 0, 3)
	renderer.Upload()
	if got := renderer.pixels[0]; got != render.Palette[7].R {
		t.Errorf("pixel changed while not dirty: %d", got)
	}

	renderer.MarkDirty()
	renderer.Upload()
	if got := renderer.pixels[0]; got != render.Palette[3].R {
		t.Errorf("pixel = %d after dirty upload, want %d", got, render.Palette[3].R)
	}
}

func TestRendererPresent(t *testing.T) {
	renderer := NewRenderer(2, 2, slog.New(slog.DiscardHandler))
	renderer.Surface.Set(0, 0, 4)
	renderer.MarkDirty()
	renderer.Upload()

	// Ebitengine forbids ReadPixels before the game starts, so this asserts the
	// present path and its 1:1 DrawImage call without pixel readback.
	target := ebiten.NewImage(4, 4)
	renderer.Present(target)
}

func TestGameDraw(t *testing.T) {
	game := newTestGame(t)
	screen := ebiten.NewImage(CanvasWidth, CanvasHeight)
	game.Draw(screen)
	if game.fatalErr != nil {
		t.Fatalf("Draw recorded a fatal error: %v", game.fatalErr)
	}
}
