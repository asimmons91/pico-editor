package input

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

func pressed(keys ...Key) map[Key]bool {
	m := make(map[Key]bool, len(keys))
	for _, key := range keys {
		m[key] = true
	}
	return m
}

func keyEvents(events []Event) []Event {
	out := make([]Event, 0, len(events))
	for _, event := range events {
		if event.Kind == KindKeyDown {
			out = append(out, event)
		}
	}
	return out
}

func TestTrackerRepeatTiming(t *testing.T) {
	tracker := NewTracker()
	first := tracker.Poll(t0, Snapshot{
		JustPressed: []Key{KeyA},
		Pressed:     pressed(KeyA),
		Focused:     true,
	})
	events := keyEvents(first)
	if len(events) != 1 || events[0].Repeat {
		t.Fatalf("first poll = %+v, want one non-repeat key event", events)
	}

	for _, at := range []time.Duration{399 * time.Millisecond, 0, 1 * time.Millisecond} {
		got := keyEvents(tracker.Poll(t0.Add(at), Snapshot{Pressed: pressed(KeyA), Focused: true}))
		if len(got) != 0 {
			t.Fatalf("poll at %v = %+v, want no repeat", at, got)
		}
	}

	for _, wantAt := range []time.Duration{400 * time.Millisecond, 450 * time.Millisecond, 500 * time.Millisecond} {
		got := keyEvents(tracker.Poll(t0.Add(wantAt), Snapshot{Pressed: pressed(KeyA), Focused: true}))
		if len(got) != 1 || !got[0].Repeat {
			t.Fatalf("poll at %v = %+v, want one repeat", wantAt, got)
		}
	}

	tracker.Poll(t0.Add(510*time.Millisecond), Snapshot{Pressed: pressed(), Focused: true})
	got := keyEvents(tracker.Poll(t0.Add(1000*time.Millisecond), Snapshot{Pressed: pressed(), Focused: true}))
	if len(got) != 0 {
		t.Fatalf("released key repeated: %+v", got)
	}
}

func TestTrackerDoesNotRepeatModifiersOrEscape(t *testing.T) {
	for _, key := range []Key{KeyShift, KeyControl, KeyAlt, KeyMeta, KeyEsc} {
		tracker := NewTracker()
		tracker.Poll(t0, Snapshot{JustPressed: []Key{key}, Pressed: pressed(key), Focused: true})
		if got := keyEvents(tracker.Poll(t0.Add(2*time.Second), Snapshot{Pressed: pressed(key), Focused: true})); len(got) != 0 {
			t.Errorf("key %v repeated: %+v", key, got)
		}
	}
}

func TestTrackerEmitsTextAndNotKeyRepeatText(t *testing.T) {
	tracker := NewTracker()
	events := tracker.Poll(t0, Snapshot{
		JustPressed: []Key{KeyA},
		Pressed:     pressed(KeyA),
		Runes:       []rune("a"),
		Focused:     true,
	})
	var textEvents int
	for _, event := range events {
		if event.Kind == KindText {
			textEvents++
			if string(event.Runes) != "a" {
				t.Errorf("text event = %q, want a", string(event.Runes))
			}
		}
	}
	if textEvents != 1 {
		t.Fatalf("text events = %d, want 1", textEvents)
	}

	for _, event := range tracker.Poll(t0.Add(2*time.Second), Snapshot{Pressed: pressed(KeyA), Focused: true}) {
		if event.Kind == KindText {
			t.Fatalf("key repeat synthesized a text event: %+v", event)
		}
	}
}

func TestTrackerFocusLossClearsHeldKeys(t *testing.T) {
	tracker := NewTracker()
	tracker.Poll(t0, Snapshot{JustPressed: []Key{KeyA}, Pressed: pressed(KeyA), Focused: true})

	events := tracker.Poll(t0.Add(time.Second), Snapshot{Focused: false})
	if len(events) != 1 || events[0].Kind != KindFocus || events[0].Focused {
		t.Fatalf("focus-loss poll = %+v, want one unfocused event", events)
	}

	tracker.Poll(t0.Add(2*time.Second), Snapshot{Focused: true})
	events = tracker.Poll(t0.Add(3*time.Second), Snapshot{Pressed: pressed(KeyA), Focused: true})
	if got := keyEvents(events); len(got) != 0 {
		t.Fatalf("held key repeated after focus loss: %+v", got)
	}

	events = tracker.Poll(t0.Add(4*time.Second), Snapshot{Focused: false})
	if len(events) != 1 || events[0].Kind != KindFocus || events[0].Focused {
		t.Fatalf("second focus loss = %+v", events)
	}
}

func TestTrackerMouseTransitions(t *testing.T) {
	tracker := NewTracker()
	events := tracker.Poll(t0, Snapshot{Focused: true, Mouse: MouseSnapshot{X: 10, Y: 20}})
	if len(events) != 0 {
		t.Fatalf("first poll = %+v, want no mouse event", events)
	}

	events = tracker.Poll(t0.Add(time.Second), Snapshot{Focused: true, Mouse: MouseSnapshot{X: 11, Y: 20}})
	if len(events) != 1 || events[0].Kind != KindMouse || events[0].X != 11 {
		t.Fatalf("move poll = %+v, want one mouse event at x=11", events)
	}

	events = tracker.Poll(t0.Add(2*time.Second), Snapshot{Focused: true, Mouse: MouseSnapshot{X: 11, Y: 20, Buttons: MouseLeft}})
	if len(events) != 1 || events[0].Kind != KindMouse || events[0].Buttons != MouseLeft {
		t.Fatalf("button poll = %+v, want one left-button event", events)
	}

	events = tracker.Poll(t0.Add(3*time.Second), Snapshot{Focused: true, Mouse: MouseSnapshot{X: 11, Y: 20, Buttons: MouseLeft}})
	if len(events) != 0 {
		t.Fatalf("unchanged mouse poll = %+v, want no event", events)
	}
}

func TestTrackerInitialFocusDoesNotEmit(t *testing.T) {
	for _, focused := range []bool{true, false} {
		tracker := NewTracker()
		events := tracker.Poll(t0, Snapshot{Focused: focused})
		if len(events) != 0 {
			t.Errorf("initial focused=%t emitted %+v", focused, events)
		}
	}
}
