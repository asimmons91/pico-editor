package app

import (
	"testing"
	"time"
)

func TestNotificationDedupeAndTimeout(t *testing.T) {
	now := time.Unix(1000, 0)
	center := newNotificationCenter(func() time.Time { return now })
	center.configure(4*time.Second, 10)
	center.Push("same", NotifyInfo, true)
	now = now.Add(time.Second)
	center.Push("same", NotifyInfo, true)
	toasts := center.Toasts()
	if len(toasts) != 1 || toasts[0].Count != 2 {
		t.Fatalf("toasts = %+v, want one deduped toast with count 2", toasts)
	}
	now = now.Add(5 * time.Second)
	if !center.Tick(now) {
		t.Fatal("Tick did not expire the info toast")
	}
	if len(center.Toasts()) != 0 {
		t.Fatalf("toasts = %+v, want none after timeout", center.Toasts())
	}
}

func TestNotificationErrorsPersistUntilDismissed(t *testing.T) {
	now := time.Unix(1000, 0)
	center := newNotificationCenter(func() time.Time { return now })
	center.configure(time.Second, 10)
	center.Push("boom", NotifyError, true)
	now = now.Add(time.Hour)
	if center.Tick(now) {
		t.Fatal("error toast expired")
	}
	if !center.HasError() {
		t.Fatal("HasError = false with an error toast")
	}
	if !center.Dismiss() {
		t.Fatal("Dismiss = false with an error toast")
	}
	if len(center.Toasts()) != 0 {
		t.Fatalf("toasts = %+v, want empty", center.Toasts())
	}
}

func TestNotificationMaxStackAndHistoryCap(t *testing.T) {
	now := time.Unix(1000, 0)
	center := newNotificationCenter(func() time.Time { return now })
	center.configure(time.Minute, 3)
	for _, message := range []string{"one", "two", "three", "four"} {
		center.Push(message, NotifyWarning, true)
		now = now.Add(3 * time.Second)
	}
	if len(center.Toasts()) != notificationMaxToasts {
		t.Fatalf("toasts = %+v, want %d", center.Toasts(), notificationMaxToasts)
	}
	if len(center.HistoryEntries()) != 3 {
		t.Fatalf("history = %+v, want cap 3", center.HistoryEntries())
	}
}

func TestNotificationRoutineDoesNotToast(t *testing.T) {
	center := newNotificationCenter(nil)
	center.Push("lsp noise", NotifyInfo, false)
	if len(center.Toasts()) != 0 {
		t.Fatalf("toasts = %+v, want none", center.Toasts())
	}
	if len(center.HistoryEntries()) != 1 {
		t.Fatalf("history = %+v, want one entry", center.HistoryEntries())
	}
}

func TestNotificationZeroTimeoutKeepsErrors(t *testing.T) {
	now := time.Unix(1000, 0)
	center := newNotificationCenter(func() time.Time { return now })
	center.configure(0, 10)
	center.Push("info", NotifyInfo, true)
	center.Push("error", NotifyError, true)
	if !center.Tick(now) {
		t.Fatal("Tick did not remove the info toast")
	}
	toasts := center.Toasts()
	if len(toasts) != 1 || toasts[0].Message != "error" {
		t.Fatalf("toasts = %+v, want only the persistent error", toasts)
	}
}

func TestShowMessagesBuildsHistoryOverlay(t *testing.T) {
	game := New(Config{})
	game.notifications.Push("one", NotifyInfo, true)
	game.notifications.Push("two", NotifyError, true)
	if !game.showMessages() {
		t.Fatal("showMessages = false")
	}
	if game.session.Help == nil || game.session.Help.Title != "Messages" {
		t.Fatalf("help = %+v, want the messages overlay", game.session.Help)
	}
	if len(game.session.Help.Lines) != 2 {
		t.Fatalf("lines = %v, want two", game.session.Help.Lines)
	}
}
