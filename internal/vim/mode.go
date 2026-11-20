// Package vim implements the curated Vim core defined by docs/VIM.md. It owns
// modal state, registers, search, ex commands, and dot repeat, but never owns a
// buffer: every edit goes through internal/editor.
package vim

// Mode is one Vim editing mode.
type Mode uint8

const (
	ModeNormal Mode = iota
	ModeInsert
	ModeVisualChar
	ModeVisualLine
	ModeVisualBlock
	ModeCommandLine
)

// String returns the explicit mode indicator from the M3 spec.
func (m Mode) String() string {
	switch m {
	case ModeInsert:
		return "INSERT"
	case ModeVisualChar:
		return "VISUAL"
	case ModeVisualLine:
		return "V-LINE"
	case ModeVisualBlock:
		return "V-BLOCK"
	case ModeCommandLine:
		return "COMMAND"
	default:
		return "NORMAL"
	}
}

// Visual reports whether m is one of the selection modes.
func (m Mode) Visual() bool {
	return m == ModeVisualChar || m == ModeVisualLine || m == ModeVisualBlock
}

// Operator is a pending Vim operator.
type Operator uint8

const (
	OpNone Operator = iota
	OpDelete
	OpChange
	OpYank
	OpShiftRight
	OpShiftLeft
)

func (o Operator) String() string {
	switch o {
	case OpDelete:
		return "delete"
	case OpChange:
		return "change"
	case OpYank:
		return "yank"
	case OpShiftRight:
		return "shift-right"
	case OpShiftLeft:
		return "shift-left"
	default:
		return "none"
	}
}
