package lsp

import (
	"strings"
	"testing"

	"github.com/asimmons91/pico-editor/internal/editor"
)

func TestDecodeTokensExpandsClusters(t *testing.T) {
	buffer := editor.New("")
	buffer.Lines = []string{"a⬅️b"}
	// token 0: line 0, start 0, length 1 -> 'a'
	// token 1: line 0, start 1, length 1 -> inside the wide glyph, expands to
	// the whole base-rune/VS16 cluster.
	data := []uint32{
		0, 0, 1, 0, 0,
		0, 1, 1, 1, 0,
	}
	set, err := DecodeTokens(data, []string{"variable", "string"}, nil, buffer, 0, "file:///x.p8", 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Tokens) != 2 {
		t.Fatalf("tokens = %+v, want 2", set.Tokens)
	}
	if set.Tokens[0].StartRune != 0 || set.Tokens[0].EndRune != 1 {
		t.Fatalf("first token = %+v", set.Tokens[0])
	}
	if set.Tokens[1].StartRune != 1 || set.Tokens[1].EndRune != 3 {
		t.Fatalf("second token = %+v, want the whole cluster", set.Tokens[1])
	}
}

func TestDecodeTokensRejectsMalformedDelta(t *testing.T) {
	buffer := editor.New("")
	buffer.Lines = []string{"abc"}
	if _, err := DecodeTokens([]uint32{0, 0, 1}, nil, nil, buffer, 0, "file:///x", 1); err == nil {
		t.Fatal("malformed token data was accepted")
	}
}

func TestDecodeModifiers(t *testing.T) {
	buffer := editor.New("")
	buffer.Lines = []string{"abc"}
	data := []uint32{0, 0, 3, 0, 0b101}
	set, err := DecodeTokens(data, []string{"variable"}, []string{"declaration", "readonly", "deprecated"}, buffer, 0, "file:///x", 1)
	if err != nil {
		t.Fatal(err)
	}
	modifiers := strings.Join(set.Tokens[0].Modifiers, ",")
	if modifiers != "declaration,deprecated" {
		t.Fatalf("modifiers = %q", modifiers)
	}
}

func TestTokenPriorityAndModifiers(t *testing.T) {
	set := TokenSet{Tokens: []Token{
		{Line: 0, StartRune: 0, EndRune: 4, Type: "comment"},
		{Line: 0, StartRune: 0, EndRune: 4, Type: "keyword"},
	}}
	color, ok := set.ColorAt(0, 0)
	if !ok || color != ColorComment {
		t.Fatalf("color = %d, ok = %t, want comment", color, ok)
	}
	style := StyleFor("variable", []string{"readonly"})
	if style.Color != ColorReadonly {
		t.Fatalf("readonly color = %d", style.Color)
	}
	style = StyleFor("variable", []string{"deprecated"})
	if style.Color != ColorDeprecated {
		t.Fatalf("deprecated color = %d", style.Color)
	}
}

func TestSettingsNormalize(t *testing.T) {
	settings := Settings{TypeChecking: "turbo", MaxProblems: -1}.normalize()
	if settings.TypeChecking != "basic" || settings.MaxProblems != 100 {
		t.Fatalf("normalized = %+v", settings)
	}
}

func TestPathForURIRejectsRemote(t *testing.T) {
	if _, ok := PathForURI("https://example.com/x.p8"); ok {
		t.Fatal("non-file URI was accepted")
	}
	path, ok := PathForURI(URIForPath("/tmp/game.p8"))
	if !ok || path != "/tmp/game.p8" {
		t.Fatalf("path = %q, ok = %t", path, ok)
	}
}
