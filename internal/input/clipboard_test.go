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
