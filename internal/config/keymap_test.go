package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/asimmons91/pico-editor/internal/input"
)

func TestDefaultKeysAreCanonicalAndUniquePerMode(t *testing.T) {
	settings := DefaultSettings()
	for mode, bindings := range settings.Keys {
		if !validMode(mode) {
			t.Errorf("unknown default mode %q", mode)
		}
		for key, command := range bindings {
			if !validCommandSpelling(command) {
				t.Errorf("%s.%s has invalid command %q", mode, key, command)
			}
			parsed, mods, err := input.ParseKey(key)
			if err != nil {
				t.Errorf("%s.%s: %v", mode, key, err)
				continue
			}
			if canonical := input.FormatKey(parsed, mods); canonical != key {
				t.Errorf("%s.%s is not canonical (want %s)", mode, key, canonical)
			}
		}
	}
}

func TestKeymapRebindAndUnbind(t *testing.T) {
	root, configHome := testRoot(t)
	writeFile(t, filepath.Join(configHome, "pico-editor", "config.toml"), `
[keys.normal]
"ctrl+s" = "file.close"
"ctrl+shift+b" = ""
f2 = "tree.refresh"
`)
	settings, _, err := LoadSettings(root)
	if err != nil {
		t.Fatal(err)
	}
	if command, _ := settings.CommandFor("normal", "ctrl+s"); command != "file.close" {
		t.Errorf("ctrl+s = %q, want file.close", command)
	}
	if _, ok := settings.CommandFor("normal", "ctrl+shift+b"); ok {
		t.Error("ctrl+shift+b remains bound after the empty unbind")
	}
	if command, _ := settings.CommandFor("normal", "f2"); command != "tree.refresh" {
		t.Errorf("f2 = %q, want tree.refresh", command)
	}
	if got := settings.KeyFor("normal", "file.close"); got != "ctrl+s" {
		t.Errorf("KeyFor(file.close) = %q", got)
	}
}

func TestKeymapRejectsReservedAndGrammarKeys(t *testing.T) {
	root, configHome := testRoot(t)
	writeFile(t, filepath.Join(configHome, "pico-editor", "config.toml"), `
[keys.insert]
"ctrl+p" = "palette.show"
[keys.normal]
d = "file.close"
"ctrl+b" = "tree.toggle"
"not+a+key" = "file.save"
"ctrl+s" = "not a command"
`)
	settings, warnings, err := LoadSettings(root)
	if err != nil {
		t.Fatal(err)
	}
	if command, _ := settings.CommandFor("normal", "ctrl+s"); command != "file.save" {
		t.Errorf("invalid command ID overwrote the default: %q", command)
	}
	if _, ok := settings.CommandFor("normal", "d"); ok {
		t.Error("grammar key d was accepted")
	}
	if _, ok := settings.CommandFor("normal", "ctrl+b"); ok {
		t.Error("reserved ctrl+b was accepted")
	}
	if _, ok := settings.CommandFor("insert", "ctrl+p"); ok {
		t.Error("reserved insert ctrl+p was accepted")
	}
	text := ""
	for _, warning := range warnings {
		text += warning.String() + "\n"
	}
	for _, want := range []string{"reserved", "Vim grammar", "unknown modifier", "invalid command ID"} {
		if !strings.Contains(text, want) {
			t.Errorf("warnings = %q, want %q", text, want)
		}
	}
}

func TestKeymapLayersMergeByKey(t *testing.T) {
	root, configHome := testRoot(t)
	writeFile(t, filepath.Join(configHome, "pico-editor", "config.toml"), `
[keys.normal]
"ctrl+s" = "file.close"
f2 = "file.reload"
`)
	writeFile(t, filepath.Join(root, ".pico-editor.toml"), `
[keys.normal]
"ctrl+s" = "file.save"
`)
	settings, _, err := LoadSettings(root)
	if err != nil {
		t.Fatal(err)
	}
	if command, _ := settings.CommandFor("normal", "ctrl+s"); command != "file.save" {
		t.Errorf("ctrl+s = %q, want the workspace file.save", command)
	}
	if command, _ := settings.CommandFor("normal", "f2"); command != "file.reload" {
		t.Errorf("f2 = %q, want the global file.reload to survive", command)
	}
}

func TestKeymapModeNames(t *testing.T) {
	for _, test := range []struct{ in, want string }{
		{"normal", "normal"},
		{"insert", "insert"},
		{"visual-char", "visual"},
		{"visual-line", "visual"},
		{"visual-block", "visual"},
		{"command", "command"},
		{"unknown", "normal"},
	} {
		if got := KeymapMode(test.in); got != test.want {
			t.Errorf("KeymapMode(%q) = %q, want %q", test.in, got, test.want)
		}
	}
}

func TestLoadWorkspaceGlyphArrayReplaces(t *testing.T) {
	root, configHome := testRoot(t)
	writeFile(t, filepath.Join(configHome, "pico-editor", "config.toml"), `
[[lsp.glyphs]]
name = "global"
byte = 128
`)
	writeFile(t, filepath.Join(root, ".pico-editor.toml"), `
[lsp]
glyphs = []
`)
	settings, _, err := LoadSettings(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.LSP.Glyphs) != 0 {
		t.Fatalf("glyphs = %+v, want an empty workspace array to clear the global glyphs", settings.LSP.Glyphs)
	}
}

func TestLoadWorkspaceGlyphEntriesReplace(t *testing.T) {
	root, configHome := testRoot(t)
	writeFile(t, filepath.Join(configHome, "pico-editor", "config.toml"), `
[[lsp.glyphs]]
name = "global"
byte = 128
`)
	writeFile(t, filepath.Join(root, ".pico-editor.toml"), `
[[lsp.glyphs]]
name = "workspace"
byte = 129
`)
	settings, _, err := LoadSettings(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.LSP.Glyphs) != 1 || settings.LSP.Glyphs[0].Name != "workspace" {
		t.Fatalf("glyphs = %+v, want workspace replacement", settings.LSP.Glyphs)
	}
}

func TestGlobalPathFallsBackToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)
	if got := GlobalPath(); got != filepath.Join(home, ".config", "pico-editor", "config.toml") {
		t.Errorf("GlobalPath = %q", got)
	}
}

func TestWorkspacePath(t *testing.T) {
	if got := WorkspacePath("/tmp/project"); got != filepath.Join("/tmp/project", ".pico-editor.toml") {
		t.Errorf("WorkspacePath = %q", got)
	}
	if got := WorkspacePath(""); got != "" {
		t.Errorf("WorkspacePath(empty) = %q", got)
	}
}
