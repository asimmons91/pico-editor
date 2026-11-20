package vim

import (
	"strings"

	"github.com/asimmons91/pico-editor/internal/editor"
)

type registerKind uint8

const (
	registerChar registerKind = iota
	registerLine
	registerBlock
)

type registerValue struct {
	Text string
	Kind registerKind
}

type registerStore struct {
	unnamed  registerValue
	zero     registerValue
	numbered [9]registerValue
	named    map[rune]registerValue
	clip     map[byte]registerValue
}

func newRegisterStore() registerStore {
	return registerStore{
		named: make(map[rune]registerValue),
		clip:  make(map[byte]registerValue),
	}
}

// get resolves a register selector. A zero selector is the unnamed register.
func (r *registerStore) get(selector byte) (registerValue, bool) {
	switch {
	case selector == 0:
		return r.unnamed, r.unnamed.Text != ""
	case selector == '0':
		return r.zero, r.zero.Text != ""
	case selector >= '1' && selector <= '9':
		value := r.numbered[selector-'1']
		return value, value.Text != ""
	case selector >= 'a' && selector <= 'z':
		value, ok := r.named[rune(selector)]
		return value, ok && value.Text != ""
	case selector >= 'A' && selector <= 'Z':
		value, ok := r.named[rune(selector-'A'+'a')]
		return value, ok && value.Text != ""
	case selector == '+' || selector == '*':
		value, ok := r.clip[selector]
		return value, ok
	case selector == '_':
		return registerValue{}, false
	default:
		return registerValue{}, false
	}
}

// store writes a value. Yanks update the unnamed and "0 registers; deletes
// update the unnamed register and rotate the numbered delete history.
func (r *registerStore) store(text string, kind registerKind, selector byte, yank bool) {
	if selector == '_' {
		return
	}
	if r.named == nil {
		r.named = make(map[rune]registerValue)
	}
	if r.clip == nil {
		r.clip = make(map[byte]registerValue)
	}
	value := registerValue{Text: text, Kind: kind}

	if selector == '+' || selector == '*' {
		r.clip[selector] = value
		r.unnamed = value
		if yank {
			r.zero = value
		}
		return
	}

	if selector >= 'A' && selector <= 'Z' {
		lower := rune(selector - 'A' + 'a')
		existing := r.named[lower]
		if existing.Kind == kind || existing.Text == "" {
			existing.Text += text
			existing.Kind = kind
		} else {
			existing = value
		}
		r.named[lower] = existing
		r.unnamed = existing
		if yank {
			r.zero = existing
		}
		return
	}

	switch {
	case selector == '0':
		r.zero = value
	case selector >= '1' && selector <= '9':
		r.numbered[selector-'1'] = value
	case selector >= 'a' && selector <= 'z':
		r.named[rune(selector)] = value
	}

	r.unnamed = value
	if yank {
		r.zero = value
	} else {
		r.rotate(value)
	}
}

func (r *registerStore) rotate(value registerValue) {
	for i := len(r.numbered) - 1; i > 0; i-- {
		r.numbered[i] = r.numbered[i-1]
	}
	r.numbered[0] = value
}

// clipboardRegister returns the cached system clipboard value.
func (r *registerStore) clipboardRegister(selector byte) (registerValue, bool) {
	value, ok := r.clip[selector]
	return value, ok
}

func (v *Vim) recordRegister(text string, kind registerKind, selector byte, yank bool) {
	v.registers.store(text, kind, selector, yank)
	if selector == '+' || selector == '*' {
		v.queue(Action{Kind: ActionClipboardWrite, Register: selector, Text: text})
	}
}

func (v *Vim) selectedRegister() byte {
	selector := v.Register
	v.Register = 0
	return selector
}

// registerTextForPaste loads the register, requesting an asynchronous
// clipboard read when a system register has no cached value.
func (v *Vim) registerTextForPaste(selector byte) (registerValue, bool, bool) {
	if selector == '+' || selector == '*' {
		// Always read the system clipboard on paste. A previous yank in this
		// session is only a fallback when the host clipboard is unavailable.
		v.lastClipboardRegister = selector
		v.queue(Action{Kind: ActionClipboardRead, Register: selector})
		return registerValue{}, false, true
	}
	value, ok := v.registers.get(selector)
	return value, ok, false
}

// CompleteClipboardRead applies the result of an asynchronous clipboard read.
// It returns actions produced by a pending paste.
func (v *Vim) CompleteClipboardRead(text string, err error, ed *editor.Editor) Result {
	v.actions = nil
	v.notify = ""
	v.notifyError = false
	v.bell = false
	request := v.pendingPaste
	v.pendingPaste = nil
	if err != nil {
		v.message("clipboard unavailable")
		if value, ok := v.registers.clipboardRegister(v.lastClipboardRegister); ok {
			if request != nil && request.visual {
				v.pasteOverSelection(ed, value)
			} else {
				v.pasteValue(ed, value, request)
			}
		}
		return v.finish()
	}
	selector := v.lastClipboardRegister
	if selector == 0 {
		selector = '+'
	}
	v.registers.clip[selector] = registerValue{Text: text, Kind: registerTextKind(text)}
	if request != nil {
		if request.visual {
			v.pasteOverSelection(ed, v.registers.clip[selector])
		} else {
			v.pasteValue(ed, v.registers.clip[selector], request)
		}
	}
	v.clampNormal(ed)
	return v.finish()
}

func registerTextKind(text string) registerKind {
	if strings.HasSuffix(text, "\n") {
		return registerLine
	}
	return registerChar
}
