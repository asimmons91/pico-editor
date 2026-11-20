package input

import (
	"log/slog"
	"strings"
	"unicode"

	"github.com/asimmons91/pico-lsp/pkg/p8scii"
)

// P8Mode is the active insert-mode P8SCII input mode. PICO-8's modes are
// mutually exclusive; default means Shift+letter inserts the symbol glyphs and
// normal letters insert lowercase ASCII.
type P8Mode uint8

const (
	P8ModeDefault P8Mode = iota
	P8ModeHiragana
	P8ModeKatakana
	P8ModePuny
)

// P8Input owns P8SCII insert-mode state. It never touches a buffer; callers
// pass key and text events through it and apply the returned text.
type P8Input struct {
	Mode P8Mode
	// Pending is the in-progress romaji sequence for kana modes.
	Pending string
	// Log receives one debug record when a layout cannot produce a US letter.
	Log *slog.Logger

	pendingGlyph Key
	pendingDigit Key
	logged       map[Key]bool
}

// NewP8Input returns a default-mode input state.
func NewP8Input(log *slog.Logger) *P8Input {
	return &P8Input{Log: log}
}

// ModeLabel returns the compact status-bar label for the active mode.
func (p *P8Input) ModeLabel() string {
	switch p.Mode {
	case P8ModeHiragana:
		return "HIRA"
	case P8ModeKatakana:
		return "KATA"
	case P8ModePuny:
		return "PUNY"
	default:
		return ""
	}
}

// Reset clears mode and pending state, for example when insert mode ends.
func (p *P8Input) Reset() {
	p.Mode = P8ModeDefault
	p.Pending = ""
	p.pendingGlyph = KeyUnknown
	p.pendingDigit = KeyUnknown
}

// HandleKey consumes P8SCII mode toggles and records the physical key needed
// to resolve the following text event. It returns true when the key must not
// reach Vim.
func (p *P8Input) HandleKey(key Key, mods Mods) bool {
	return p.handleKey(key, mods, false)
}

// HandleRepeat consumes an auto-repeated key without toggling a mode.
func (p *P8Input) HandleRepeat(key Key, mods Mods) bool {
	return p.handleKey(key, mods, true)
}

func (p *P8Input) handleKey(key Key, mods Mods, repeat bool) bool {
	// A new key always cancels a deferred text conversion that never arrived.
	p.pendingGlyph = KeyUnknown
	p.pendingDigit = KeyUnknown

	if mods&ModCtrl != 0 && mods&(ModAlt|ModMeta|ModShift) == 0 {
		switch key {
		case KeyJ, KeyK, KeyP:
			if repeat {
				return true
			}
			switch key {
			case KeyJ:
				p.toggle(P8ModeHiragana)
			case KeyK:
				p.toggle(P8ModeKatakana)
			case KeyP:
				p.toggle(P8ModePuny)
			}
			return true
		}
	}

	if mods == ModShift && key >= KeyA && key <= KeyZ {
		if p.Mode != P8ModePuny {
			p.Pending = ""
			p.pendingGlyph = key
			// Let Vim see the key (insert-mode key handling has no text
			// effect); the paired text event carries the layout result.
			return false
		}
		p.Pending = ""
		return false
	}

	if (p.Mode == P8ModeHiragana || p.Mode == P8ModeKatakana) &&
		mods == ModShift && key >= Key0 && key <= Key9 {
		p.Pending = ""
		p.pendingDigit = key
		return false
	}

	if mods&(ModCtrl|ModAlt|ModMeta) != 0 {
		p.Pending = ""
		return false
	}

	// Non-letter keys flush a pending romaji sequence; letter keydowns keep it
	// so the following text event can complete it.
	if !isLetterKey(key) {
		p.Pending = ""
	}
	return false
}

func (p *P8Input) toggle(mode P8Mode) {
	p.Pending = ""
	if p.Mode == mode {
		p.Mode = P8ModeDefault
		return
	}
	p.Mode = mode
}

// TransformText rewrites one text event for the active mode. handled is true
// when the event was consumed and must not reach Vim.
func (p *P8Input) TransformText(runes []rune, mods Mods) (out []rune, handled bool) {
	if len(runes) == 0 {
		return runes, false
	}

	if p.pendingGlyph != KeyUnknown {
		key := p.pendingGlyph
		p.pendingGlyph = KeyUnknown
		expected := rune('A' + int(key-KeyA))
		if len(runes) == 1 && runes[0] == expected {
			return []rune(p8scii.Decode([]byte{byte(128 + int(key-KeyA))})), false
		}
		p.logLayoutFallback(key, runes)
		return runes, false
	}

	if p.pendingDigit != KeyUnknown {
		key := p.pendingDigit
		p.pendingDigit = KeyUnknown
		if glyph := p.kanaDigitGlyph(key); glyph != "" {
			out = append([]rune(glyph), runes[1:]...)
			return out, false
		}
	}

	switch p.Mode {
	case P8ModePuny:
		return punyText(runes, mods), false
	case P8ModeHiragana, P8ModeKatakana:
		return p.kanaText(runes, mods)
	default:
		return runes, false
	}
}

func punyText(runes []rune, mods Mods) []rune {
	out := append([]rune(nil), runes...)
	shift := mods&ModShift != 0
	for i, r := range out {
		switch {
		case r >= 'a' && r <= 'z':
			if !shift {
				out[i] = unicode.ToUpper(r)
			}
		case r >= 'A' && r <= 'Z':
			if shift {
				out[i] = unicode.ToLower(r)
			}
		}
	}
	return out
}

func (p *P8Input) kanaText(runes []rune, mods Mods) ([]rune, bool) {
	if mods&ModShift != 0 {
		p.Pending = ""
		return runes, false
	}
	out := make([]rune, 0, len(runes))
	consumed := false
	for _, r := range runes {
		if r >= 'a' && r <= 'z' {
			consumed = true
			out = append(out, []rune(p.feedKana(r))...)
			continue
		}
		p.Pending = ""
		out = append(out, r)
	}
	if consumed && len(out) == 0 {
		return nil, true
	}
	return out, false
}

func (p *P8Input) feedKana(r rune) string {
	if p.Pending == "" {
		if text, ok := kanaTable[string(r)]; ok {
			return p.kanaModeText(text)
		}
		if r == 'n' || kanaPrefixes[string(r)] {
			p.Pending = string(r)
			return ""
		}
		return string(r)
	}

	// Doubled consonants geminate, except n which has its own nn sequence.
	if len(p.Pending) == 1 && p.Pending[0] == byte(r) && r != 'n' && isKanaConsonant(r) {
		p.Pending = string(r)
		return p8scii.Decode([]byte{0xc8})
	}

	candidate := p.Pending + string(r)
	if text, ok := kanaTable[candidate]; ok {
		p.Pending = ""
		return p.kanaModeText(text)
	}
	if kanaPrefixes[candidate] {
		p.Pending = candidate
		return ""
	}
	p.Pending = ""
	return ""
}

func (p *P8Input) kanaModeText(text string) string {
	if p.Mode != P8ModeKatakana {
		return text
	}
	bytes := p8scii.Encode(text)
	for i, b := range bytes {
		if b >= 0x9a && b <= 0xcb {
			bytes[i] = b + 0x32
		}
	}
	return p8scii.Decode(bytes)
}

func (p *P8Input) kanaDigitGlyph(key Key) string {
	digit := int(key - Key0)
	if p.Mode == P8ModeHiragana {
		switch digit {
		case 0:
			return p8scii.Decode([]byte{0xff})
		case 9:
			return p8scii.Decode([]byte{0xfe})
		default:
			return p8scii.Decode([]byte{0x10 + byte(digit-1)})
		}
	}
	if p.Mode == P8ModeKatakana {
		if digit == 0 {
			return p8scii.Decode([]byte{0x19})
		}
		return p8scii.Decode([]byte{0x10 + byte(digit-1)})
	}
	return ""
}

func (p *P8Input) logLayoutFallback(key Key, runes []rune) {
	if p.Log == nil {
		return
	}
	if p.logged == nil {
		p.logged = make(map[Key]bool)
	}
	if p.logged[key] {
		return
	}
	p.logged[key] = true
	p.Log.Debug("p8scii: layout did not produce a US letter",
		"key", KeyName(key), "text", string(runes))
}

func isLetterKey(key Key) bool {
	return key >= KeyA && key <= KeyZ
}

func isKanaConsonant(r rune) bool {
	return strings.ContainsRune("kstnhmyrwgzdbpcf", r)
}

// kanaTable maps romaji to canonical Hiragana text. Katakana is derived by
// shifting the Hiragana P8SCII bytes by 0x32.
//
// Capture: PICO-8 0.2.7, testdata/pico8/input/manifest.json
// sha256 241f2d910dd2eef26e1f80e019c9efb3dcb770a30da25d9530ea55dfa5f097b3
var kanaTable, kanaPrefixes = buildKanaTable()

func buildKanaTable() (map[string]string, map[string]bool) {
	table := make(map[string]string, 160)
	add := func(key string, bytes ...byte) {
		table[key] = p8scii.Decode(bytes)
	}
	add("a", 0x9a)
	add("i", 0x9b)
	add("u", 0x9c)
	add("e", 0x9d)
	add("o", 0x9e)

	// base is the hiragana a-row byte; vowels follow consecutively.
	rows := []kanaRow{
		{key: "k", base: 0x9f, smallKey: "k"},
		{key: "s", base: 0xa4, smallKey: "s"},
		// PICO-8 spells the t-row digraphs cha/chu/cho; tya/tyu/tyo are not
		// valid sequences.
		{key: "t", base: 0xa9},
		{key: "n", base: 0xae, smallKey: "n"},
		{key: "h", base: 0xb3, smallKey: "h"},
		{key: "m", base: 0xb8, smallKey: "m"},
		{key: "r", base: 0xc0, smallKey: "r"},
		{key: "g", base: 0x9f, dakuten: 0x1e, smallKey: "k"},
		{key: "z", base: 0xa4, dakuten: 0x1e, smallKey: "s"},
		{key: "d", base: 0xa9, dakuten: 0x1e, smallKey: "t"},
		{key: "b", base: 0xb3, dakuten: 0x1e, smallKey: "h"},
		{key: "p", base: 0xb3, handaku: 0x1f, smallKey: "h"},
	}
	for _, r := range rows {
		for i, vowel := range []byte{'a', 'i', 'u', 'e', 'o'} {
			b := r.base + byte(i)
			switch {
			case r.dakuten != 0:
				add(r.key+string(vowel), b, r.dakuten)
			case r.handaku != 0:
				add(r.key+string(vowel), b, r.handaku)
			default:
				add(r.key+string(vowel), b)
			}
		}
		if r.smallKey == "" {
			continue
		}
		base := r.smallKeyBase()
		if base == 0 {
			continue
		}
		for i, vowel := range []string{"ya", "yu", "yo"} {
			b := base + byte(i)
			switch {
			case r.dakuten != 0:
				add(r.key+vowel, b, r.dakuten, 0xc9+byte(i))
			case r.handaku != 0:
				add(r.key+vowel, b, r.handaku, 0xc9+byte(i))
			default:
				add(r.key+vowel, b, 0xc9+byte(i))
			}
		}
	}

	// y and w rows are not five consecutive vowels.
	add("ya", 0xbd)
	add("yu", 0xbe)
	add("yo", 0xbf)
	add("wa", 0xc5)
	add("wo", 0xc6)

	// Hepburn/Kunrei aliases.
	add("si", 0xa5)
	add("shi", 0xa5)
	add("ti", 0xaa)
	add("chi", 0xaa)
	add("tu", 0xab)
	add("tsu", 0xab)
	add("hu", 0xb5)
	add("fu", 0xb5)
	add("zi", 0xa5, 0x1e)
	add("ji", 0xa5, 0x1e)
	add("di", 0xaa, 0x1e)
	add("du", 0xab, 0x1e)
	add("sya", 0xa5, 0xc9)
	add("syu", 0xa5, 0xca)
	add("syo", 0xa5, 0xcb)
	add("sha", 0xa5, 0xc9)
	add("shu", 0xa5, 0xca)
	add("sho", 0xa5, 0xcb)
	add("zya", 0xa5, 0x1e, 0xc9)
	add("zyu", 0xa5, 0x1e, 0xca)
	add("zyo", 0xa5, 0x1e, 0xcb)
	add("ja", 0xa5, 0x1e, 0xc9)
	add("ju", 0xa5, 0x1e, 0xca)
	add("jo", 0xa5, 0x1e, 0xcb)
	add("cha", 0xaa, 0xc9)
	add("chu", 0xaa, 0xca)
	add("cho", 0xaa, 0xcb)
	add("ca", 0x9f)
	add("cu", 0xa1)
	add("co", 0xa3)
	add("nn", 0xc7)
	add("ltsu", 0xc8)
	add("xtsu", 0xc8)
	add("lya", 0xc9)
	add("lyu", 0xca)
	add("lyo", 0xcb)
	add("xya", 0xc9)
	add("xyu", 0xca)
	add("xyo", 0xcb)

	prefixes := make(map[string]bool, len(table))
	for key := range table {
		for i := 1; i < len(key); i++ {
			prefixes[key[:i]] = true
		}
	}
	return table, prefixes
}

type kanaRow struct {
	key      string
	base     byte
	dakuten  byte
	handaku  byte
	smallKey string
}

func (r kanaRow) smallKeyBase() byte {
	switch r.smallKey {
	case "s":
		return 0xa5
	case "n":
		return 0xaf
	case "h":
		return 0xb4
	case "k":
		return 0xa0
	case "t":
		return 0xaa
	case "m":
		return 0xb9
	case "r":
		return 0xc1
	default:
		return 0
	}
}
