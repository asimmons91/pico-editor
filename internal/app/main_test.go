package app

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain isolates XDG config, state, and recovery directories so tests never
// read a developer's real settings or startup recovery journal.
func TestMain(m *testing.M) {
	base, err := os.MkdirTemp("", "pico-editor-app-test-")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "config"))
	_ = os.Setenv("XDG_STATE_HOME", filepath.Join(base, "state"))
	_ = os.Setenv("HOME", filepath.Join(base, "home"))
	disableRecoveryForTests = true
	code := m.Run()
	_ = os.RemoveAll(base)
	os.Exit(code)
}
