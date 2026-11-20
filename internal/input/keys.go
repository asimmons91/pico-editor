package input

import (
	"fmt"
	"strings"
)

// Key is a stable US-layout key identity. Modifiers are carried separately in
// Mods, so shifted letters keep the same Key.
type Key int

const (
	KeyUnknown Key = iota

	KeyA
	KeyB
	KeyC
	KeyD
	KeyE
	KeyF
	KeyG
	KeyH
	KeyI
	KeyJ
	KeyK
	KeyL
	KeyM
	KeyN
	KeyO
	KeyP
	KeyQ
	KeyR
	KeyS
	KeyT
	KeyU
	KeyV
	KeyW
	KeyX
	KeyY
	KeyZ

	Key0
	Key1
	Key2
	Key3
	Key4
	Key5
	Key6
	Key7
	Key8
	Key9

	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12

	KeyEsc
	KeyEnter
	KeySpace
	KeyTab
	KeyBackspace
	KeyDelete
	KeyInsert
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
	KeyUp
	KeyDown
	KeyLeft
	KeyRight

	KeyShift
	KeyControl
	KeyAlt
	KeyMeta

	KeyMinus
	KeyEqual
	KeyLeftBracket
	KeyRightBracket
	KeyBackslash
	KeySemicolon
	KeyApostrophe
	KeyComma
	KeyPeriod
	KeySlash
	KeyBacktick
)

type keyName struct {
	key  Key
	name string
}

var keyNames = []keyName{
	{KeyA, "a"}, {KeyB, "b"}, {KeyC, "c"}, {KeyD, "d"}, {KeyE, "e"},
	{KeyF, "f"}, {KeyG, "g"}, {KeyH, "h"}, {KeyI, "i"}, {KeyJ, "j"},
	{KeyK, "k"}, {KeyL, "l"}, {KeyM, "m"}, {KeyN, "n"}, {KeyO, "o"},
	{KeyP, "p"}, {KeyQ, "q"}, {KeyR, "r"}, {KeyS, "s"}, {KeyT, "t"},
	{KeyU, "u"}, {KeyV, "v"}, {KeyW, "w"}, {KeyX, "x"}, {KeyY, "y"},
	{KeyZ, "z"},

	{Key0, "0"}, {Key1, "1"}, {Key2, "2"}, {Key3, "3"}, {Key4, "4"},
	{Key5, "5"}, {Key6, "6"}, {Key7, "7"}, {Key8, "8"}, {Key9, "9"},

	{KeyF1, "f1"}, {KeyF2, "f2"}, {KeyF3, "f3"}, {KeyF4, "f4"},
	{KeyF5, "f5"}, {KeyF6, "f6"}, {KeyF7, "f7"}, {KeyF8, "f8"},
	{KeyF9, "f9"}, {KeyF10, "f10"}, {KeyF11, "f11"}, {KeyF12, "f12"},

	{KeyEsc, "esc"}, {KeyEnter, "enter"}, {KeySpace, "space"},
	{KeyTab, "tab"}, {KeyBackspace, "backspace"}, {KeyDelete, "delete"},
	{KeyInsert, "insert"}, {KeyHome, "home"}, {KeyEnd, "end"},
	{KeyPageUp, "pageup"}, {KeyPageDown, "pagedown"},
	{KeyUp, "up"}, {KeyDown, "down"}, {KeyLeft, "left"}, {KeyRight, "right"},

	{KeyShift, "shift"}, {KeyControl, "ctrl"}, {KeyAlt, "alt"}, {KeyMeta, "meta"},

	{KeyMinus, "minus"}, {KeyEqual, "equal"},
	{KeyLeftBracket, "leftbracket"}, {KeyRightBracket, "rightbracket"},
	{KeyBackslash, "backslash"}, {KeySemicolon, "semicolon"},
	{KeyApostrophe, "apostrophe"}, {KeyComma, "comma"},
	{KeyPeriod, "period"}, {KeySlash, "slash"}, {KeyBacktick, "backtick"},
}

var (
	namesByKey  = make(map[Key]string, len(keyNames))
	keysByName  = make(map[string]Key, len(keyNames)+4)
	keyNameList = make([]string, 0, len(keyNames))
)

func init() {
	for _, entry := range keyNames {
		namesByKey[entry.key] = entry.name
		keysByName[entry.name] = entry.key
		keyNameList = append(keyNameList, entry.name)
	}
	keysByName["escape"] = KeyEsc
	keysByName["return"] = KeyEnter
	keysByName["control"] = KeyControl
}

// KeyName returns the canonical config name for a key.
func KeyName(key Key) string {
	if name, ok := namesByKey[key]; ok {
		return name
	}
	return "unknown"
}

// FormatKey renders a key and modifiers in the canonical order used by
// config.toml: ctrl+alt+shift+meta.
func FormatKey(key Key, mods Mods) string {
	parts := make([]string, 0, 5)
	if mods&ModCtrl != 0 && key != KeyControl {
		parts = append(parts, "ctrl")
	}
	if mods&ModAlt != 0 && key != KeyAlt {
		parts = append(parts, "alt")
	}
	if mods&ModShift != 0 && key != KeyShift {
		parts = append(parts, "shift")
	}
	if mods&ModMeta != 0 && key != KeyMeta {
		parts = append(parts, "meta")
	}
	parts = append(parts, KeyName(key))
	return joinPlus(parts)
}

// ParseKey parses a config binding such as "ctrl+shift+p" or "f1".
func ParseKey(binding string) (Key, Mods, error) {
	text := strings.ToLower(strings.TrimSpace(binding))
	if text == "" {
		return KeyUnknown, 0, fmt.Errorf("empty key binding")
	}
	fields := strings.Split(text, "+")
	keyText := fields[len(fields)-1]
	if keyText == "" {
		return KeyUnknown, 0, fmt.Errorf("key binding %q has no key", binding)
	}

	var mods Mods
	seen := make(map[string]bool, len(fields)-1)
	for _, field := range fields[:len(fields)-1] {
		if seen[field] {
			return KeyUnknown, 0, fmt.Errorf("key binding %q repeats modifier %q", binding, field)
		}
		seen[field] = true
		switch field {
		case "ctrl":
			mods |= ModCtrl
		case "alt":
			mods |= ModAlt
		case "shift":
			mods |= ModShift
		case "meta":
			mods |= ModMeta
		default:
			return KeyUnknown, 0, fmt.Errorf("unknown modifier %q in %q", field, binding)
		}
	}

	key, ok := keysByName[keyText]
	if !ok {
		return KeyUnknown, 0, fmt.Errorf("unknown key %q in %q; accepted names: %s", keyText, binding, strings.Join(keyNameList, ", "))
	}
	return key, mods, nil
}

// IsModifier reports whether key is a modifier key.
func IsModifier(key Key) bool {
	switch key {
	case KeyShift, KeyControl, KeyAlt, KeyMeta:
		return true
	default:
		return false
	}
}

func isRepeatable(key Key) bool {
	return !IsModifier(key) && key != KeyEsc
}

func joinPlus(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	size := len(parts) - 1
	for _, part := range parts {
		size += len(part)
	}
	var out strings.Builder
	out.Grow(size)
	for i, part := range parts {
		if i != 0 {
			out.WriteByte('+')
		}
		out.WriteString(part)
	}
	return out.String()
}
