// Package config owns the two-layer TOML settings file and the validated
// command keymap. Settings are immutable after load; a reload builds a new
// value and the app swaps it at a frame boundary.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Version is the only settings schema this V1 understands. A missing version
// is treated as 1 for forward compatibility.
const Version = 1

// Settings is the merged global and workspace configuration.
type Settings struct {
	Version       int
	Editor        EditorSettings
	Files         FileSettings
	LSP           LSPSettings
	Notifications NotificationSettings
	Recovery      RecoverySettings
	// Keys maps a mode (normal, insert, visual, command) to key -> command ID.
	Keys map[string]map[string]string
}

// EditorSettings control the buffer and window look.
type EditorSettings struct {
	Scale     int // 0 selects automatic sizing; 1..8 is fixed
	TabWidth  int
	ScrollOff int
	BlinkMS   int
}

// FileSettings control the workspace scan.
type FileSettings struct {
	ShowHidden bool
	Exclude    []string
	MaxEntries int
}

// LSPSettings control pico-lsp and custom glyph definitions.
type LSPSettings struct {
	Enabled      bool
	AutoComplete bool
	DebounceMS   int
	Glyphs       []Glyph
}

// Glyph is one custom pico8.glyphs definition. Exactly one of Byte, Glyph, or
// Text should be set; pico-lsp validates the combination.
type Glyph struct {
	Name        string   `toml:"name"`
	Glyph       string   `toml:"glyph"`
	Byte        *int     `toml:"byte"`
	Text        string   `toml:"text"`
	Context     string   `toml:"context"`
	Aliases     []string `toml:"aliases"`
	Description string   `toml:"description"`
}

// NotificationSettings control toasts and history.
type NotificationSettings struct {
	TimeoutMS int
	History   int
}

// RecoverySettings control the crash-recovery journal.
type RecoverySettings struct {
	Enabled    bool
	IntervalMS int
}

// Warning is one recoverable configuration problem. Path and Key may be empty.
type Warning struct {
	Path    string
	Key     string
	Message string
}

// String renders a warning for notifications and logs.
func (w Warning) String() string {
	switch {
	case w.Path != "" && w.Key != "":
		return fmt.Sprintf("%s: %s: %s", w.Path, w.Key, w.Message)
	case w.Path != "":
		return fmt.Sprintf("%s: %s", w.Path, w.Message)
	default:
		return w.Message
	}
}

// DefaultSettings returns the documented V1 defaults. Every call returns fresh
// slices and maps so callers may mutate the result.
func DefaultSettings() Settings {
	return Settings{
		Version: Version,
		Editor: EditorSettings{
			Scale:     4,
			TabWidth:  2,
			ScrollOff: 2,
			BlinkMS:   500,
		},
		Files: FileSettings{
			ShowHidden: false,
			Exclude:    []string{".git", "node_modules", "vendor", "bin", "dist"},
			MaxEntries: 20000,
		},
		LSP: LSPSettings{
			Enabled:      true,
			AutoComplete: true,
			DebounceMS:   120,
		},
		Notifications: NotificationSettings{
			TimeoutMS: 4000,
			History:   100,
		},
		Recovery: RecoverySettings{
			Enabled:    true,
			IntervalMS: 2000,
		},
		Keys: DefaultKeys(),
	}
}

// Clone returns a deep copy of s.
func (s Settings) Clone() Settings {
	out := s
	out.Files.Exclude = append([]string(nil), s.Files.Exclude...)
	out.LSP.Glyphs = append([]Glyph(nil), s.LSP.Glyphs...)
	for index := range out.LSP.Glyphs {
		out.LSP.Glyphs[index].Aliases = append([]string(nil), s.LSP.Glyphs[index].Aliases...)
	}
	out.Keys = make(map[string]map[string]string, len(s.Keys))
	for mode, bindings := range s.Keys {
		copied := make(map[string]string, len(bindings))
		for key, command := range bindings {
			copied[key] = command
		}
		out.Keys[mode] = copied
	}
	return out
}

type layer struct {
	Version *int `toml:"version"`

	Editor struct {
		Scale     *int `toml:"scale"`
		TabWidth  *int `toml:"tab_width"`
		ScrollOff *int `toml:"scrolloff"`
		BlinkMS   *int `toml:"blink_ms"`
	} `toml:"editor"`

	Files struct {
		ShowHidden *bool     `toml:"show_hidden"`
		Exclude    *[]string `toml:"exclude"`
		MaxEntries *int      `toml:"max_entries"`
	} `toml:"files"`

	LSP struct {
		Enabled      *bool   `toml:"enabled"`
		AutoComplete *bool   `toml:"auto_complete"`
		DebounceMS   *int    `toml:"debounce_ms"`
		Glyphs       []Glyph `toml:"glyphs"`
	} `toml:"lsp"`

	Notifications struct {
		TimeoutMS *int `toml:"timeout_ms"`
		History   *int `toml:"history"`
	} `toml:"notifications"`

	Recovery struct {
		Enabled    *bool `toml:"enabled"`
		IntervalMS *int  `toml:"interval_ms"`
	} `toml:"recovery"`

	Keys map[string]map[string]string `toml:"keys"`
}

// LoadSettings reads the global and workspace layers for root.
//
// A malformed global file is returned as an error so startup can offer "Use
// defaults" or "Quit". A malformed workspace file is a warning: the global
// layer is still usable and the editor remains fully functional.
func LoadSettings(root string) (Settings, []Warning, error) {
	settings := DefaultSettings()
	var warnings []Warning

	globalPath := GlobalPath()
	if globalPath != "" {
		loaded, fileWarnings, err := loadLayer(globalPath)
		warnings = append(warnings, fileWarnings...)
		if err != nil {
			return settings, warnings, err
		}
		if loaded != nil {
			applyLayer(&settings, loaded, globalPath, &warnings)
		}
	}

	if workspacePath := WorkspacePath(root); workspacePath != "" {
		loaded, fileWarnings, err := loadLayer(workspacePath)
		warnings = append(warnings, fileWarnings...)
		if err != nil {
			warnings = append(warnings, Warning{Path: workspacePath, Message: err.Error() + "; using the global layer"})
		} else if loaded != nil {
			applyLayer(&settings, loaded, workspacePath, &warnings)
		}
	}

	settings.Keys = normalizeKeys(settings.Keys)
	settings.LSP.Glyphs = validateGlyphs(settings.LSP.Glyphs, &warnings)
	return settings, warnings, nil
}

// validateGlyphs applies the documented custom-glyph shape before pico-lsp
// sees it. Invalid entries are skipped with a warning.
func validateGlyphs(glyphs []Glyph, warnings *[]Warning) []Glyph {
	if len(glyphs) == 0 {
		return glyphs
	}
	out := make([]Glyph, 0, len(glyphs))
	for _, glyph := range glyphs {
		name := strings.TrimSpace(glyph.Name)
		if name == "" {
			*warnings = append(*warnings, Warning{Key: "lsp.glyphs", Message: "glyph without a name ignored"})
			continue
		}
		set := 0
		if glyph.Byte != nil {
			if *glyph.Byte < 0 || *glyph.Byte > 255 {
				*warnings = append(*warnings, Warning{Key: "lsp.glyphs." + name, Message: fmt.Sprintf("byte %d is outside 0..255; glyph ignored", *glyph.Byte)})
				continue
			}
			set++
		}
		if glyph.Glyph != "" {
			set++
		}
		if glyph.Text != "" {
			set++
		}
		if set != 1 {
			*warnings = append(*warnings, Warning{Key: "lsp.glyphs." + name, Message: "exactly one of byte, glyph, or text is required; glyph ignored"})
			continue
		}
		switch glyph.Context {
		case "", "both", "string":
		default:
			*warnings = append(*warnings, Warning{Key: "lsp.glyphs." + name, Message: fmt.Sprintf("unknown context %q; using both", glyph.Context)})
			glyph.Context = "both"
		}
		glyph.Name = name
		out = append(out, glyph)
	}
	return out
}

// normalizeKeys guarantees every keymap section exists after merging.
func normalizeKeys(keys map[string]map[string]string) map[string]map[string]string {
	for _, mode := range []string{"normal", "insert", "visual", "command"} {
		if keys[mode] == nil {
			keys[mode] = make(map[string]string)
		}
	}
	return keys
}

// loadLayer returns nil when path does not exist.
func loadLayer(path string) (*layer, []Warning, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	var loaded layer
	metadata, err := toml.Decode(string(data), &loaded)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	var warnings []Warning
	if version := effectiveVersion(loaded.Version); version != Version {
		return nil, warnings, fmt.Errorf("%s: unsupported config version %d (want %d)", path, version, Version)
	}
	for _, key := range metadata.Undecoded() {
		warnings = append(warnings, Warning{Path: path, Key: key.String(), Message: "unknown key ignored"})
	}
	return &loaded, warnings, nil
}

func effectiveVersion(version *int) int {
	if version == nil {
		return Version
	}
	return *version
}

func applyLayer(settings *Settings, loaded *layer, path string, warnings *[]Warning) {
	defaults := DefaultSettings()

	if loaded.Editor.Scale != nil {
		value := *loaded.Editor.Scale
		if value < 0 || value > 8 {
			warnValue(warnings, path, "editor.scale", value, "want 0 (auto) or 1..8; using default")
			fallback(&settings.Editor.Scale, defaults.Editor.Scale)
		} else {
			settings.Editor.Scale = value
		}
	}
	if loaded.Editor.TabWidth != nil {
		value := *loaded.Editor.TabWidth
		if value < 1 || value > 16 {
			warnValue(warnings, path, "editor.tab_width", value, "want 1..16; using default")
			fallback(&settings.Editor.TabWidth, defaults.Editor.TabWidth)
		} else {
			settings.Editor.TabWidth = value
		}
	}
	if loaded.Editor.ScrollOff != nil {
		value := *loaded.Editor.ScrollOff
		if value < 0 || value > 32 {
			warnValue(warnings, path, "editor.scrolloff", value, "want 0..32; using default")
			fallback(&settings.Editor.ScrollOff, defaults.Editor.ScrollOff)
		} else {
			settings.Editor.ScrollOff = value
		}
	}
	if loaded.Editor.BlinkMS != nil {
		value := *loaded.Editor.BlinkMS
		if value < 0 || value > 60000 {
			warnValue(warnings, path, "editor.blink_ms", value, "want 0..60000; using default")
			fallback(&settings.Editor.BlinkMS, defaults.Editor.BlinkMS)
		} else {
			settings.Editor.BlinkMS = value
		}
	}

	if loaded.Files.ShowHidden != nil {
		settings.Files.ShowHidden = *loaded.Files.ShowHidden
	}
	if loaded.Files.Exclude != nil {
		settings.Files.Exclude = append([]string{}, (*loaded.Files.Exclude)...)
	}
	if loaded.Files.MaxEntries != nil {
		value := *loaded.Files.MaxEntries
		if value < 1 || value > 1_000_000 {
			warnValue(warnings, path, "files.max_entries", value, "want 1..1000000; using default")
			fallback(&settings.Files.MaxEntries, defaults.Files.MaxEntries)
		} else {
			settings.Files.MaxEntries = value
		}
	}

	if loaded.LSP.Enabled != nil {
		settings.LSP.Enabled = *loaded.LSP.Enabled
	}
	if loaded.LSP.AutoComplete != nil {
		settings.LSP.AutoComplete = *loaded.LSP.AutoComplete
	}
	if loaded.LSP.DebounceMS != nil {
		value := *loaded.LSP.DebounceMS
		if value < 0 || value > 5000 {
			warnValue(warnings, path, "lsp.debounce_ms", value, "want 0..5000; using default")
			fallback(&settings.LSP.DebounceMS, defaults.LSP.DebounceMS)
		} else {
			settings.LSP.DebounceMS = value
		}
	}
	if loaded.LSP.Glyphs != nil {
		settings.LSP.Glyphs = append([]Glyph(nil), loaded.LSP.Glyphs...)
	}

	if loaded.Notifications.TimeoutMS != nil {
		value := *loaded.Notifications.TimeoutMS
		if value < 0 || value > 600000 {
			warnValue(warnings, path, "notifications.timeout_ms", value, "want 0..600000; using default")
			fallback(&settings.Notifications.TimeoutMS, defaults.Notifications.TimeoutMS)
		} else {
			settings.Notifications.TimeoutMS = value
		}
	}
	if loaded.Notifications.History != nil {
		value := *loaded.Notifications.History
		if value < 0 || value > 10000 {
			warnValue(warnings, path, "notifications.history", value, "want 0..10000; using default")
			fallback(&settings.Notifications.History, defaults.Notifications.History)
		} else {
			settings.Notifications.History = value
		}
	}

	if loaded.Recovery.Enabled != nil {
		settings.Recovery.Enabled = *loaded.Recovery.Enabled
	}
	if loaded.Recovery.IntervalMS != nil {
		value := *loaded.Recovery.IntervalMS
		if value < 100 || value > 600000 {
			warnValue(warnings, path, "recovery.interval_ms", value, "want 100..600000; using default")
			fallback(&settings.Recovery.IntervalMS, defaults.Recovery.IntervalMS)
		} else {
			settings.Recovery.IntervalMS = value
		}
	}

	// Keys are merged key by key; an empty value unbinds. Unknown modes and
	// reserved or malformed keys are rejected with a warning.
	for mode, bindings := range loaded.Keys {
		applyKeys(settings, path, mode, bindings, warnings)
	}
}

// fallback restores defaults for one invalid scalar. Keeping the lower layer
// would make a typo in the workspace file silently inherit a global value
// instead of the documented default, so validation resets the field.
func fallback[T any](target *T, value T) {
	*target = value
}

func warnValue(warnings *[]Warning, path, key string, value any, message string) {
	*warnings = append(*warnings, Warning{Path: path, Key: key, Message: fmt.Sprintf("%v: %s", value, message)})
}

// CommandFor returns the command bound to key in mode. mode is one of normal,
// insert, visual, or command. key uses the canonical input.FormatKey spelling.
func (s Settings) CommandFor(mode, key string) (string, bool) {
	bindings := s.Keys[mode]
	if bindings == nil {
		return "", false
	}
	command, ok := bindings[key]
	return command, ok && command != ""
}

// KeyFor returns the first bound key for commandID in mode, in sorted order.
func (s Settings) KeyFor(mode, commandID string) string {
	bindings := s.Keys[mode]
	if bindings == nil {
		return ""
	}
	keys := make([]string, 0, len(bindings))
	for key, command := range bindings {
		if command == commandID {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

// AllBindings returns a copy of the bindings for mode.
func (s Settings) AllBindings(mode string) map[string]string {
	out := make(map[string]string, len(s.Keys[mode]))
	for key, command := range s.Keys[mode] {
		out[key] = command
	}
	return out
}

func validMode(mode string) bool {
	switch mode {
	case "normal", "insert", "visual", "command":
		return true
	default:
		return false
	}
}

// GlobalPath returns $XDG_CONFIG_HOME/pico-editor/config.toml. It returns an
// empty string when no home directory is available.
func GlobalPath() string {
	if configHome := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(configHome) {
		return filepath.Join(configHome, "pico-editor", "config.toml")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".config", "pico-editor", "config.toml")
	}
	return ""
}

// WorkspacePath returns <root>/.pico-editor.toml.
func WorkspacePath(root string) string {
	if root == "" {
		return ""
	}
	return filepath.Join(root, ".pico-editor.toml")
}

// StateDir returns $XDG_STATE_HOME/pico-editor, or the documented fallback.
func StateDir() string {
	if stateHome := os.Getenv("XDG_STATE_HOME"); filepath.IsAbs(stateHome) {
		return filepath.Join(stateHome, "pico-editor")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".local", "state", "pico-editor")
	}
	return filepath.Join(os.TempDir(), "pico-editor")
}

// RecoveryDir returns the crash-recovery journal directory.
func RecoveryDir() string {
	return filepath.Join(StateDir(), "recovery")
}

// LogPath returns the default rotating log file path.
func LogPath() string {
	return filepath.Join(StateDir(), "pico-editor.log")
}
