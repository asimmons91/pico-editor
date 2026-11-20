// Package lsp owns the in-process pico-lsp client, document synchronization,
// language-feature caches, and the UI-facing feature results.
package lsp

import (
	"log/slog"
	"time"
)

// Status is the lifecycle state of the embedded language server.
type Status string

const (
	StatusOff      Status = "off"
	StatusStarting Status = "starting"
	StatusReady    Status = "ready"
	StatusFailed   Status = "failed"
)

// Severity mirrors LSP diagnostic severity without exposing protocol types.
type Severity uint8

const (
	SeverityError   Severity = 1
	SeverityWarning Severity = 2
	SeverityInfo    Severity = 3
	SeverityHint    Severity = 4
)

// Position is an LSP position: zero-based line and UTF-16 column.
type Position struct {
	Line      int
	Character int
}

// Range is an LSP half-open range.
type Range struct {
	Start Position
	End   Position
}

// Diagnostic is one cached server diagnostic in full-document coordinates.
type Diagnostic struct {
	URI      string
	Version  int
	Range    Range
	Severity Severity
	Message  string
	Source   string
}

// Token is one semantic token expressed in editor code-buffer coordinates.
// Line is relative to the code buffer, not the full cart.
type Token struct {
	Line      int
	StartRune int
	EndRune   int
	Type      string
	Modifiers []string
}

// TokenSet is the semantic token cache for one document version.
type TokenSet struct {
	URI     string
	Version int
	Tokens  []Token
	ByLine  map[int][]Token
}

// CartInfo is the pico8/cartInfo result used by the status bar.
type CartInfo struct {
	RootURI           string
	Tokens            int
	TokenLimit        int
	Chars             int
	CharLimit         int
	Compressed        int
	CompressedLimit   int
	CompressedWarning int
	CompressedStatus  string
}

// Unavailable reports that the server could not produce cart budget data.
func (c CartInfo) Unavailable() bool {
	return c.CompressedStatus == "" || c.CompressedStatus == "unavailable"
}

// TextEdit is one LSP edit in full-document coordinates.
type TextEdit struct {
	Range   Range
	NewText string
}

// CompletionItem is the editor-facing subset of a completion item.
type CompletionItem struct {
	Label         string
	Detail        string
	Documentation string
	TextEdit      *TextEdit
	InsertText    string
	Additional    []TextEdit
}

// Hover is plain-text hover content.
type Hover struct {
	Text string
}

// SignatureParameter is one parameter with an optional label span.
type SignatureParameter struct {
	Label      string
	Start, End int // UTF-16 offsets into the signature label, -1 when unknown
	Document   string
}

// Signature is one signature-help entry.
type Signature struct {
	Label          string
	Documentation  string
	Parameters     []SignatureParameter
	ActiveParam    int
	ActiveParamSet bool
}

// SignatureHelp is the signature-help popup state.
type SignatureHelp struct {
	Signatures      []Signature
	ActiveSignature int
	ActiveParameter int
}

// Location is a go-to-definition target in full-document coordinates.
type Location struct {
	URI   string
	Range Range
}

// Glyph is one P8SCII catalog entry from pico8/glyphs.
type Glyph struct {
	Name        string
	Text        string
	Byte        int
	Aliases     []string
	Category    string
	Description string
	Context     string
	Custom      bool
}

// EventKind identifies one asynchronous result from the LSP service.
type EventKind uint8

const (
	EventStatus EventKind = iota + 1
	EventDiagnostics
	EventTokens
	EventSemanticRefresh
	EventCompletion
	EventHover
	EventSignature
	EventDefinition
	EventCartInfo
	EventGlyphs
	EventMessage
	EventRequestError
	EventFailure
)

// Event is one asynchronous LSP result. Events are applied on the Update
// goroutine by the application.
type Event struct {
	Kind EventKind

	Status  Status
	Message string
	Err     error

	URI     string
	Version int

	Diagnostics []Diagnostic
	Tokens      TokenSet
	CartInfo    *CartInfo

	RequestID uint64
	Position  Position

	Completion *CompletionResult
	Hover      *Hover
	Signature  *SignatureHelp
	Locations  []Location
	Glyphs     []Glyph
}

// CompletionResult carries a completion response and its request context.
type CompletionResult struct {
	Items    []CompletionItem
	Manual   bool
	Trigger  string
	Prefix   string
	Document string
	Position Position
}

// Options configures the embedded service.
type Options struct {
	WorkspaceRoot string
	Settings      Settings
	Log           *slog.Logger
	Version       string
	// Debounce overrides documentDebounce when positive.
	Debounce time.Duration
}
