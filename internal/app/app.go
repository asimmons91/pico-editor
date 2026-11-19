// Package app owns the application state and the Ebitengine game loop.
package app

import (
	"fmt"
	"image/color"
	"log/slog"
	"runtime/debug"

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
	logger   *slog.Logger
	step     func() error
	fatalErr error
}

// New returns a game configured by cfg.
func New(cfg Config) *Game {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	game := &Game{logger: logger}
	game.step = game.updateFrame
	return game
}

// Update advances one tick. Ebitengine calls Update on its game goroutine, so
// frame panics are recovered here and returned as an error to stop RunGame.
func (g *Game) Update() (err error) {
	if g.fatalErr != nil {
		return g.fatalErr
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			g.fatalErr = g.reportPanic("update", recovered)
			err = g.fatalErr
		}
	}()
	return g.step()
}

func (g *Game) updateFrame() error {
	if inpututil.IsKeyJustPressed(ebiten.KeyQ) && ctrlPressed() {
		g.logger.Info("quit requested", "key", "ctrl+q")
		return ebiten.Termination
	}
	return nil
}

// Draw renders the 256x256 canvas. M0 paints it PICO-8 black.
func (g *Game) Draw(screen *ebiten.Image) {
	if g.fatalErr != nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			g.fatalErr = g.reportPanic("draw", recovered)
		}
	}()
	screen.Fill(picoBlack)
}

// DrawFinalScreen scales the canvas by an integer factor, centers it, and
// letterboxes the remainder. Nearest-neighbor sampling keeps every canvas
// pixel sharp at any window size.
func (g *Game) DrawFinalScreen(screen ebiten.FinalScreen, offscreen *ebiten.Image, _ ebiten.GeoM) {
	if g.fatalErr != nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			g.fatalErr = g.reportPanic("final screen", recovered)
		}
	}()

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
func (g *Game) Layout(_, _ int) (width, height int) {
	defer func() {
		if recovered := recover(); recovered != nil {
			g.fatalErr = g.reportPanic("layout", recovered)
			width, height = CanvasWidth, CanvasHeight
		}
	}()
	return CanvasWidth, CanvasHeight
}

// reportPanic logs a frame panic with its stack and returns the fatal error
// that Update hands back to Ebitengine, which stops RunGame and exits non-zero.
func (g *Game) reportPanic(where string, recovered any) error {
	g.logger.Error("panic",
		"where", where,
		"panic", recovered,
		"stack", string(debug.Stack()),
	)
	return fmt.Errorf("panic in %s: %v", where, recovered)
}

func ctrlPressed() bool {
	return ebiten.IsKeyPressed(ebiten.KeyControl) ||
		ebiten.IsKeyPressed(ebiten.KeyControlLeft) ||
		ebiten.IsKeyPressed(ebiten.KeyControlRight)
}
