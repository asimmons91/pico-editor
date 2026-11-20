package input

import "math"

// CanvasPosition maps Ebitengine's cursor position into logical canvas pixels.
//
// Ebitengine reports cursor coordinates through its default letterbox
// transform, which uses a fractional scale. pico-editor draws the canvas
// through its own integer-scaled, centered rectangle. Inverting the default
// transform and applying the editor's rectangle keeps mouse events in canvas
// coordinates at any window size or device scale.
func CanvasPosition(cursorX, cursorY, windowWidth, windowHeight int, deviceScale float64, canvasWidth, canvasHeight int) (int, int) {
	if windowWidth <= 0 || windowHeight <= 0 || canvasWidth <= 0 || canvasHeight <= 0 {
		return cursorX, cursorY
	}
	if deviceScale <= 0 {
		deviceScale = 1
	}

	physicalWidth := math.Round(float64(windowWidth) * deviceScale)
	physicalHeight := math.Round(float64(windowHeight) * deviceScale)
	defaultScale := math.Min(physicalWidth/float64(canvasWidth), physicalHeight/float64(canvasHeight))
	if defaultScale <= 0 {
		return cursorX, cursorY
	}
	defaultOffsetX := (physicalWidth - float64(canvasWidth)*defaultScale) / 2
	defaultOffsetY := (physicalHeight - float64(canvasHeight)*defaultScale) / 2
	physicalX := float64(cursorX)*defaultScale + defaultOffsetX
	physicalY := float64(cursorY)*defaultScale + defaultOffsetY

	scale := max(1, min(int(physicalWidth)/canvasWidth, int(physicalHeight)/canvasHeight))
	offsetX := (physicalWidth - float64(canvasWidth*scale)) / 2
	offsetY := (physicalHeight - float64(canvasHeight*scale)) / 2
	canvasX := int(math.Floor((physicalX - offsetX) / float64(scale)))
	canvasY := int(math.Floor((physicalY - offsetY) / float64(scale)))
	return canvasX, canvasY
}
