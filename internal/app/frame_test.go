package app

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestFirstFrameLoggedOnce(t *testing.T) {
	var logs bytes.Buffer
	game := New(Config{Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	game.markFirstFrame()
	game.markFirstFrame()
	if got := strings.Count(logs.String(), "msg=first-frame"); got != 1 {
		t.Errorf("first-frame log count = %d, want 1\n%s", got, logs.String())
	}
}
