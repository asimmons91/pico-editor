package app

import (
	"fmt"
	"time"

	"github.com/asimmons91/pico-editor/internal/font"
	"github.com/asimmons91/pico-editor/internal/render"
)

// NotificationSeverity controls toast lifetime and color.
type NotificationSeverity uint8

const (
	NotifyInfo NotificationSeverity = iota
	NotifySuccess
	NotifyWarning
	NotifyError
)

// Notification is one message in the toast stack or the history.
type Notification struct {
	Message  string
	Severity NotificationSeverity
	Time     time.Time
	Count    int
}

// NotificationCenter owns the toast stack and bounded history. It is only
// touched on the Update goroutine.
type NotificationCenter struct {
	toasts     []Notification
	history    []Notification
	timeout    time.Duration
	historyCap int
	now        func() time.Time
}

const (
	notificationToastCells = 40
	notificationMaxToasts  = 3
	notificationDedupe     = 2 * time.Second
)

func newNotificationCenter(now func() time.Time) *NotificationCenter {
	if now == nil {
		now = time.Now
	}
	return &NotificationCenter{
		timeout:    4 * time.Second,
		historyCap: 100,
		now:        now,
	}
}

func (n *NotificationCenter) configure(timeout time.Duration, history int) {
	n.timeout = timeout
	n.historyCap = max(0, history)
	n.trimHistory()
}

// Push records a notification. Routine logs pass toast=false to appear only in
// the history. Identical toasts within two seconds increment Count instead of
// stacking another panel.
func (n *NotificationCenter) Push(message string, severity NotificationSeverity, toast bool) {
	if n == nil || message == "" {
		return
	}
	now := n.now()
	n.history = append(n.history, Notification{Message: message, Severity: severity, Time: now, Count: 1})
	n.trimHistory()
	if !toast {
		return
	}
	for index := range n.toasts {
		item := &n.toasts[index]
		if item.Message == message && item.Severity == severity && now.Sub(item.Time) < notificationDedupe {
			item.Count++
			item.Time = now
			return
		}
	}
	n.toasts = append(n.toasts, Notification{Message: message, Severity: severity, Time: now, Count: 1})
	if len(n.toasts) > notificationMaxToasts {
		n.toasts = append([]Notification(nil), n.toasts[len(n.toasts)-notificationMaxToasts:]...)
	}
}

func (n *NotificationCenter) trimHistory() {
	if n.historyCap < 0 {
		n.historyCap = 0
	}
	if len(n.history) > n.historyCap {
		n.history = append([]Notification(nil), n.history[len(n.history)-n.historyCap:]...)
	}
}

// Tick expires auto-dismissed toasts and reports whether anything changed.
func (n *NotificationCenter) Tick(now time.Time) bool {
	if n == nil || len(n.toasts) == 0 {
		return false
	}
	kept := n.toasts[:0]
	changed := false
	for _, item := range n.toasts {
		if item.Severity == NotifyError {
			kept = append(kept, item)
			continue
		}
		if n.timeout > 0 && now.Sub(item.Time) < n.timeout {
			kept = append(kept, item)
			continue
		}
		changed = true
	}
	n.toasts = kept
	return changed
}

// Dismiss removes persistent errors first, then the newest toast. It reports
// whether anything was dismissed.
func (n *NotificationCenter) Dismiss() bool {
	if n == nil || len(n.toasts) == 0 {
		return false
	}
	for index, item := range n.toasts {
		if item.Severity == NotifyError {
			n.toasts = append(n.toasts[:index], n.toasts[index+1:]...)
			return true
		}
	}
	n.toasts = n.toasts[:len(n.toasts)-1]
	return true
}

// HasError reports whether a persistent error toast is visible.
func (n *NotificationCenter) HasError() bool {
	if n == nil {
		return false
	}
	for _, item := range n.toasts {
		if item.Severity == NotifyError {
			return true
		}
	}
	return false
}

// Active returns the newest visible notification for the status bar.
func (n *NotificationCenter) Active() (Notification, bool) {
	if n == nil || len(n.toasts) == 0 {
		return Notification{}, false
	}
	return n.toasts[len(n.toasts)-1], true
}

// Toasts returns a copy of the current stack, oldest first.
func (n *NotificationCenter) Toasts() []Notification {
	if n == nil {
		return nil
	}
	return append([]Notification(nil), n.toasts...)
}

// HistoryEntries returns a copy of the bounded history, oldest first.
func (n *NotificationCenter) HistoryEntries() []Notification {
	if n == nil {
		return nil
	}
	return append([]Notification(nil), n.history...)
}

func notificationColor(severity NotificationSeverity) uint8 {
	switch severity {
	case NotifyError:
		return 8
	case NotifyWarning:
		return 9
	case NotifySuccess:
		return 11
	default:
		return 6
	}
}

func notificationLabel(severity NotificationSeverity) string {
	switch severity {
	case NotifyError:
		return "error"
	case NotifyWarning:
		return "warn"
	case NotifySuccess:
		return "ok"
	default:
		return "info"
	}
}

func (g *Game) drawNotifications(surface *render.Surface) {
	if g.notifications == nil {
		return
	}
	toasts := g.notifications.Toasts()
	for index, item := range toasts {
		width := notificationToastCells * font.NarrowWidth
		height := 2 * font.LineAdvance
		x := surface.Width - width - font.NarrowWidth
		y := font.NarrowWidth + index*(height+font.NarrowWidth)
		drawPanel(surface, x, y, width, height)
		color := notificationColor(item.Severity)
		surface.FillRect(x, y, width, 1, color)
		text := fmt.Sprintf("%s: %s", notificationLabel(item.Severity), item.Message)
		if item.Count > 1 {
			text += fmt.Sprintf(" (x%d)", item.Count)
		}
		surface.DrawText(x+font.NarrowWidth, y+font.CellHeight/2+1, clipCells(text, notificationToastCells-2), color, render.DefaultBackground)
	}
}

// messagesOverlay renders the notification history as a scrollable help panel.
func (g *Game) showMessages() bool {
	if g.session == nil || g.notifications == nil {
		return false
	}
	entries := g.notifications.HistoryEntries()
	lines := make([]string, 0, len(entries))
	for _, item := range entries {
		lines = append(lines, fmt.Sprintf("%s %-5s %s", item.Time.Format("15:04:05"), notificationLabel(item.Severity), item.Message))
	}
	if len(lines) == 0 {
		lines = []string{"no messages"}
	}
	g.session.Help = &HelpOverlay{Title: "Messages", Lines: lines}
	return true
}
