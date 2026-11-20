// Package app owns the application state and the Ebitengine game loop.
package app

import (
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/asimmons91/pico-editor/internal/input"
	"github.com/asimmons91/pico-editor/internal/render"
)

// Config carries the dependencies the game needs. Later milestones extend it
// rather than rewiring main.
type Config struct {
	// Logger receives diagnostics. A nil logger discards them.
	Logger *slog.Logger
	// Clipboard defaults to the host clipboard when nil.
	Clipboard input.Clipboard
	// Now supplies wall-clock time for cursor blink. Nil uses time.Now.
	Now func() time.Time
	// DebugCharset forces the M1 charset screen and starts with the width
	// overlay. M1 shows the screen regardless; M2 keeps the flag.
	DebugCharset bool
}

type clipboardResult struct {
	text string
	err  error
}

// maxPastePreviewBytes bounds the text normalized and quoted for the debug
// log. The full clipboard text is never copied just to show a short preview.
const maxPastePreviewBytes = 4096

// Game is the root Ebitengine game. It owns no package-level state.
type Game struct {
	logger           *slog.Logger
	step             func() error
	fatalErr         error
	source           input.Source
	clipboard        input.Clipboard
	clipboardResults chan clipboardResult
	clipboardBusy    bool
	renderer         *Renderer
	charset          *Charset
	blink            *Blink
	scale            int
	now              func() time.Time
	firstFrame       bool
	screenWidth      int
	screenHeight     int
}

// New returns a game configured by cfg.
func New(cfg Config) *Game {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	clipboard := cfg.Clipboard
	if clipboard == nil {
		clipboard = input.NewSystemClipboard()
	}
	charset := NewCharset()
	if cfg.DebugCharset {
		charset.Overlay = true
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	game := &Game{
		logger:           logger,
		clipboard:        clipboard,
		clipboardResults: make(chan clipboardResult, 1),
		renderer:         NewRenderer(CanvasWidth, CanvasHeight, logger),
		charset:          charset,
		blink:            NewBlink(now()),
		scale:            DefaultScale,
		now:              now,
	}
	game.source = input.NewEbitenSource(game.mapCursor)
	game.step = game.updateFrame
	game.redraw()
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
	now := g.now()
	changed := g.blink.Update(now)
	if g.pollPaste() {
		changed = true
	}
	events := g.source.Poll()
	if len(events) > 0 {
		wasVisible := g.blink.Visible()
		g.blink.Reset(now)
		if !wasVisible {
			changed = true
		}
	}
	for _, event := range events {
		quit, eventChanged := g.handleEvent(event)
		changed = changed || eventChanged
		if quit {
			g.logger.Info("quit requested", "key", "ctrl+q")
			return ebiten.Termination
		}
	}
	if changed {
		g.redraw()
	}
	return nil
}

func (g *Game) handleEvent(event input.Event) (quit, changed bool) {
	switch event.Kind {
	case input.KindFocus:
		g.charset.AddLog(fmt.Sprintf("focus %t", event.Focused))
		return false, true
	case input.KindText:
		g.charset.AddLog(fmt.Sprintf("text %q", string(event.Runes)))
		return false, true
	case input.KindMouse:
		return false, false
	case input.KindResize:
		return false, false
	case input.KindKeyDown:
		return g.handleKey(event)
	default:
		return false, false
	}
}

func (g *Game) handleKey(event input.Event) (quit, changed bool) {
	key := event.Key
	if event.Repeat {
		if event.Mods == 0 {
			switch key {
			case input.KeyLeft:
				g.charset.Move(-1, 0)
				return false, true
			case input.KeyRight:
				g.charset.Move(1, 0)
				return false, true
			case input.KeyUp:
				g.charset.Move(0, -1)
				return false, true
			case input.KeyDown:
				g.charset.Move(0, 1)
				return false, true
			}
		}
		g.charset.AddLog(fmt.Sprintf("key %s repeat", input.FormatKey(key, event.Mods)))
		return false, true
	}

	if key == input.KeyQ && event.Mods&input.ModCtrl != 0 {
		return true, false
	}
	if key == input.KeyV && event.Mods&input.ModCtrl != 0 {
		g.startPaste()
		return false, true
	}
	if event.Mods == 0 {
		switch {
		case key >= input.Key1 && key <= input.Key4:
			g.setScale(int(key-input.Key1) + 1)
			g.charset.AddLog(fmt.Sprintf("scale %d", g.scale))
			return false, true
		case key == input.KeyW:
			g.charset.Overlay = !g.charset.Overlay
			g.charset.AddLog(fmt.Sprintf("overlay %t", g.charset.Overlay))
			return false, true
		case key == input.KeyP:
			g.charset.CycleBackground()
			g.charset.AddLog(fmt.Sprintf("background %d", g.charset.Background))
			return false, true
		case key == input.KeyLeft:
			g.charset.Move(-1, 0)
			return false, true
		case key == input.KeyRight:
			g.charset.Move(1, 0)
			return false, true
		case key == input.KeyUp:
			g.charset.Move(0, -1)
			return false, true
		case key == input.KeyDown:
			g.charset.Move(0, 1)
			return false, true
		case key == input.KeyEsc:
			g.charset.Log = nil
			return false, true
		}
	}

	g.charset.AddLog(fmt.Sprintf("key %s", input.FormatKey(key, event.Mods)))
	return false, true
}

func (g *Game) startPaste() {
	if g.clipboardBusy {
		g.charset.AddLog("paste pending")
		return
	}
	if g.clipboardResults == nil {
		g.clipboardResults = make(chan clipboardResult, 1)
	}
	g.clipboardBusy = true
	clipboard := g.clipboard
	results := g.clipboardResults
	go func() {
		result := clipboardResult{}
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					result.err = fmt.Errorf("clipboard panic: %v", recovered)
				}
			}()
			result.text, result.err = clipboard.ReadText()
		}()
		results <- result
	}()
}

func (g *Game) pollPaste() bool {
	if g.clipboardResults == nil {
		return false
	}
	select {
	case result := <-g.clipboardResults:
		g.clipboardBusy = false
		g.applyPaste(result)
		return true
	default:
		return false
	}
}

func (g *Game) applyPaste(result clipboardResult) {
	if result.err != nil {
		g.charset.AddLog("paste error: " + shortError(result.err))
		return
	}
	text := input.NormalizeNewlines(previewPrefix(result.text, maxPastePreviewBytes))
	g.charset.AddLog("paste " + truncate(text, 48))
}

func previewPrefix(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(text) <= limit {
		return text
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut]
}

func shortError(err error) string {
	if errors.Is(err, input.ErrUnavailable) {
		return "clipboard unavailable"
	}
	return truncate(err.Error(), 48)
}

func truncate(text string, limit int) string {
	if limit < 0 {
		limit = 0
	}
	count := 0
	for index := range text {
		if count == limit {
			return fmt.Sprintf("%q...", text[:index])
		}
		count++
	}
	return fmt.Sprintf("%q", text)
}

func (g *Game) setScale(scale int) {
	g.scale = scale
	ebiten.SetWindowSize(CanvasWidth*scale, CanvasHeight*scale)
}

func (g *Game) redraw() {
	g.charset.CursorVisible = g.blink.Visible()
	g.charset.Draw(&g.renderer.Surface)
	g.renderer.MarkDirty()
}

// Draw renders the 256x256 canvas. The renderer owns the only Ebitengine image
// that receives surface pixels.
func (g *Game) Draw(screen *ebiten.Image) {
	g.doFrame("draw", func() {
		g.renderer.Upload()
		g.renderer.Present(screen)
	})
}

// DrawFinalScreen scales the canvas by an integer factor, centers it, and
// letterboxes the remainder. Nearest-neighbor sampling keeps every canvas
// pixel sharp at any window size.
func (g *Game) DrawFinalScreen(screen ebiten.FinalScreen, offscreen *ebiten.Image, _ ebiten.GeoM) {
	g.doFrame("final screen", func() {
		g.drawFinalScreen(screen, offscreen)
		g.markFirstFrame()
	})
}

func (g *Game) markFirstFrame() {
	if g.firstFrame {
		return
	}
	g.firstFrame = true
	g.logger.Info("first-frame")
}

func (g *Game) drawFinalScreen(screen ebiten.FinalScreen, offscreen *ebiten.Image) {
	screen.Fill(render.Palette[render.DefaultBackground])

	bounds := screen.Bounds()
	g.screenWidth = bounds.Dx()
	g.screenHeight = bounds.Dy()
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

// doFrame runs a frame callback, recovering any panic so it becomes a fatal
// error for the next Update. Once a frame has failed, later frames are no-ops.
func (g *Game) doFrame(where string, action func()) {
	if g.fatalErr != nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			g.fatalErr = g.reportPanic(where, recovered)
		}
	}()
	action()
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
