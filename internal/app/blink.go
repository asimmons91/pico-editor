package app

import "time"

// BlinkInterval is the visible and hidden duration of the editor cursor.
const BlinkInterval = 500 * time.Millisecond

// Blink owns cursor visibility. It belongs to the app, not the renderer.
type Blink struct {
	visible bool
	next    time.Time
}

// NewBlink returns a visible cursor that will hide after BlinkInterval.
func NewBlink(now time.Time) *Blink {
	return &Blink{visible: true, next: now.Add(BlinkInterval)}
}

// Update advances the blink clock and reports whether visibility changed.
func (b *Blink) Update(now time.Time) bool {
	changed := false
	for !now.Before(b.next) {
		b.visible = !b.visible
		b.next = b.next.Add(BlinkInterval)
		changed = true
	}
	return changed
}

// Reset makes the cursor visible and restarts the timer after any input.
func (b *Blink) Reset(now time.Time) {
	b.visible = true
	b.next = now.Add(BlinkInterval)
}

// Visible reports the current cursor visibility.
func (b *Blink) Visible() bool {
	return b.visible
}
