package lsp

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"go.lsp.dev/protocol"
)

const (
	documentDebounce  = 120 * time.Millisecond
	cartInfoDebounce  = 300 * time.Millisecond
	eventQueueSize    = 256
	outboundQueueSize = 256
)

type outbound struct {
	method      string
	params      any
	documentURI string
	version     int
}

// Service owns the in-process language server and its document/feature caches.
// Lifecycle and document methods are safe to call from the Update goroutine;
// background request goroutines only emit events.
type Service struct {
	opts     Options
	log      *slog.Logger
	settings Settings

	events chan Event
	out    chan outbound

	mu           sync.Mutex
	status       Status
	client       *client
	runCtx       context.Context
	stopping     bool
	docs         map[string]*document
	tokens       map[string]TokenSet
	diagnostics  map[string][]Diagnostic
	cartInfo     map[string]CartInfo
	glyphs       []Glyph
	tokenTypes   []string
	tokenMods    []string
	requestSeq   uint64
	latest       map[requestKey]uint64
	sentVersions map[string]int
	sentNotify   map[string]chan struct{}
	writerCancel context.CancelFunc
}

type document struct {
	URI        string
	LanguageID string
	Version    int
	Text       string
	Cart       bool
	Dirty      bool
	LastChange time.Time
	Sent       bool

	CartInfoVersion int
}

// New returns a stopped service. Call Start to launch the embedded server.
func New(opts Options) *Service {
	logger := opts.Log
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	if opts.WorkspaceRoot != "" {
		opts.WorkspaceRoot = filepath.Clean(opts.WorkspaceRoot)
	}
	return &Service{
		opts:         opts,
		log:          logger.With("component", "lsp"),
		settings:     opts.Settings.normalize(),
		events:       make(chan Event, eventQueueSize),
		out:          make(chan outbound, outboundQueueSize),
		status:       StatusOff,
		docs:         make(map[string]*document),
		tokens:       make(map[string]TokenSet),
		diagnostics:  make(map[string][]Diagnostic),
		cartInfo:     make(map[string]CartInfo),
		latest:       make(map[requestKey]uint64),
		sentVersions: make(map[string]int),
		sentNotify:   make(map[string]chan struct{}),
	}
}

// Status returns the last applied lifecycle state.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Settings returns the effective settings.
func (s *Service) Settings() Settings {
	return s.settings
}

// Start launches pico-lsp and opens every known document. It is safe to call
// from a background goroutine.
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.status == StatusStarting || s.status == StatusReady {
		s.mu.Unlock()
		return nil
	}
	s.status = StatusStarting
	s.stopping = false
	s.mu.Unlock()
	s.emit(Event{Kind: EventStatus, Status: StatusStarting})

	c, result, err := startClient(ctx, s.opts, &clientHandler{service: s})
	if err != nil {
		s.fail(err)
		return err
	}

	s.mu.Lock()
	s.client = c
	s.runCtx = ctx
	s.applyLegend(result)
	docs := make([]document, 0, len(s.docs))
	for _, doc := range s.docs {
		doc.Sent = false
		docs = append(docs, *doc)
	}
	s.status = StatusReady
	s.sentVersions = make(map[string]int)
	for _, doc := range s.docs {
		if notify := s.sentNotify[doc.URI]; notify != nil {
			close(notify)
			delete(s.sentNotify, doc.URI)
		}
	}
	writerCtx, writerCancel := context.WithCancel(ctx)
	s.writerCancel = writerCancel
	s.mu.Unlock()

	s.emit(Event{Kind: EventStatus, Status: StatusReady})
	for _, doc := range docs {
		s.sendOpen(doc)
	}
	go s.writer(writerCtx, c)
	go s.watch(c)
	return nil
}

// Stop shuts down the server and clears feature caches. Documents stay known
// so a restart can reopen them.
func (s *Service) Stop(ctx context.Context) error {
	s.mu.Lock()
	c := s.client
	s.client = nil
	s.runCtx = nil
	s.stopping = true
	if s.writerCancel != nil {
		s.writerCancel()
		s.writerCancel = nil
	}
	s.status = StatusOff
	s.tokens = make(map[string]TokenSet)
	s.diagnostics = make(map[string][]Diagnostic)
	s.cartInfo = make(map[string]CartInfo)
	s.sentVersions = make(map[string]int)
	s.latest = make(map[requestKey]uint64)
	for documentURI, notify := range s.sentNotify {
		close(notify)
		delete(s.sentNotify, documentURI)
	}
	for _, doc := range s.docs {
		doc.Sent = false
	}
	s.mu.Unlock()
	s.emit(Event{Kind: EventStatus, Status: StatusOff})
	if c != nil {
		c.stop(ctx)
	}
	return nil
}

// Restart stops and starts the server. Callers should run it in a goroutine.
func (s *Service) Restart(ctx context.Context) error {
	_ = s.Stop(ctx)
	return s.Start(ctx)
}

func (s *Service) fail(err error) {
	s.mu.Lock()
	s.client = nil
	s.status = StatusFailed
	if s.writerCancel != nil {
		s.writerCancel()
		s.writerCancel = nil
	}
	s.mu.Unlock()
	s.emit(Event{Kind: EventStatus, Status: StatusFailed})
	s.emit(Event{Kind: EventFailure, Err: err})
}

func (s *Service) watch(c *client) {
	<-c.serverConn.Done()
	s.mu.Lock()
	current := s.client == c && !s.stopping
	s.mu.Unlock()
	if current {
		s.fail(fmt.Errorf("pico-lsp connection closed"))
	}
}

func (s *Service) applyLegend(result *protocol.InitializeResult) {
	if result == nil {
		return
	}
	switch provider := result.Capabilities.SemanticTokensProvider.(type) {
	case *protocol.SemanticTokensOptions:
		s.tokenTypes = append([]string(nil), provider.Legend.TokenTypes...)
		s.tokenMods = append([]string(nil), provider.Legend.TokenModifiers...)
	case *protocol.SemanticTokensRegistrationOptions:
		s.tokenTypes = append([]string(nil), provider.Legend.TokenTypes...)
		s.tokenMods = append([]string(nil), provider.Legend.TokenModifiers...)
	}
}

func (s *Service) writer(ctx context.Context, c *client) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-s.out:
			callCtx, cancel := context.WithTimeout(ctx, requestTimeoutSlow)
			err := c.conn.Notify(callCtx, msg.method, msg.params)
			cancel()
			if err != nil {
				s.log.Warn("lsp notify", "method", msg.method, "error", err)
				continue
			}
			if msg.documentURI != "" {
				s.markSent(msg.documentURI, msg.version)
			}
		}
	}
}

func (s *Service) markSent(documentURI string, version int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if version > s.sentVersions[documentURI] {
		s.sentVersions[documentURI] = version
	}
	if notify := s.sentNotify[documentURI]; notify != nil {
		close(notify)
		delete(s.sentNotify, documentURI)
	}
}

func (s *Service) waitDocument(ctx context.Context, documentURI string, version int) bool {
	for {
		s.mu.Lock()
		if s.client == nil {
			s.mu.Unlock()
			return false
		}
		if s.sentVersions[documentURI] >= version {
			s.mu.Unlock()
			return true
		}
		notify := s.sentNotify[documentURI]
		if notify == nil {
			notify = make(chan struct{})
			s.sentNotify[documentURI] = notify
		}
		s.mu.Unlock()
		select {
		case <-notify:
		case <-ctx.Done():
			return false
		}
	}
}

func (s *Service) enqueue(method string, params any) {
	s.enqueueDocument(method, params, "", 0)
}

func (s *Service) enqueueDocument(method string, params any, documentURI string, version int) {
	select {
	case s.out <- outbound{method: method, params: params, documentURI: documentURI, version: version}:
	default:
		s.log.Warn("lsp outbound queue full", "method", method)
		if documentURI != "" {
			s.mu.Lock()
			if doc := s.docs[documentURI]; doc != nil {
				doc.Dirty = true
				doc.Sent = false
			}
			s.mu.Unlock()
		}
	}
}

func (s *Service) emit(event Event) {
	select {
	case s.events <- event:
	default:
		s.log.Warn("lsp event queue full", "kind", event.Kind)
	}
}

// Poll drains asynchronous events and applies their caches. It must run on the
// application's Update goroutine.
func (s *Service) Poll() []Event {
	var out []Event
	for {
		select {
		case event := <-s.events:
			s.apply(event)
			out = append(out, event)
		default:
			return out
		}
	}
}

func (s *Service) apply(event Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch event.Kind {
	case EventStatus:
		s.status = event.Status
	case EventDiagnostics:
		current := s.diagnostics[event.URI]
		if event.Version > 0 && len(current) > 0 {
			cachedVersion := current[0].Version
			if event.Version < cachedVersion {
				return
			}
		}
		if len(event.Diagnostics) == 0 {
			delete(s.diagnostics, event.URI)
			return
		}
		s.diagnostics[event.URI] = append([]Diagnostic(nil), event.Diagnostics...)
	case EventTokens:
		current := s.tokens[event.URI]
		if event.Tokens.Version > 0 && current.Version > event.Tokens.Version {
			return
		}
		s.tokens[event.URI] = event.Tokens
	case EventCartInfo:
		if event.CartInfo != nil {
			s.cartInfo[event.URI] = *event.CartInfo
		}
	case EventGlyphs:
		s.glyphs = append([]Glyph(nil), event.Glyphs...)
	}
}

// Tokens returns the cached semantic tokens for a URI.
func (s *Service) Tokens(uri string) TokenSet {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tokens[uri]
}

// Diagnostics returns the cached diagnostics for a URI.
func (s *Service) Diagnostics(uri string) []Diagnostic {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Diagnostic(nil), s.diagnostics[uri]...)
}

// CartInfo returns the cached cart budget for a URI.
func (s *Service) CartInfo(uri string) (CartInfo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	info, ok := s.cartInfo[uri]
	return info, ok
}

// Glyphs returns the last pico8/glyphs result.
func (s *Service) Glyphs() []Glyph {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Glyph(nil), s.glyphs...)
}

// ClearDocument drops cached feature data for a closed document.
func (s *Service) ClearDocument(uri string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, uri)
	delete(s.diagnostics, uri)
	delete(s.cartInfo, uri)
}

// TokenTypes returns the negotiated semantic token legend.
func (s *Service) TokenTypes() ([]string, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.tokenTypes...), append([]string(nil), s.tokenMods...)
}
