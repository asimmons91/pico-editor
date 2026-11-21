package lsp

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"go.lsp.dev/protocol"

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
	if !style.Deprecated {
		t.Fatal("deprecated modifier did not set the marker flag")
	}
}

func TestSettingsNormalize(t *testing.T) {
	settings := Settings{TypeChecking: "turbo", MaxProblems: -1}.normalize()
	if settings.TypeChecking != "basic" || settings.MaxProblems != 100 {
		t.Fatalf("normalized = %+v", settings)
	}
}

func TestSignatureParameterOffsetSkipsFunctionName(t *testing.T) {
	help := convertSignature(&protocol.SignatureHelp{
		Signatures: []protocol.SignatureInformation{{
			Label: "function init(n: number): number",
			Parameters: []protocol.ParameterInformation{{
				Label: protocol.String("n"),
			}},
		}},
	})
	if help == nil || len(help.Signatures) != 1 || len(help.Signatures[0].Parameters) != 1 {
		t.Fatalf("signature = %+v", help)
	}
	parameter := help.Signatures[0].Parameters[0]
	want := strings.Index(help.Signatures[0].Label, "(n")
	if want >= 0 {
		want++
	}
	if parameter.Start != want {
		t.Fatalf("parameter start = %d, want %d", parameter.Start, want)
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

func TestM7ColorTable(t *testing.T) {
	table := map[string]struct {
		color    uint8
		priority int
	}{
		"keyword":   {ColorKeyword, PriorityKeyword},
		"string":    {ColorString, PriorityString},
		"number":    {ColorNumber, PriorityNumber},
		"comment":   {ColorComment, PriorityComment},
		"operator":  {ColorOperator, PriorityOperator},
		"variable":  {ColorVariable, PriorityVariable},
		"parameter": {ColorParameter, PriorityParameter},
		"property":  {ColorProperty, PriorityProperty},
		"function":  {ColorFunction, PriorityFunction},
		"type":      {ColorType, PriorityType},
		"class":     {ColorType, PriorityType},
		"enum":      {ColorType, PriorityType},
		"namespace": {ColorNamespace, PriorityNamespace},
		"macro":     {ColorMacro, PriorityType},
		"regexp":    {ColorRegexp, PriorityString},
		"decorator": {ColorDecorator, PriorityType},
		"modifier":  {ColorModifier, PriorityComment},
		"unknown":   {ColorDefault, PriorityDefault},
	}
	for tokenType, want := range table {
		style := StyleFor(tokenType, nil)
		if style.Color != want.color || style.Priority != want.priority {
			t.Errorf("StyleFor(%q) = %+v, want color %d priority %d", tokenType, style, want.color, want.priority)
		}
	}
	for modifier, color := range map[string]uint8{
		"readonly": ColorReadonly, "static": ColorReadonly,
		"defaultLibrary": ColorLibrary, "deprecated": ColorDeprecated,
	} {
		if style := StyleFor("variable", []string{modifier}); style.Color != color {
			t.Errorf("modifier %q color = %d, want %d", modifier, style.Color, color)
		}
	}
}

func TestLSPMessagesAreNotLoggedVerbatim(t *testing.T) {
	var logs bytes.Buffer
	service := New(Options{Log: slog.New(slog.NewTextHandler(&logs, nil))})
	handler := &clientHandler{service: service}
	if err := handler.ShowMessage(context.Background(), &protocol.ShowMessageParams{Message: "SECRET_SHOW_TEXT"}); err != nil {
		t.Fatal(err)
	}
	if err := handler.LogMessage(context.Background(), &protocol.LogMessageParams{Type: protocol.MessageTypeInfo, Message: "SECRET_LOG_TEXT"}); err != nil {
		t.Fatal(err)
	}
	output := logs.String()
	for _, secret := range []string{"SECRET_SHOW_TEXT", "SECRET_LOG_TEXT"} {
		if strings.Contains(output, secret) {
			t.Fatalf("log output contains message text %q: %s", secret, output)
		}
	}
	if !strings.Contains(output, "chars=") {
		t.Fatalf("log output = %q, want bounded message lengths", output)
	}
}
