package app

import (
	"testing"
	"time"
)

func TestBlink(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	blink := NewBlink(t0)
	if !blink.Visible() {
		t.Fatal("new blink is hidden")
	}
	if blink.Update(t0.Add(499 * time.Millisecond)) {
		t.Fatal("blink changed before 500ms")
	}
	if !blink.Update(t0.Add(500 * time.Millisecond)) {
		t.Fatal("blink did not hide at 500ms")
	}
	if blink.Visible() {
		t.Fatal("blink is visible after hiding")
	}
	if blink.Update(t0.Add(999 * time.Millisecond)) {
		t.Fatal("blink changed before 1000ms")
	}
	if !blink.Update(t0.Add(1000 * time.Millisecond)) {
		t.Fatal("blink did not show at 1000ms")
	}
	if !blink.Visible() {
		t.Fatal("blink is hidden after showing")
	}

	blink.Reset(t0.Add(1200 * time.Millisecond))
	if !blink.Visible() {
		t.Fatal("Reset did not show the cursor")
	}
	if blink.Update(t0.Add(1699 * time.Millisecond)) {
		t.Fatal("blink changed before the reset interval elapsed")
	}
	if !blink.Update(t0.Add(1700 * time.Millisecond)) {
		t.Fatal("blink did not hide after the reset interval")
	}
}

func TestBlinkCatchesUpAfterDelay(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	blink := NewBlink(t0)
	if !blink.Update(t0.Add(1600 * time.Millisecond)) {
		t.Fatal("Update did not report a visibility change")
	}
	if blink.Visible() {
		t.Fatal("after three intervals visible = true, want false")
	}
}

func TestCharsetCursorFollowsBlink(t *testing.T) {
	t.Parallel()

	game := newTestGame(t)
	if !game.charset.CursorVisible {
		t.Fatal("new game did not show the cursor")
	}
	game.charset.CursorVisible = false
	game.redraw()
	if !game.charset.CursorVisible {
		t.Fatal("redraw did not restore cursor visibility from the blink state")
	}
}
