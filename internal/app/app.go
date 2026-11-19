// Package app owns the application state and the Ebitengine game loop.
package app

import (
	"image/color"
	"log/slog"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// picoBlack is PICO-8 palette index 0.
var picoBlack = color.RGBA{A: 0xff}

// Config carries the dependencies the game needs. Later milestones extend it
// rather than rewiring main.
type Config struct {
	// Logger receives diagnostics. A nil logger discards them.
	Logger *slog.Logger
}

// Game is the root Ebitengine game. It owns no package-level state.
type Game struct {
	logger *slog.Logger
}

// New returns a game configured by cfg.
func New(cfg Config) *Game {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Game{logger: logger}
}

// Update advances one tick. M0 only handles quit; later milestones run the
// editor, Vim, and LSP message pumps here.
func (g *Game) Update() error {
	if inpututil.IsKeyJustPressed(ebiten.KeyQ) && ctrlPressed() {
		g.logger.Info("quit requested", "key", "ctrl+q")
		return ebiten.Termination
	}
	return nil
}

// Draw renders the 256x256 canvas. M0 paints it PICO-8 black.
func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(picoBlack)
}

// DrawFinalScreen scales the canvas by an integer factor, centers it, and
// letterboxes the remainder. Nearest-neighbor sampling keeps every canvas
// pixel sharp at any window size.
func (g *Game) DrawFinalScreen(screen ebiten.FinalScreen, offscreen *ebiten.Image, _ ebiten.GeoM) {
	screen.Fill(picoBlack)

	bounds := screen.Bounds()
	scale := ScaleFor(bounds.Dx(), bounds.Dy())
	destination := CanvasRect(bounds.Dx(), bounds.Dy(), scale)

	options := &ebiten.DrawImageOptions{Filter: ebiten.FilterNearest}
	options.GeoM.Scale(float64(scale), float64(scale))
	options.GeoM.Translate(
		float64(bounds.Min.X+destination.Min.X),
		float64(bounds.Min.Y+destination.Min.Y),
	)
	screen.DrawImage(offscreen, options)
}

// Layout fixes the logical canvas at 256x256 device-independent pixels.
func (g *Game) Layout(_, _ int) (int, int) {
	return CanvasWidth, CanvasHeight
}

func ctrlPressed() bool {
	return ebiten.IsKeyPressed(ebiten.KeyControl) ||
		ebiten.IsKeyPressed(ebiten.KeyControlLeft) ||
		ebiten.IsKeyPressed(ebiten.KeyControlRight)
}
