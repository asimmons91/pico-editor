package lsp

import (
	"fmt"

	"github.com/asimmons91/pico-editor/internal/editor"
)

// DecodeTokens converts an LSP semantic-token delta stream to code-buffer
// tokens. Columns and lengths are UTF-16; both endpoints expand to P8SCII
// cluster boundaries.
func DecodeTokens(data []uint32, tokenTypes, modifierNames []string, buffer *editor.Buffer, baseLine int, document string, version int) (TokenSet, error) {
	set := TokenSet{URI: document, Version: version, ByLine: make(map[int][]Token)}
	if len(data)%5 != 0 {
		return set, fmt.Errorf("semantic tokens: %d values is not a multiple of 5", len(data))
	}
	line := 0
	start := 0
	for index := 0; index < len(data); index += 5 {
		deltaLine := int(data[index])
		deltaStart := int(data[index+1])
		length := int(data[index+2])
		tokenType := int(data[index+3])
		modBits := data[index+4]

		line += deltaLine
		if deltaLine == 0 {
			start += deltaStart
		} else {
			start = deltaStart
		}
		codeLine := line - baseLine
		if codeLine < 0 || codeLine >= len(buffer.Lines) {
			continue
		}
		if tokenType < 0 || tokenType >= len(tokenTypes) {
			continue
		}
		startPos := buffer.PosFromUTF16Start(codeLine, start)
		endPos := buffer.PosFromUTF16End(codeLine, start+length)
		if !startPos.Before(endPos) {
			continue
		}
		modifiers := decodeModifiers(modBits, modifierNames)
		token := Token{
			Line:      codeLine,
			StartRune: startPos.Rune,
			EndRune:   endPos.Rune,
			Type:      tokenTypes[tokenType],
			Modifiers: modifiers,
		}
		set.Tokens = append(set.Tokens, token)
		set.ByLine[codeLine] = append(set.ByLine[codeLine], token)
	}
	return set, nil
}

func decodeModifiers(bits uint32, names []string) []string {
	if bits == 0 || len(names) == 0 {
		return nil
	}
	out := make([]string, 0, 2)
	for index, name := range names {
		if index >= 32 {
			break
		}
		if bits&(1<<uint(index)) != 0 {
			out = append(out, name)
		}
	}
	return out
}
