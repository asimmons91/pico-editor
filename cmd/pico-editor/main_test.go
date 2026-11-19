package main

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run(--version) = %d, want 0; stderr: %s", code, stderr.String())
	}

	got := strings.TrimSpace(stdout.String())
	want := "pico-editor dev (unknown, unknown)"
	if got != want {
		t.Errorf("version output = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunHelp(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run(--help) = %d, want 0", code)
	}
	if !strings.Contains(stderr.String(), "usage: pico-editor") {
		t.Errorf("stderr = %q, want usage output", stderr.String())
	}
}

func TestRunRejectsBadFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "unknown flag", args: []string{"--nope"}},
		{name: "invalid log level", args: []string{"--log-level", "verbose"}},
		{name: "unexpected argument", args: []string{"cart.p8"}},
		{name: "empty log level", args: []string{"--log-level", ""}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			if code := run(test.args, &stdout, &stderr); code != 2 {
				t.Errorf("run(%v) = %d, want 2", test.args, code)
			}
			if !strings.Contains(stderr.String(), "usage: pico-editor") {
				t.Errorf("run(%v) stderr = %q, want usage output", test.args, stderr.String())
			}
		})
	}
}

func TestParseLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text string
		want slog.Level
	}{
		{text: "debug", want: slog.LevelDebug},
		{text: "info", want: slog.LevelInfo},
		{text: "warn", want: slog.LevelWarn},
		{text: "error", want: slog.LevelError},
	}

	for _, test := range tests {
		t.Run(test.text, func(t *testing.T) {
			t.Parallel()

			got, err := parseLevel(test.text)
			if err != nil {
				t.Fatalf("parseLevel(%q): %v", test.text, err)
			}
			if got != test.want {
				t.Errorf("parseLevel(%q) = %v, want %v", test.text, got, test.want)
			}
		})
	}

	if _, err := parseLevel("INFO"); err == nil {
		t.Error("parseLevel(INFO) succeeded, want an error for non-canonical levels")
	}
}

func TestSetupLoggerWritesPrivateLogFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state", "pico-editor.log")
	logger, closeLog := setupLogger(slog.LevelInfo, path, &bytes.Buffer{})
	logger.Info("hello", "key", "value")
	closeLog()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}
	if !strings.Contains(string(data), "msg=hello") {
		t.Errorf("log file = %q, want it to contain msg=hello", data)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat log file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("log file mode = %o, want 600", got)
	}

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat log directory: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Errorf("log directory mode = %o, want 700", got)
	}
}

func TestSetupLoggerFallsBackToStderr(t *testing.T) {
	t.Parallel()

	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("creating blocker file: %v", err)
	}

	var stderr bytes.Buffer
	logger, closeLog := setupLogger(slog.LevelInfo, filepath.Join(blocker, "pico-editor.log"), &stderr)
	logger.Warn("fallback in use")
	closeLog()

	output := stderr.String()
	if !strings.Contains(output, "cannot create log directory") {
		t.Errorf("stderr = %q, want a log directory warning", output)
	}
	if !strings.Contains(output, "fallback in use") {
		t.Errorf("stderr = %q, want fallback log output", output)
	}
}

func TestSetupLoggerFallsBackWhenFileCannotBeOpened(t *testing.T) {
	t.Parallel()

	// Passing a directory as the log path fails at OpenFile rather than at
	// MkdirAll, exercising the second fallback branch.
	path := t.TempDir()

	var stderr bytes.Buffer
	logger, closeLog := setupLogger(slog.LevelInfo, path, &stderr)
	logger.Warn("fallback in use")
	closeLog()

	output := stderr.String()
	if !strings.Contains(output, "cannot open log file") {
		t.Errorf("stderr = %q, want a log file warning", output)
	}
	if !strings.Contains(output, "fallback in use") {
		t.Errorf("stderr = %q, want fallback log output", output)
	}
}

func TestSetupLoggerHonorsLevel(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "pico-editor.log")
	logger, closeLog := setupLogger(slog.LevelError, path, &bytes.Buffer{})
	logger.Info("hidden message")
	logger.Error("visible message")
	closeLog()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}
	if strings.Contains(string(data), "hidden message") {
		t.Errorf("log file = %q, want info records filtered out", data)
	}
	if !strings.Contains(string(data), "visible message") {
		t.Errorf("log file = %q, want error records present", data)
	}
}

func TestDefaultLogPath(t *testing.T) {
	t.Run("xdg state home", func(t *testing.T) {
		stateHome := t.TempDir()
		t.Setenv("XDG_STATE_HOME", stateHome)

		want := filepath.Join(stateHome, "pico-editor", "pico-editor.log")
		if got := defaultLogPath(); got != want {
			t.Errorf("defaultLogPath() = %q, want %q", got, want)
		}
	})

	t.Run("home fallback", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("XDG_STATE_HOME", "")
		t.Setenv("HOME", home)

		want := filepath.Join(home, ".local", "state", "pico-editor", "pico-editor.log")
		if got := defaultLogPath(); got != want {
			t.Errorf("defaultLogPath() = %q, want %q", got, want)
		}
	})

	t.Run("relative xdg state home is ignored", func(t *testing.T) {
		t.Setenv("XDG_STATE_HOME", "relative/path")

		got := defaultLogPath()
		if !filepath.IsAbs(got) {
			t.Errorf("defaultLogPath() = %q, want an absolute path", got)
		}
	})
}
