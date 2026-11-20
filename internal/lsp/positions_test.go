package lsp

import (
	"testing"

	"github.com/asimmons91/pico-editor/internal/editor"
)

func TestPositionRoundTripAcrossCartBaseLine(t *testing.T) {
	buffer := editor.New("")
	buffer.Lines = []string{"a⬅️b", "plain"}
	for _, pos := range []editor.Pos{{Line: 0, Rune: 0}, {Line: 0, Rune: 1}, {Line: 0, Rune: 3}, {Line: 1, Rune: 5}} {
		wire := PositionFor(buffer, 3, pos)
		if wire.Line != pos.Line+3 {
			t.Fatalf("position %+v mapped to line %d, want %d", pos, wire.Line, pos.Line+3)
		}
		back, ok := EditorPos(buffer, 3, wire)
		if !ok || back != pos {
			t.Fatalf("round trip %+v -> %+v -> %+v, ok=%t", pos, wire, back, ok)
		}
	}
	if _, ok := EditorPos(buffer, 3, Position{Line: 2, Character: 0}); ok {
		t.Fatal("header line was accepted as a code position")
	}
}

func TestEditorRangeExpandsClusters(t *testing.T) {
	buffer := editor.New("")
	buffer.Lines = []string{"a⬅️b"}
	start, end, ok := EditorRange(buffer, 0, Range{
		Start: Position{Line: 0, Character: 2},
		End:   Position{Line: 0, Character: 2},
	})
	if !ok {
		t.Fatal("range rejected")
	}
	if start.Rune != 1 || end.Rune != 3 {
		t.Fatalf("expanded range = %+v..%+v, want the wide cluster", start, end)
	}
}

func TestPathForURIRejectsVirtualDocuments(t *testing.T) {
	if _, ok := PathForURI("pico8-cart:/tmp/game.p8"); ok {
		t.Fatal("virtual cart URI was accepted")
	}
}
