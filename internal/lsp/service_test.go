package lsp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/asimmons91/pico-lsp/pkg/p8scii"

	"github.com/asimmons91/pico-editor/internal/cart"
	"github.com/asimmons91/pico-editor/internal/editor"
)

func TestServiceSemanticTokensDiagnosticsAndCartInfo(t *testing.T) {
	dir := t.TempDir()
	path := copyFixture(t, "smoke.p8", dir, "smoke.p8")
	service, cancel := startTestService(t, dir)
	defer func() {
		_ = service.Stop(context.Background())
		cancel()
	}()
	document, err := cart.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	text := readFile(t, path)
	uri := URIForPath(path)
	service.DidOpen(uri, "pico8", document.Code.Version, text, true)

	service.RequestTokens(document.Code, document.BaseLine, true)
	tokens := waitEvent(t, service, EventTokens)
	if len(tokens.Tokens.Tokens) == 0 {
		t.Fatal("no semantic tokens")
	}
	seen := make(map[string]bool)
	for _, token := range tokens.Tokens.Tokens {
		seen[token.Type] = true
	}
	for _, want := range []string{"keyword", "comment", "string", "number", "function", "variable"} {
		if !seen[want] {
			t.Errorf("semantic token types missing %q: %v", want, seen)
		}
	}

	service.DidChange(uri, "pico8", document.Code.Version+1, strings.Replace(text, "x=64", "x=65", 1), true)
	if !service.Flush(time.Now().Add(documentDebounce + time.Millisecond)) {
		t.Fatal("flush did not queue the edit")
	}

	service.RequestCartInfo(uri)
	infoEvent := waitEvent(t, service, EventCartInfo)
	if infoEvent.CartInfo == nil {
		t.Fatal("cart info unavailable")
	}
	info := *infoEvent.CartInfo
	if info.Tokens == 0 || info.Chars == 0 || info.TokenLimit != 8192 || info.CharLimit != 65535 {
		t.Fatalf("cart info = %+v", info)
	}
	if info.RootURI != uri {
		t.Fatalf("root uri = %q, want %q", info.RootURI, uri)
	}

	// A visible diagnostic requires an annotated mismatch.
	mismatch := "pico-8 cartridge // pico-8.com\nversion 42\n__lua__\n---@type number\nlocal value = \"bad\"\nprint(value)\n"
	service.DidChange(uri, "pico8", document.Code.Version+2, mismatch, true)
	service.Flush(time.Now().Add(documentDebounce + time.Millisecond))
	diagnostics := waitEvent(t, service, EventDiagnostics)
	if len(diagnostics.Diagnostics) == 0 {
		t.Fatal("expected a diagnostic for the annotated mismatch")
	}
	found := false
	for _, diagnostic := range diagnostics.Diagnostics {
		if diagnostic.Severity == SeverityError || diagnostic.Severity == SeverityWarning {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %+v", diagnostics.Diagnostics)
	}

	service.RequestGlyphs()
	glyphs := waitEvent(t, service, EventGlyphs)
	if len(glyphs.Glyphs) == 0 {
		t.Fatal("glyph catalog was empty")
	}
}

func TestServiceCompletionHoverSignatureDefinition(t *testing.T) {
	dir := fixtureDir(t, "navigation")
	path := filepath.Join(dir, "cart.p8")
	service, cancel := startTestService(t, dir)
	defer func() {
		_ = service.Stop(context.Background())
		cancel()
	}()
	document, err := cart.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	openDocument(t, service, path, "pico8", document.Code.Version, readFile(t, path), true)
	for _, include := range []string{"lib/player.lua", "lib/util.lua"} {
		includePath := filepath.Join(dir, include)
		buffer, err := editor.Open(includePath)
		if err != nil {
			t.Fatal(err)
		}
		openDocument(t, service, includePath, "lua", buffer.Version, readFile(t, includePath), false)
	}

	// Completion inside "print".
	line := document.Code.Lines[8] // root code line 8: "  print(player)"
	prefix := strings.Index(line, "print") + 3
	service.RequestCompletion(document.Code, document.BaseLine, editor.Pos{Line: 8, Rune: prefix}, true, "", "pri")
	completion := waitEvent(t, service, EventCompletion)
	if completion.Completion == nil || len(completion.Completion.Items) == 0 {
		t.Fatalf("completion = %+v", completion.Completion)
	}
	if !containsLabel(completion.Completion.Items, "print") {
		t.Fatalf("completion items missing print: %+v", completion.Completion.Items)
	}

	// Hover over make_player.
	callLine := 2
	callRune := strings.Index(document.Code.Lines[callLine], "make_player")
	service.RequestHover(document.Code, document.BaseLine, editor.Pos{Line: callLine, Rune: callRune})
	hover := waitEvent(t, service, EventHover)
	if hover.Hover == nil || hover.Hover.Text == "" {
		t.Fatal("hover was empty")
	}

	// Signature help inside print(.
	signatureRune := strings.Index(line, "(") + 1
	service.RequestSignature(document.Code, document.BaseLine, editor.Pos{Line: 8, Rune: signatureRune}, false)
	signature := waitEvent(t, service, EventSignature)
	if signature.Signature == nil || len(signature.Signature.Signatures) == 0 {
		t.Fatal("signature help was empty")
	}

	// Definition of make_player jumps to the included player.lua.
	service.RequestDefinition(document.Code, document.BaseLine, editor.Pos{Line: callLine, Rune: callRune})
	definition := waitEvent(t, service, EventDefinition)
	if len(definition.Locations) == 0 {
		t.Fatal("definition was empty")
	}
	includeURI := URIForPath(filepath.Join(dir, "lib", "player.lua"))
	if definition.Locations[0].URI != includeURI {
		t.Fatalf("definition URI = %q, want %q", definition.Locations[0].URI, includeURI)
	}
	if definition.Locations[0].Range.Start.Line != 0 || definition.Locations[0].Range.Start.Character != 9 {
		t.Fatalf("definition range = %+v", definition.Locations[0].Range)
	}
}

func TestServiceGlyphCompletion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "glyph.lua")
	text := "local button = p8_left\n"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	service, cancel := startTestService(t, dir)
	defer func() {
		_ = service.Stop(context.Background())
		cancel()
	}()
	buffer, err := editor.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	openDocument(t, service, path, "lua", buffer.Version, text, false)

	position := editor.Pos{Line: 0, Rune: len([]rune("local button = p8_left"))}
	service.RequestCompletion(buffer, 0, position, true, "", "p8_left")
	event := waitEvent(t, service, EventCompletion)
	if event.Completion == nil {
		t.Fatal("no completion result")
	}
	want, _ := p8scii.GlyphFor("⬅️")
	found := false
	for _, item := range event.Completion.Items {
		if item.TextEdit != nil && item.TextEdit.NewText == want.Text {
			found = true
		}
	}
	if !found {
		t.Fatalf("p8_left completion missing canonical glyph %q: %+v", want.Text, event.Completion.Items)
	}
}

func TestServiceDidSaveFlushesPendingChange(t *testing.T) {
	dir := t.TempDir()
	path := copyFixture(t, "smoke.p8", dir, "smoke.p8")
	service, cancel := startTestService(t, dir)
	defer func() {
		_ = service.Stop(context.Background())
		cancel()
	}()
	document, err := cart.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	documentURI := URIForPath(path)
	text := readFile(t, path)
	service.DidOpen(documentURI, "pico8", document.Code.Version, text, true)
	updated := strings.Replace(text, "x=64", "x=65", 1)
	service.DidChange(documentURI, "pico8", document.Code.Version+1, updated, true)
	// Save arrives before the 120 ms debounce. DidSave must flush the pending
	// text so cart-info does not wait for a version the server never saw.
	service.DidSave(documentURI)
	event := waitEvent(t, service, EventCartInfo)
	if event.CartInfo == nil {
		t.Fatal("cart info was unavailable after save with a pending change")
	}
}

func TestServiceCartInfoUnavailableForStandaloneLua(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "standalone.lua")
	if err := os.WriteFile(path, []byte("print(1)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	service, cancel := startTestService(t, dir)
	defer func() {
		_ = service.Stop(context.Background())
		cancel()
	}()
	buffer, err := editor.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	openDocument(t, service, path, "lua", buffer.Version, readFile(t, path), false)
	service.RequestCartInfo(URIForPath(path))
	event := waitEvent(t, service, EventCartInfo)
	if event.CartInfo == nil || !event.CartInfo.Unavailable() {
		t.Fatalf("standalone cart info = %+v, want unavailable", event.CartInfo)
	}
}

func TestServiceServerCloseMarksFailed(t *testing.T) {
	dir := t.TempDir()
	path := copyFixture(t, "smoke.p8", dir, "smoke.p8")
	service, cancel := startTestService(t, dir)
	defer cancel()
	document, err := cart.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	service.DidOpen(URIForPath(path), "pico8", document.Code.Version, readFile(t, path), true)
	service.mu.Lock()
	c := service.client
	service.mu.Unlock()
	if c == nil {
		t.Fatal("no client to close")
	}
	_ = c.serverConn.Close()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, event := range service.Poll() {
			if event.Kind == EventStatus && event.Status == StatusFailed {
				return
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("service did not report failure after the pipe closed (status %s)", service.Status())
}

func TestServiceRestartReopensDocuments(t *testing.T) {
	dir := t.TempDir()
	path := copyFixture(t, "smoke.p8", dir, "smoke.p8")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := New(Options{WorkspaceRoot: dir, Settings: DefaultSettings()})
	go func() { _ = service.Start(ctx) }()
	waitStatus(t, service, StatusReady)
	document, err := cart.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	uri := URIForPath(path)
	service.DidOpen(uri, "pico8", document.Code.Version, readFile(t, path), true)
	service.RequestCartInfo(uri)
	if event := waitEvent(t, service, EventCartInfo); event.CartInfo == nil {
		t.Fatal("cart info before restart")
	}

	_ = service.Stop(context.Background())
	waitStatus(t, service, StatusOff)
	go func() { _ = service.Start(ctx) }()
	waitStatus(t, service, StatusReady)
	service.RequestCartInfo(uri)
	if event := waitEvent(t, service, EventCartInfo); event.CartInfo == nil {
		t.Fatal("cart info after restart")
	}
	_ = service.Stop(context.Background())
}

func TestServiceStopDuringStart(t *testing.T) {
	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		service := New(Options{WorkspaceRoot: t.TempDir(), Settings: DefaultSettings()})
		go func() { _ = service.Start(ctx) }()
		deadline := time.Now().Add(2 * time.Second)
		for service.Status() != StatusStarting && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		_ = service.Stop(context.Background())
		time.Sleep(20 * time.Millisecond)
		if status := service.Status(); status == StatusReady || status == StatusStarting {
			t.Fatalf("iteration %d: status %q after Stop returned", i, status)
		}
		cancel()
	}
}

func startTestService(t *testing.T, root string) (*Service, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	service := New(Options{WorkspaceRoot: root, Settings: DefaultSettings()})
	go func() { _ = service.Start(ctx) }()
	waitStatus(t, service, StatusReady)
	return service, cancel
}

func waitStatus(t *testing.T, service *Service, want Status) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		for _, event := range service.Poll() {
			if event.Kind == EventStatus && event.Status == want {
				return
			}
			if event.Kind == EventFailure && event.Err != nil {
				t.Fatalf("lsp failed before %s: %v", want, event.Err)
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("lsp status did not become %s (current %s)", want, service.Status())
}

func waitEvent(t *testing.T, service *Service, kind EventKind) Event {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		for _, event := range service.Poll() {
			if event.Kind == kind {
				return event
			}
			if event.Kind == EventRequestError && event.Err != nil {
				t.Fatalf("lsp request failed while waiting for kind %d: %v", kind, event.Err)
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for lsp event %d", kind)
	return Event{}
}

func openDocument(t *testing.T, service *Service, path, languageID string, version int, text string, isCart bool) {
	t.Helper()
	service.DidOpen(URIForPath(path), languageID, version, text, isCart)
}

func fixtureDir(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join("testdata", name)
	err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dir, relative)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func copyFixture(t *testing.T, name, dir, target string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, target)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func containsLabel(items []CompletionItem, label string) bool {
	for _, item := range items {
		if item.Label == label || strings.HasPrefix(item.Label, label) {
			return true
		}
	}
	return false
}
