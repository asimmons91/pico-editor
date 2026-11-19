package app

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
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

func TestUpdateForwardsStepError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("step failed")
	game := New(Config{})
	game.step = func() error { return sentinel }

	if err := game.Update(); !errors.Is(err, sentinel) {
		t.Errorf("Update() = %v, want %v", err, sentinel)
	}
}

func TestUpdateForwardsTermination(t *testing.T) {
	t.Parallel()

	game := New(Config{})
	game.step = func() error { return ebiten.Termination }

	if err := game.Update(); err != ebiten.Termination {
		t.Errorf("Update() = %v, want ebiten.Termination", err)
	}
}

func TestNewInitializesStep(t *testing.T) {
	t.Parallel()

	if game := New(Config{}); game.step == nil {
		t.Error("New() left the update step nil")
	}
}

func TestDoFrameRecordsPanic(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	game := New(Config{Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	game.doFrame("draw", func() {
		panic("boom")
	})

	if game.fatalErr == nil {
		t.Fatal("doFrame() recorded no error")
	}
	if !strings.Contains(game.fatalErr.Error(), "panic in draw: boom") {
		t.Errorf("fatal error = %q, want it to describe the panic", game.fatalErr)
	}
	if !strings.Contains(logs.String(), "where=draw") {
		t.Errorf("log output = %q, want the callback name", logs.String())
	}

	called := false
	game.doFrame("draw", func() {
		called = true
	})
	if called {
		t.Error("doFrame() ran after a fatal frame error")
	}
}
