package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testRoot(t *testing.T) (root, configHome string) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "project")
	configHome = filepath.Join(base, "config")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", configHome)
	return root, configHome
}

func TestLoadSettingsDefaults(t *testing.T) {
	root, _ := testRoot(t)
	settings, warnings, err := LoadSettings(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	want := DefaultSettings()
	if settings.Editor != want.Editor || settings.Files.MaxEntries != want.Files.MaxEntries ||
		settings.Notifications != want.Notifications || settings.Recovery != want.Recovery {
		t.Fatalf("settings = %+v, want defaults %+v", settings, want)
	}
	if command, ok := settings.CommandFor("normal", "ctrl+s"); !ok || command != "file.save" {
		t.Fatalf("ctrl+s = %q, %t", command, ok)
	}
}

func TestLoadSettingsLayerPrecedence(t *testing.T) {
	root, configHome := testRoot(t)
	writeFile(t, filepath.Join(configHome, "pico-editor", "config.toml"), `
version = 1
[editor]
scale = 3
tab_width = 4
[files]
exclude = ["global"]
max_entries = 100
[keys.normal]
"ctrl+s" = "file.close"
`)
	writeFile(t, filepath.Join(root, ".pico-editor.toml"), `
[editor]
tab_width = 8
[files]
exclude = ["workspace", "tmp"]
[keys.normal]
"ctrl+q" = ""
`)
	settings, _, err := LoadSettings(root)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Editor.Scale != 3 {
		t.Errorf("scale = %d, want global 3", settings.Editor.Scale)
	}
	if settings.Editor.TabWidth != 8 {
		t.Errorf("tab_width = %d, want workspace 8", settings.Editor.TabWidth)
	}
	if len(settings.Files.Exclude) != 2 || settings.Files.Exclude[0] != "workspace" {
		t.Errorf("exclude = %v, want the workspace array to replace the global array", settings.Files.Exclude)
	}
	if settings.Files.MaxEntries != 100 {
		t.Errorf("max_entries = %d, want global 100", settings.Files.MaxEntries)
	}
	if command, _ := settings.CommandFor("normal", "ctrl+s"); command != "file.close" {
		t.Errorf("ctrl+s = %q, want file.close", command)
	}
	if _, ok := settings.CommandFor("normal", "ctrl+q"); ok {
		t.Error("ctrl+q remains bound after the workspace unbind")
	}
}

func TestLoadSettingsEmptyExcludeClearsDefaults(t *testing.T) {
	root, configHome := testRoot(t)
	writeFile(t, filepath.Join(configHome, "pico-editor", "config.toml"), "[files]\nexclude = []\n")
	settings, _, err := LoadSettings(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.Files.Exclude) != 0 {
		t.Fatalf("exclude = %v, want an empty array to replace the defaults", settings.Files.Exclude)
	}
}

func TestLoadSettingsWarnsUnknownAndInvalid(t *testing.T) {
	root, configHome := testRoot(t)
	writeFile(t, filepath.Join(configHome, "pico-editor", "config.toml"), `
version = 1
[editor]
scale = 99
font = "pico"
[files]
exclude = [".git"]
mystery = true
`)
	settings, warnings, err := LoadSettings(root)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Editor.Scale != DefaultSettings().Editor.Scale {
		t.Errorf("scale = %d, want the default after an invalid value", settings.Editor.Scale)
	}
	text := ""
	for _, warning := range warnings {
		text += warning.String() + "\n"
	}
	for _, want := range []string{"editor.scale", "editor.font", "files.mystery", "using default"} {
		if !strings.Contains(text, want) {
			t.Errorf("warnings = %q, want %q", text, want)
		}
	}
}

func TestLoadSettingsMissingVersionIsOne(t *testing.T) {
	root, configHome := testRoot(t)
	writeFile(t, filepath.Join(configHome, "pico-editor", "config.toml"), "[editor]\ntab_width = 3\n")
	settings, _, err := LoadSettings(root)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Editor.TabWidth != 3 {
		t.Errorf("tab_width = %d, want 3", settings.Editor.TabWidth)
	}
}

func TestLoadSettingsRejectsUnsupportedVersion(t *testing.T) {
	root, configHome := testRoot(t)
	path := filepath.Join(configHome, "pico-editor", "config.toml")
	writeFile(t, path, "version = 2\n")
	if _, _, err := LoadSettings(root); err == nil || !strings.Contains(err.Error(), "unsupported config version") {
		t.Fatalf("LoadSettings err = %v, want unsupported version", err)
	}
}

func TestLoadSettingsWorkspaceErrorKeepsGlobal(t *testing.T) {
	root, configHome := testRoot(t)
	writeFile(t, filepath.Join(configHome, "pico-editor", "config.toml"), "[editor]\ntab_width = 3\n")
	writeFile(t, filepath.Join(root, ".pico-editor.toml"), "version = 2\n")
	settings, warnings, err := LoadSettings(root)
	if err != nil {
		t.Fatalf("workspace error should not fail startup: %v", err)
	}
	if settings.Editor.TabWidth != 3 {
		t.Errorf("tab_width = %d, want the global 3", settings.Editor.TabWidth)
	}
	found := false
	for _, warning := range warnings {
		if strings.Contains(warning.Message, "using the global layer") {
			found = true
		}
	}
	if !found {
		t.Errorf("warnings = %v, want a workspace fallback", warnings)
	}
}

func TestLoadSettingsMalformedGlobalIsError(t *testing.T) {
	root, configHome := testRoot(t)
	path := filepath.Join(configHome, "pico-editor", "config.toml")
	writeFile(t, path, "version = [\n")
	_, _, err := LoadSettings(root)
	if err == nil {
		t.Fatal("LoadSettings accepted malformed TOML")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %v, want the path", err)
	}
}

func TestLoadSettingsCustomGlyphs(t *testing.T) {
	root, configHome := testRoot(t)
	writeFile(t, filepath.Join(configHome, "pico-editor", "config.toml"), `
version = 1
[[lsp.glyphs]]
name = "logo"
byte = 128
context = "both"
aliases = ["mark"]
description = "project mark"
`)
	settings, warnings, err := LoadSettings(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if len(settings.LSP.Glyphs) != 1 {
		t.Fatalf("glyphs = %+v", settings.LSP.Glyphs)
	}
	glyph := settings.LSP.Glyphs[0]
	if glyph.Name != "logo" || glyph.Byte == nil || *glyph.Byte != 128 || glyph.Context != "both" {
		t.Fatalf("glyph = %+v", glyph)
	}
	if len(glyph.Aliases) != 1 || glyph.Aliases[0] != "mark" {
		t.Fatalf("aliases = %v", glyph.Aliases)
	}
}

func TestSettingsCloneIsDeep(t *testing.T) {
	settings := DefaultSettings()
	settings.Files.Exclude[0] = "changed"
	settings.Keys["normal"]["ctrl+s"] = "file.close"
	clone := settings.Clone()
	clone.Files.Exclude[0] = "clone"
	clone.Keys["normal"]["ctrl+s"] = "file.reload"
	if settings.Files.Exclude[0] != "changed" {
		t.Error("Clone shared the exclude slice")
	}
	if settings.Keys["normal"]["ctrl+s"] != "file.close" {
		t.Error("Clone shared the keymap")
	}
}

func TestPathsRespectXDG(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(base, "state"))
	t.Setenv("HOME", filepath.Join(base, "home"))
	if got := GlobalPath(); got != filepath.Join(base, "config", "pico-editor", "config.toml") {
		t.Errorf("GlobalPath = %q", got)
	}
	if got := RecoveryDir(); got != filepath.Join(base, "state", "pico-editor", "recovery") {
		t.Errorf("RecoveryDir = %q", got)
	}
	if got := LogPath(); got != filepath.Join(base, "state", "pico-editor", "pico-editor.log") {
		t.Errorf("LogPath = %q", got)
	}
}
