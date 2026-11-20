package input

// Kind identifies one input event.
type Kind uint8

const (
	// KindKeyDown is a physical key press or repeat.
	KindKeyDown Kind = iota
	// KindText carries committed OS text.
	KindText
	// KindFocus reports a focus transition.
	KindFocus
	// KindResize reports a logical canvas or window resize.
	KindResize
	// KindMouse reports a mouse state change.
	KindMouse
)

// Mods is a normalized modifier set.
type Mods uint8

const (
	ModShift Mods = 1 << iota
	ModCtrl
	ModAlt
	ModMeta
)

// String returns the canonical config prefix, for example "ctrl+shift".
func (m Mods) String() string {
	parts := make([]string, 0, 4)
	if m&ModCtrl != 0 {
		parts = append(parts, "ctrl")
	}
	if m&ModAlt != 0 {
		parts = append(parts, "alt")
	}
	if m&ModShift != 0 {
		parts = append(parts, "shift")
	}
	if m&ModMeta != 0 {
		parts = append(parts, "meta")
	}
	if len(parts) == 0 {
		return ""
	}
	return joinPlus(parts)
}

// MouseButtons is a normalized mouse-button state.
type MouseButtons uint8

const (
	MouseLeft MouseButtons = 1 << iota
	MouseRight
	MouseMiddle
	MouseButton4
	MouseButton5
)

// Event is one normalized input event. Key events and text events are emitted
// separately for the same physical key so consumers can choose which to use.
type Event struct {
	Kind    Kind
	Key     Key
	Mods    Mods
	Runes   []rune
	Repeat  bool
	Focused bool
	X       int
	Y       int
	Buttons MouseButtons
}

// Source produces normalized events once per frame.
type Source interface {
	Poll() []Event
}
