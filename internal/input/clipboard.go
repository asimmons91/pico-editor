package input

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"golang.design/x/clipboard"
)

// ErrUnavailable reports that the host has no usable clipboard. Callers
// should degrade to internal registers.
var ErrUnavailable = errors.New("clipboard unavailable")

// Clipboard is the editor's narrow clipboard seam.
type Clipboard interface {
	ReadText() (string, error)
	WriteText(string) error
}

// SystemClipboard uses golang.design/x/clipboard, initialized lazily on first
// use so headless runs never pay for or depend on a display connection.
type SystemClipboard struct {
	once    sync.Once
	initErr error
	init    func() error
	read    func(context.Context) ([]byte, error)
	write   func(context.Context, []byte) error
}

// NewSystemClipboard returns a clipboard backed by the host clipboard.
func NewSystemClipboard() *SystemClipboard {
	return newSystemClipboard(clipboard.Init,
		func(ctx context.Context) ([]byte, error) {
			return clipboard.Read(ctx, clipboard.FmtText)
		},
		func(ctx context.Context, data []byte) error {
			_, err := clipboard.Write(ctx, clipboard.FmtText, data)
			return err
		},
	)
}

func newSystemClipboard(init func() error, read func(context.Context) ([]byte, error), write func(context.Context, []byte) error) *SystemClipboard {
	return &SystemClipboard{init: init, read: read, write: write}
}

// ReadText returns clipboard text as UTF-8 with a leading BOM removed.
func (c *SystemClipboard) ReadText() (string, error) {
	if err := c.initialize(); err != nil {
		return "", err
	}
	data, err := c.read(context.Background())
	if errors.Is(err, clipboard.ErrNoData) {
		return "", nil
	}
	if errors.Is(err, clipboard.ErrUnavailable) || errors.Is(err, clipboard.ErrUnsupported) {
		return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if err != nil {
		return "", fmt.Errorf("read clipboard: %w", err)
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	return string(data), nil
}

// WriteText writes UTF-8 text and never writes binary data.
func (c *SystemClipboard) WriteText(text string) error {
	if err := c.initialize(); err != nil {
		return err
	}
	if err := c.write(context.Background(), []byte(text)); err != nil {
		if errors.Is(err, clipboard.ErrUnavailable) || errors.Is(err, clipboard.ErrUnsupported) {
			return fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		return fmt.Errorf("write clipboard: %w", err)
	}
	return nil
}

func (c *SystemClipboard) initialize() error {
	c.once.Do(func() {
		init := c.init
		if init == nil {
			init = clipboard.Init
		}
		c.initErr = init()
	})
	if c.initErr != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, c.initErr)
	}
	return nil
}

// NormalizeNewlines converts CRLF to LF. It deliberately leaves a lone CR
// alone because CR is a valid P8SCII byte. Callers apply this at paste sites,
// never inside the clipboard adapter.
func NormalizeNewlines(text string) string {
	return strings.ReplaceAll(text, "\r\n", "\n")
}
