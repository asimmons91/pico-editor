package app

import (
	"image"
	"math"

	"github.com/asimmons91/pico-editor/internal/font"
)

// Canvas geometry is a locked V1 decision: a 256x256 logical canvas of 64x42
// cells with a 4x6 glyph cell. Every later milestone builds on these values.
const (
	CanvasWidth  = 256
	CanvasHeight = 256

	// The layout grid is 64 columns x 42 rows of 4x6 cells. Forty-two rows
	// cover 252 pixels, leaving the bottom four pixels free.
	CellWidth   = font.NarrowWidth
	CellHeight  = font.CellHeight
	GridColumns = 64
	GridRows    = 42

	// DefaultScale is the initial window scale in device-independent pixels.
	DefaultScale = 4

	// MinAutoScale and MaxAutoScale bound Scale == 0 (automatic) window sizing.
	MinAutoScale = 2
	MaxAutoScale = 8
)

const (
	autoScaleNumerator   = 4
	autoScaleDenominator = 5
)

// ScaleFor returns the largest integer scale that fits a window of the given
// size. It never returns less than 1 so the canvas is always legible.
func ScaleFor(width, height int) int {
	return max(1, min(width/CanvasWidth, height/CanvasHeight))
}

// AutoScale returns the largest integer scale that fits 80% of a monitor,
// clamped to MinAutoScale..MaxAutoScale. Monitor dimensions are in
// device-independent pixels, so fractional device scale factors are already
// accounted for by the caller.
func AutoScale(monitorWidth, monitorHeight int) int {
	fitWidth := monitorWidth * autoScaleNumerator / autoScaleDenominator / CanvasWidth
	fitHeight := monitorHeight * autoScaleNumerator / autoScaleDenominator / CanvasHeight
	scale := min(fitWidth, fitHeight)
	return max(MinAutoScale, min(scale, MaxAutoScale))
}

// ResolveScale converts a requested window scale into a concrete one. A
// requested scale of 0 means automatic; a missing monitor size falls back to
// DefaultScale.
func ResolveScale(requested, monitorWidth, monitorHeight int) int {
	if requested > 0 {
		return requested
	}
	if monitorWidth <= 0 || monitorHeight <= 0 {
		return DefaultScale
	}
	return AutoScale(monitorWidth, monitorHeight)
}

// CanvasRect returns the destination rectangle for the canvas inside a window.
// The canvas is centered and never scaled fractionally, so the letterbox bars
// can be uneven by at most one pixel.
func CanvasRect(windowWidth, windowHeight, scale int) image.Rectangle {
	width := CanvasWidth * scale
	height := CanvasHeight * scale
	x := (windowWidth - width) / 2
	y := (windowHeight - height) / 2
	return image.Rect(x, y, x+width, y+height)
}

// mapCursor converts Ebitengine's default-transform cursor coordinates into
// logical canvas pixels. The final-screen bounds are recorded by
// drawFinalScreen; before the first frame there is no canvas rectangle to map
// against, so the raw coordinates are used.
func (g *Game) mapCursor(cursorX, cursorY float64) (int, int) {
	if g.screenWidth <= 0 || g.screenHeight <= 0 {
		return int(cursorX), int(cursorY)
	}
	return canvasPosition(cursorX, cursorY, g.screenWidth, g.screenHeight)
}

// canvasPosition inverts Ebitengine's default letterbox transform and applies
// the editor's integer CanvasRect. Both steps use physical final-screen pixels.
func canvasPosition(cursorX, cursorY float64, windowWidth, windowHeight int) (int, int) {
	if windowWidth <= 0 || windowHeight <= 0 {
		return int(cursorX), int(cursorY)
	}
	defaultScale := math.Min(
		float64(windowWidth)/float64(CanvasWidth),
		float64(windowHeight)/float64(CanvasHeight),
	)
	if defaultScale <= 0 {
		return int(cursorX), int(cursorY)
	}
	defaultOffsetX := (float64(windowWidth) - float64(CanvasWidth)*defaultScale) / 2
	defaultOffsetY := (float64(windowHeight) - float64(CanvasHeight)*defaultScale) / 2
	physicalX := cursorX*defaultScale + defaultOffsetX
	physicalY := cursorY*defaultScale + defaultOffsetY

	scale := ScaleFor(windowWidth, windowHeight)
	rect := CanvasRect(windowWidth, windowHeight, scale)
	const epsilon = 1e-6
	canvasX := int(math.Floor((physicalX-float64(rect.Min.X))/float64(scale) + epsilon))
	canvasY := int(math.Floor((physicalY-float64(rect.Min.Y))/float64(scale) + epsilon))
	return canvasX, canvasY
}
