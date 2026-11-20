package input

import (
	"context"
	"errors"
	"strings"
	"testing"

	"golang.design/x/clipboard"
)

func TestClipboardUnavailableIsTyped(t *testing.T) {
	calls := 0
	c := newSystemClipboard(
		func() error { calls++; return errors.New("no display") },
		func(context.Context) ([]byte, error) { return nil, nil },
		func(context.Context, []byte) error { return nil },
	)
	if _, err := c.ReadText(); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ReadText() error = %v, want ErrUnavailable", err)
	}
	if err := c.WriteText("x"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("WriteText() error = %v, want ErrUnavailable", err)
	}
	if calls != 1 {
		t.Errorf("init calls = %d, want 1", calls)
	}
}

func TestClipboardReadStripsBOMAndEmptyIsNil(t *testing.T) {
	c := newSystemClipboard(
		func() error { return nil },
		func(context.Context) ([]byte, error) {
			return append([]byte{0xef, 0xbb, 0xbf}, []byte("hello")...), nil
		},
		func(context.Context, []byte) error { return nil },
	)
	text, err := c.ReadText()
	if err != nil {
		t.Fatalf("ReadText() error = %v", err)
	}
	if text != "hello" {
		t.Errorf("ReadText() = %q, want hello", text)
	}

	empty := newSystemClipboard(
		func() error { return nil },
		func(context.Context) ([]byte, error) { return nil, clipboard.ErrNoData },
		func(context.Context, []byte) error { return nil },
	)
	text, err = empty.ReadText()
	if err != nil || text != "" {
		t.Errorf("empty ReadText() = (%q, %v), want (\"\", nil)", text, err)
	}
}

func TestClipboardWriteText(t *testing.T) {
	var wrote string
	c := newSystemClipboard(
		func() error { return nil },
		func(context.Context) ([]byte, error) { return nil, nil },
		func(_ context.Context, data []byte) error { wrote = string(data); return nil },
	)
	if err := c.WriteText("héllo\n"); err != nil {
		t.Fatalf("WriteText() error = %v", err)
	}
	if wrote != "héllo\n" {
		t.Errorf("wrote %q", wrote)
	}
}

func TestNormalizeNewlines(t *testing.T) {
	cases := []struct{ in, want string }{
		{"a\r\nb", "a\nb"},
		{"a\rb", "a\rb"},
		{"a\nb", "a\nb"},
		{"\r\n\r\n", "\n\n"},
	}
	for _, tc := range cases {
		if got := NormalizeNewlines(tc.in); got != tc.want {
			t.Errorf("NormalizeNewlines(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if got := NormalizeNewlines(strings.Repeat("\r\n", 1000)); strings.Contains(got, "\r") {
		t.Error("NormalizeNewlines left a CRLF behind")
	}
}

func TestClipboardReadError(t *testing.T) {
	c := newSystemClipboard(
		func() error { return nil },
		func(context.Context) ([]byte, error) { return nil, errors.New("boom") },
		func(context.Context, []byte) error { return nil },
	)
	if _, err := c.ReadText(); err == nil || !strings.Contains(err.Error(), "read clipboard") {
		t.Fatalf("ReadText() error = %v, want a read error", err)
	}
}

func TestClipboardWriteError(t *testing.T) {
	c := newSystemClipboard(
		func() error { return nil },
		func(context.Context) ([]byte, error) { return nil, nil },
		func(context.Context, []byte) error { return errors.New("boom") },
	)
	if err := c.WriteText("x"); err == nil || !strings.Contains(err.Error(), "write clipboard") {
		t.Fatalf("WriteText() error = %v, want a write error", err)
	}
}

func TestClipboardRuntimeUnavailableIsTyped(t *testing.T) {
	for _, unavailable := range []error{clipboard.ErrUnavailable, clipboard.ErrUnsupported} {
		c := newSystemClipboard(
			func() error { return nil },
			func(context.Context) ([]byte, error) { return nil, unavailable },
			func(context.Context, []byte) error { return unavailable },
		)
		if _, err := c.ReadText(); !errors.Is(err, ErrUnavailable) {
			t.Errorf("ReadText() error = %v, want ErrUnavailable for %v", err, unavailable)
		}
		if err := c.WriteText("x"); !errors.Is(err, ErrUnavailable) {
			t.Errorf("WriteText() error = %v, want ErrUnavailable for %v", err, unavailable)
		}
	}
}

func TestClipboardInitIsLazy(t *testing.T) {
	calls := 0
	c := newSystemClipboard(
		func() error { calls++; return nil },
		func(context.Context) ([]byte, error) { return []byte("x"), nil },
		func(context.Context, []byte) error { return nil },
	)
	if calls != 0 {
		t.Fatalf("init calls before first use = %d, want 0", calls)
	}
	if _, err := c.ReadText(); err != nil {
		t.Fatalf("ReadText() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("init calls after ReadText = %d, want 1", calls)
	}
	if err := c.WriteText("x"); err != nil {
		t.Fatalf("WriteText() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("init calls after WriteText = %d, want 1", calls)
	}
}

func TestNewSystemClipboard(t *testing.T) {
	if NewSystemClipboard() == nil {
		t.Fatal("NewSystemClipboard returned nil")
	}
}
