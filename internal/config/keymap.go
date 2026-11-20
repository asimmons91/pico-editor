package config

import (
	"fmt"
	"sort"

	"github.com/asimmons91/pico-editor/internal/input"
)

// DefaultKeys returns the documented M7 defaults. Users may rebind or unbind
// these; Vim grammar keys are validated separately and cannot be replaced.
func DefaultKeys() map[string]map[string]string {
	return map[string]map[string]string{
		"normal": {
			"ctrl+s":         "file.save",
			"ctrl+shift+b":   "tree.toggle",
			"ctrl+shift+p":   "palette.show",
			"ctrl+tab":       "cart.tab.next",
			"ctrl+shift+tab": "cart.tab.prev",
			"ctrl+shift+g":   "glyph.pick",
			"ctrl+q":         "app.quit",
			"f1":             "help.open",
			"ctrl+shift+m":   "messages.show",
		},
		"insert": {
			"ctrl+s":       "file.save",
			"ctrl+shift+p": "palette.show",
			"ctrl+shift+g": "glyph.pick",
			"ctrl+q":       "app.quit",
			"f1":           "help.open",
		},
		"visual": {
			"ctrl+s":       "file.save",
			"ctrl+shift+g": "glyph.pick",
			"f1":           "help.open",
		},
		"command": {
			"f1": "help.open",
		},
	}
}

// reservedKeys lists bindings the frozen Vim matrix and the M5/M6 input layers
// own. They cannot be reassigned to another command.
var reservedKeys = map[string]map[string]bool{
	"normal": {
		"ctrl+v": true, "ctrl+r": true, "ctrl+o": true, "ctrl+i": true,
		"ctrl+rightbracket": true, "ctrl+d": true, "ctrl+u": true,
		"ctrl+f": true, "ctrl+b": true, "ctrl+leftbracket": true,
		"ctrl+n": true, "ctrl+shift+n": true, "ctrl+space": true,
		"ctrl+shift+v": true, "ctrl+shift+c": true,
	},
	"insert": {
		"ctrl+w": true, "ctrl+u": true, "ctrl+j": true, "ctrl+k": true,
		"ctrl+p": true, "ctrl+space": true, "ctrl+n": true,
		"ctrl+shift+n": true, "ctrl+leftbracket": true,
		"ctrl+f": true, "ctrl+b": true,
		"ctrl+shift+v": true, "ctrl+shift+c": true,
		"ctrl+tab": true, "ctrl+shift+tab": true,
	},
	"visual": {
		"ctrl+v": true, "ctrl+leftbracket": true,
		"ctrl+f": true, "ctrl+b": true, "ctrl+n": true,
		"ctrl+shift+n": true, "ctrl+space": true,
		"ctrl+shift+v": true, "ctrl+shift+c": true,
	},
	"command": {
		"ctrl+u": true, "ctrl+w": true, "ctrl+leftbracket": true,
		"ctrl+f": true, "ctrl+b": true, "ctrl+n": true,
		"ctrl+shift+n": true, "ctrl+space": true,
		"ctrl+shift+v": true, "ctrl+shift+c": true,
	},
}

// applyKeys merges one user key table into the current bindings. Invalid keys,
// reserved grammar keys, and modifier-only bindings are rejected with a
// warning; the previous binding (usually the default) stays in place.
func applyKeys(settings *Settings, path, mode string, bindings map[string]string, warnings *[]Warning) {
	if !validMode(mode) {
		*warnings = append(*warnings, Warning{Path: path, Key: "keys." + mode, Message: "unknown mode ignored"})
		return
	}
	if settings.Keys[mode] == nil {
		settings.Keys[mode] = make(map[string]string)
	}
	keys := make([]string, 0, len(bindings))
	for key := range bindings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		command := bindings[key]
		parsedKey, mods, err := input.ParseKey(key)
		if err != nil {
			*warnings = append(*warnings, Warning{Path: path, Key: "keys." + mode + "." + key, Message: err.Error()})
			continue
		}
		canonical := input.FormatKey(parsedKey, mods)
		if input.IsModifier(parsedKey) {
			*warnings = append(*warnings, Warning{Path: path, Key: "keys." + mode + "." + key, Message: "a modifier key needs a non-modifier key"})
			continue
		}
		if command != "" && !validCommandSpelling(command) {
			*warnings = append(*warnings, Warning{Path: path, Key: "keys." + mode + "." + key, Message: fmt.Sprintf("invalid command ID %q", command)})
			continue
		}
		if reservedKeys[mode][canonical] {
			*warnings = append(*warnings, Warning{
				Path:    path,
				Key:     "keys." + mode + "." + key,
				Message: "reserved by the Vim/input/language bindings; keeping the previous binding",
			})
			continue
		}
		if !allowedGlobalKey(parsedKey, mods) {
			*warnings = append(*warnings, Warning{
				Path:    path,
				Key:     "keys." + mode + "." + key,
				Message: "Vim grammar keys are fixed; use a ctrl/alt/meta binding or f1..f12",
			})
			continue
		}
		if command == "" {
			delete(settings.Keys[mode], canonical)
			if canonical != key {
				delete(settings.Keys[mode], key)
			}
			continue
		}
		if previous, ok := settings.Keys[mode][canonical]; ok && previous != command {
			*warnings = append(*warnings, Warning{
				Path:    path,
				Key:     "keys." + mode + "." + key,
				Message: fmt.Sprintf("replaces the previous %s binding with %s", previous, command),
			})
		}
		settings.Keys[mode][canonical] = command
	}
}

// allowedGlobalKey enforces the V1 rule that only global/editor shortcuts are
// remappable. Vim grammar keys and insert-mode text keys stay fixed.
func allowedGlobalKey(key input.Key, mods input.Mods) bool {
	if mods&(input.ModCtrl|input.ModAlt|input.ModMeta) != 0 {
		return true
	}
	return key >= input.KeyF1 && key <= input.KeyF12
}

// validCommandSpelling accepts the stable dotted command IDs used by the
// palette, for example "file.save" or "cart.tab.next".
func validCommandSpelling(command string) bool {
	if command == "" || command[0] == '.' || command[len(command)-1] == '.' {
		return false
	}
	lastDot := false
	for _, r := range command {
		switch {
		case r == '.':
			if lastDot {
				return false
			}
			lastDot = true
		case r >= 'a' && r <= 'z':
			lastDot = false
		case r >= '0' && r <= '9':
			lastDot = false
		default:
			return false
		}
	}
	return true
}

// KeymapMode converts a Vim mode name to its keymap section.
func KeymapMode(mode string) string {
	switch mode {
	case "insert":
		return "insert"
	case "visual-char", "visual-line", "visual-block":
		return "visual"
	case "command":
		return "command"
	default:
		return "normal"
	}
}
