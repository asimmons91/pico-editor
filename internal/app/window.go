package app

import "image"

// Canvas geometry is a locked V1 decision: a 256x256 logical canvas of 64x42
// cells with a 4x6 glyph cell. Every later milestone builds on these values.
const (
	CanvasWidth  = 256
	CanvasHeight = 256

	// The layout grid is 64 columns x 42 rows of 4x6 cells. Forty-two rows
	// cover 252 pixels, leaving the bottom four pixels free.
	CellWidth   = 4
	CellHeight  = 6
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
