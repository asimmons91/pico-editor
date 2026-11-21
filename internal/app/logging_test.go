package app

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppLogsDoNotContainFileOrClipboardText(t *testing.T) {
	const fileSecret = "SECRET_FILE_TEXT"
	const clipboardSecret = "SECRET_CLIPBOARD_TEXT"
	root := t.TempDir()
	path := filepath.Join(root, "secret.lua")
	if err := os.WriteFile(path, []byte(fileSecret+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	game := New(Config{
		Logger:    slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		Clipboard: &recordingClipboard{text: clipboardSecret},
		File:      path,
	})
	if err := game.StartupError(); err != nil {
		t.Fatal(err)
	}
	game.source = &fakeSource{}
	if err := sendKeys(t, game, "\"+p"); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		_ = game.Update()
		return strings.Contains(game.session.CurrentEditor().Buffer.Text(), clipboardSecret)
	})
	output := logs.String()
	for _, secret := range []string{fileSecret, clipboardSecret} {
		if strings.Contains(output, secret) {
			t.Fatalf("log output contains %q: %s", secret, output)
		}
	}
}
