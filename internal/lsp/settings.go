package lsp

import (
	"go.lsp.dev/protocol"

	"github.com/asimmons91/pico-lsp/pkg/glyphs"
)

// Settings are the pico8 initialization options the editor sends. They mirror
// pico-lsp's public JSON settings. M7 loads custom glyphs from TOML; type
// checking, inlay hints, and max problems keep the documented defaults in V1.
type Settings struct {
	TypeChecking string
	InlayHints   bool
	MaxProblems  int
	// Glyphs carries custom picker entries from settings. M7 loads them from
	// TOML; the server owns validation, aliases, and insertion text.
	Glyphs []glyphs.Definition
}

// DefaultSettings matches pico-lsp's current defaults.
func DefaultSettings() Settings {
	return Settings{
		TypeChecking: "basic",
		InlayHints:   false,
		MaxProblems:  100,
	}
}

func (s Settings) normalize() Settings {
	defaults := DefaultSettings()
	switch s.TypeChecking {
	case "off", "basic", "strict":
	default:
		s.TypeChecking = defaults.TypeChecking
	}
	if s.MaxProblems < 0 {
		s.MaxProblems = defaults.MaxProblems
	}
	return s
}

func (s Settings) values() map[string]any {
	s = s.normalize()
	values := map[string]any{
		"typeChecking": s.TypeChecking,
		"inlayHints":   s.InlayHints,
		"maxProblems":  s.MaxProblems,
	}
	if len(s.Glyphs) > 0 {
		values["glyphs"] = s.Glyphs
	}
	return values
}

func (s Settings) initializationOptions() protocol.LSPAny {
	data, err := protocol.Marshal(map[string]any{"pico8": s.values()})
	if err != nil {
		return nil
	}
	return protocol.LSPAny(data)
}

func (s Settings) configuration(sections []string) []protocol.LSPAny {
	out := make([]protocol.LSPAny, len(sections))
	if len(sections) == 0 {
		return out
	}
	data, err := protocol.Marshal(s.values())
	if err != nil {
		return out
	}
	for index := range out {
		out[index] = protocol.LSPAny(data)
	}
	return out
}
