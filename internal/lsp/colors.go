package lsp

// Semantic token colors use the final M7 table so M5 and M7 share one mapping.
// Values are PICO-8 palette indices; internal/lsp does not import internal/render.
const (
	ColorDefault    uint8 = 7
	ColorKeyword    uint8 = 14
	ColorString     uint8 = 11
	ColorNumber     uint8 = 10
	ColorComment    uint8 = 13
	ColorOperator   uint8 = 6
	ColorVariable   uint8 = 7
	ColorParameter  uint8 = 15
	ColorProperty   uint8 = 15
	ColorFunction   uint8 = 12
	ColorType       uint8 = 9
	ColorNamespace  uint8 = 14
	ColorMacro      uint8 = 9
	ColorRegexp     uint8 = 11
	ColorDecorator  uint8 = 9
	ColorModifier   uint8 = 13
	ColorReadonly   uint8 = 6
	ColorDeprecated uint8 = 8
	ColorLibrary    uint8 = 13
)

// Token priority is fixed by M7. Higher values win when tokens overlap.
const (
	PriorityDefault = iota
	PriorityNamespace
	PriorityParameter
	PriorityOperator
	PriorityVariable
	PriorityProperty
	PriorityFunction
	PriorityType
	PriorityKeyword
	PriorityNumber
	PriorityString
	PriorityComment
)

// TokenStyle is the palette color and overlap priority for a token.
type TokenStyle struct {
	Color      uint8
	Priority   int
	Deprecated bool
}

// StyleFor maps a semantic token type and modifiers to a palette style.
func StyleFor(tokenType string, modifiers []string) TokenStyle {
	style := baseStyle(tokenType)
	for _, modifier := range modifiers {
		switch modifier {
		case "readonly", "static":
			style.Color = ColorReadonly
		case "deprecated":
			style.Color = ColorDeprecated
			style.Deprecated = true
		case "defaultLibrary":
			style.Color = ColorLibrary
		}
	}
	return style
}

func baseStyle(tokenType string) TokenStyle {
	switch tokenType {
	case "comment":
		return TokenStyle{Color: ColorComment, Priority: PriorityComment}
	case "string":
		return TokenStyle{Color: ColorString, Priority: PriorityString}
	case "number":
		return TokenStyle{Color: ColorNumber, Priority: PriorityNumber}
	case "keyword":
		return TokenStyle{Color: ColorKeyword, Priority: PriorityKeyword}
	case "type", "class", "enum", "interface", "struct", "typeParameter":
		return TokenStyle{Color: ColorType, Priority: PriorityType}
	case "function", "method":
		return TokenStyle{Color: ColorFunction, Priority: PriorityFunction}
	case "property", "enumMember", "event":
		return TokenStyle{Color: ColorProperty, Priority: PriorityProperty}
	case "variable":
		return TokenStyle{Color: ColorVariable, Priority: PriorityVariable}
	case "operator":
		return TokenStyle{Color: ColorOperator, Priority: PriorityOperator}
	case "parameter":
		return TokenStyle{Color: ColorParameter, Priority: PriorityParameter}
	case "namespace":
		return TokenStyle{Color: ColorNamespace, Priority: PriorityNamespace}
	case "macro":
		return TokenStyle{Color: ColorMacro, Priority: PriorityType}
	case "regexp":
		return TokenStyle{Color: ColorRegexp, Priority: PriorityString}
	case "decorator":
		return TokenStyle{Color: ColorDecorator, Priority: PriorityType}
	case "modifier":
		return TokenStyle{Color: ColorModifier, Priority: PriorityComment}
	default:
		return TokenStyle{Color: ColorDefault, Priority: PriorityDefault}
	}
}

// ColorAt chooses the highest-priority token covering runeOffset on line.
func (t TokenSet) ColorAt(line, runeOffset int) (uint8, bool) {
	style, ok := t.StyleAt(line, runeOffset)
	return style.Color, ok
}

// StyleAt chooses the highest-priority token covering runeOffset on line.
func (t TokenSet) StyleAt(line, runeOffset int) (TokenStyle, bool) {
	tokens := t.Tokens
	if t.ByLine != nil {
		tokens = t.ByLine[line]
	}
	best := TokenStyle{Color: ColorDefault, Priority: -1}
	found := false
	for _, token := range tokens {
		if token.Line != line || runeOffset < token.StartRune || runeOffset >= token.EndRune {
			continue
		}
		style := StyleFor(token.Type, token.Modifiers)
		if !found || style.Priority > best.Priority {
			best = style
			found = true
		}
	}
	if !found {
		return TokenStyle{Color: ColorDefault, Priority: PriorityDefault}, false
	}
	return best, true
}

// DeprecatedAt reports whether the highest-priority token at the position has
// the deprecated modifier.
func (t TokenSet) DeprecatedAt(line, runeOffset int) bool {
	style, ok := t.StyleAt(line, runeOffset)
	return ok && style.Deprecated
}
