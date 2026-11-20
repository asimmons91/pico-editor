package app

import "time"

// BlinkInterval is the default visible and hidden duration of the editor
// cursor. Settings may override it at runtime.
const BlinkInterval = 500 * time.Millisecond

// Blink owns cursor visibility. It belongs to the app, not the renderer.
type Blink struct {
	visible  bool
	next     time.Time
	interval time.Duration
}

// NewBlink returns a visible cursor that will hide after BlinkInterval.
func NewBlink(now time.Time) *Blink {
	return NewBlinkInterval(now, BlinkInterval)
}

// NewBlinkInterval returns a visible cursor for a configured interval. A zero
// or negative interval keeps the cursor permanently visible.
func NewBlinkInterval(now time.Time, interval time.Duration) *Blink {
	blink := &Blink{visible: true, interval: interval}
	blink.Reset(now)
	return blink
}

// SetInterval changes the blink interval without hiding the cursor. Zero
// disables blinking.
func (b *Blink) SetInterval(now time.Time, interval time.Duration) {
	b.interval = interval
	b.Reset(now)
}

// Update advances the blink clock and reports whether visibility changed.
func (b *Blink) Update(now time.Time) bool {
	if b.interval <= 0 {
		return false
	}
	changed := false
	for !now.Before(b.next) {
		b.visible = !b.visible
		b.next = b.next.Add(b.interval)
		changed = true
	}
	return changed
}

// Reset makes the cursor visible and restarts the timer after any input.
func (b *Blink) Reset(now time.Time) {
	b.visible = true
	if b.interval <= 0 {
		b.next = time.Time{}
		return
	}
	b.next = now.Add(b.interval)
}

// Visible reports the current cursor visibility.
func (b *Blink) Visible() bool {
	return b.visible
}
