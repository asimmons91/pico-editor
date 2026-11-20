package lsp

import (
	"go.lsp.dev/protocol"
)

// Settings are the pico8 initialization options the editor sends. They mirror
// pico-lsp's public JSON settings; M7 replaces the hard-coded defaults with
// TOML configuration.
type Settings struct {
	TypeChecking string
	InlayHints   bool
	MaxProblems  int
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
	return map[string]any{
		"typeChecking": s.TypeChecking,
		"inlayHints":   s.InlayHints,
		"maxProblems":  s.MaxProblems,
	}
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
