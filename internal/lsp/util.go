package lsp

import (
	"log/slog"
	"path/filepath"

	"go.lsp.dev/protocol"
)

func baseName(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Base(path)
}

func slogLevel(kind protocol.MessageType) slog.Level {
	switch kind {
	case protocol.MessageTypeError:
		return slog.LevelError
	case protocol.MessageTypeWarning:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}
