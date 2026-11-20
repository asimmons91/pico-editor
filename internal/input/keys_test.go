package input

import (
	"strings"
	"testing"
)

func TestKeyNameRoundTrip(t *testing.T) {
	for _, entry := range keyNames {
		if got := KeyName(entry.key); got != entry.name {
			t.Errorf("KeyName(%d) = %q, want %q", entry.key, got, entry.name)
		}
		key, mods, err := ParseKey(entry.name)
		if err != nil {
			t.Errorf("ParseKey(%q) error = %v", entry.name, err)
			continue
		}
		if key != entry.key || mods != 0 {
			t.Errorf("ParseKey(%q) = (%d, %d), want (%d, 0)", entry.name, key, mods, entry.key)
		}
		if got := FormatKey(entry.key, 0); got != entry.name {
			t.Errorf("FormatKey(%d, 0) = %q, want %q", entry.key, got, entry.name)
		}
	}
}

func TestParseKeyModifiers(t *testing.T) {
	cases := []struct {
		binding string
		key     Key
		mods    Mods
		format  string
	}{
		{"q", KeyQ, 0, "q"},
		{"ctrl+q", KeyQ, ModCtrl, "ctrl+q"},
		{"shift+p", KeyP, ModShift, "shift+p"},
		{"ctrl+shift+p", KeyP, ModCtrl | ModShift, "ctrl+shift+p"},
		{"meta+alt+shift+ctrl+left", KeyLeft, ModMeta | ModAlt | ModShift | ModCtrl, "ctrl+alt+shift+meta+left"},
		{"escape", KeyEsc, 0, "esc"},
		{"return", KeyEnter, 0, "enter"},
		{"control", KeyControl, 0, "ctrl"},
	}
	for _, tc := range cases {
		key, mods, err := ParseKey(tc.binding)
		if err != nil {
			t.Errorf("ParseKey(%q) error = %v", tc.binding, err)
			continue
		}
		if key != tc.key || mods != tc.mods {
			t.Errorf("ParseKey(%q) = (%d, %d), want (%d, %d)", tc.binding, key, mods, tc.key, tc.mods)
		}
		if got := FormatKey(key, mods); got != tc.format {
			t.Errorf("FormatKey(ParseKey(%q)) = %q, want %q", tc.binding, got, tc.format)
		}
	}
}

func TestFormatKeyOmitsOwnModifier(t *testing.T) {
	if got := FormatKey(KeyShift, ModShift); got != "shift" {
		t.Errorf("FormatKey(shift, shift) = %q, want shift", got)
	}
	if got := FormatKey(KeyControl, ModCtrl); got != "ctrl" {
		t.Errorf("FormatKey(ctrl, ctrl) = %q, want ctrl", got)
	}
}

func TestParseKeyErrors(t *testing.T) {
	cases := []struct {
		binding string
		want    string
	}{
		{"", "empty key binding"},
		{"ctrl+", "has no key"},
		{"hyper+a", "unknown modifier"},
		{"ctrl+ctrl+a", "repeats modifier"},
		{"ctrl+notakey", "unknown key"},
	}
	for _, tc := range cases {
		_, _, err := ParseKey(tc.binding)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("ParseKey(%q) error = %v, want it to contain %q", tc.binding, err, tc.want)
		}
	}
	_, _, err := ParseKey("nope")
	if err == nil || !strings.Contains(err.Error(), "accepted names") {
		t.Errorf("ParseKey(unknown) error = %v, want the accepted names", err)
	}
}

func TestModsStringAndIsModifier(t *testing.T) {
	if got := (ModCtrl | ModAlt | ModShift | ModMeta).String(); got != "ctrl+alt+shift+meta" {
		t.Errorf("Mods.String() = %q", got)
	}
	if got := Mods(0).String(); got != "" {
		t.Errorf("zero Mods.String() = %q, want empty", got)
	}
	for _, key := range []Key{KeyShift, KeyControl, KeyAlt, KeyMeta} {
		if !IsModifier(key) {
			t.Errorf("IsModifier(%v) = false, want true", key)
		}
	}
	if IsModifier(KeyA) {
		t.Error("IsModifier(a) = true, want false")
	}
}
