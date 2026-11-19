package app

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestUpdateRecoversFramePanic(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	game := New(Config{Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	game.step = func() error {
		panic("boom")
	}

	err := game.Update()
	if err == nil {
		t.Fatal("Update() = nil, want a fatal error")
	}
	if !strings.Contains(err.Error(), "panic in update: boom") {
		t.Errorf("Update() error = %q, want it to describe the panic", err)
	}

	output := logs.String()
	if !strings.Contains(output, "msg=panic") {
		t.Errorf("log output = %q, want a panic record", output)
	}
	if !strings.Contains(output, "where=update") {
		t.Errorf("log output = %q, want the callback name", output)
	}
	if !strings.Contains(output, "stack=") {
		t.Errorf("log output = %q, want a stack trace", output)
	}

	if next := game.Update(); next != err {
		t.Errorf("second Update() = %v, want the original fatal error %v", next, err)
	}
}

func TestUpdateWithoutPanic(t *testing.T) {
	t.Parallel()

	game := New(Config{})
	game.step = func() error { return nil }

	if err := game.Update(); err != nil {
		t.Errorf("Update() = %v, want nil", err)
	}
}
