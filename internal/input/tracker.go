package input

import (
	"slices"
	"time"
)

// Repeat timing is the locked V1 behavior: 400 ms before the first repeat,
// then one every 50 ms while the key is held.
const (
	RepeatDelay    = 400 * time.Millisecond
	RepeatInterval = 50 * time.Millisecond
)

// Snapshot is one frame of platform input state, kept separate from
// Ebitengine so repeat and focus behavior can be tested without a display.
type Snapshot struct {
	JustPressed []Key
	Pressed     map[Key]bool
	Mods        Mods
	Runes       []rune
	Focused     bool
	Mouse       MouseSnapshot
}

// MouseSnapshot is one frame of mouse state.
type MouseSnapshot struct {
	X       int
	Y       int
	Buttons MouseButtons
}

// Tracker turns snapshots into normalized events. It owns pressed-key and
// repeat state; it is not safe for concurrent use.
type Tracker struct {
	held      map[Key]time.Time
	focused   bool
	primed    bool
	lastMouse MouseSnapshot
	hasMouse  bool
}

// NewTracker returns an empty tracker.
func NewTracker() *Tracker {
	return &Tracker{held: make(map[Key]time.Time)}
}

// Poll converts one snapshot into events. now is the frame timestamp.
func (t *Tracker) Poll(now time.Time, snap Snapshot) []Event {
	var events []Event
	if t.held == nil {
		t.held = make(map[Key]time.Time)
	}

	if !t.primed {
		t.focused = snap.Focused
		t.primed = true
	} else if t.focused != snap.Focused {
		t.focused = snap.Focused
		events = append(events, Event{Kind: KindFocus, Focused: snap.Focused})
		if !snap.Focused {
			clear(t.held)
			t.lastMouse = snap.Mouse
			t.hasMouse = true
			return events
		}
	}
	if !snap.Focused {
		clear(t.held)
		t.lastMouse = snap.Mouse
		t.hasMouse = true
		return events
	}

	for _, key := range snap.JustPressed {
		if key == KeyUnknown {
			continue
		}
		events = append(events, Event{
			Kind: KindKeyDown,
			Key:  key,
			Mods: snap.Mods,
		})
		if isRepeatable(key) {
			if _, exists := t.held[key]; !exists {
				t.held[key] = now.Add(RepeatDelay)
			}
		}
	}

	held := make([]Key, 0, len(t.held))
	for key := range t.held {
		held = append(held, key)
	}
	slices.Sort(held)
	for _, key := range held {
		next := t.held[key]
		if !snap.Pressed[key] {
			delete(t.held, key)
			continue
		}
		if now.Before(next) {
			continue
		}
		events = append(events, Event{
			Kind:   KindKeyDown,
			Key:    key,
			Mods:   snap.Mods,
			Repeat: true,
		})
		t.held[key] = now.Add(RepeatInterval)
	}

	if len(snap.Runes) > 0 {
		runes := append([]rune(nil), snap.Runes...)
		events = append(events, Event{
			Kind:  KindText,
			Mods:  snap.Mods,
			Runes: runes,
		})
	}

	if t.hasMouse && (t.lastMouse.X != snap.Mouse.X ||
		t.lastMouse.Y != snap.Mouse.Y ||
		t.lastMouse.Buttons != snap.Mouse.Buttons) {
		events = append(events, Event{
			Kind:    KindMouse,
			Mods:    snap.Mods,
			X:       snap.Mouse.X,
			Y:       snap.Mouse.Y,
			Buttons: snap.Mouse.Buttons,
		})
	}
	t.lastMouse = snap.Mouse
	t.hasMouse = true

	return events
}
