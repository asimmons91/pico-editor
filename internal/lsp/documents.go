package lsp

import (
	"fmt"
	"time"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// DidOpen starts synchronizing a document. Calling it for an already-open URI
// advances the text instead of sending a second didOpen.
func (s *Service) DidOpen(documentURI, languageID string, version int, text string, cart bool) {
	if documentURI == "" {
		return
	}
	s.mu.Lock()
	if existing, ok := s.docs[documentURI]; ok {
		same := existing.Text == text && existing.Version == version
		s.mu.Unlock()
		if !same {
			s.DidChange(documentURI, languageID, version, text, cart)
		}
		return
	}
	doc := &document{
		URI:        documentURI,
		LanguageID: languageID,
		Version:    version,
		Text:       text,
		Cart:       cart,
	}
	s.docs[documentURI] = doc
	c := s.client
	if c != nil {
		doc.Sent = true
	}
	snapshot := *doc
	s.mu.Unlock()
	if c != nil {
		s.sendOpen(snapshot)
	}
}

// DidChange records the latest full text and marks the document for the
// debounced didChange notification.
func (s *Service) DidChange(documentURI, languageID string, version int, text string, cart bool) {
	if documentURI == "" {
		return
	}
	s.mu.Lock()
	doc, ok := s.docs[documentURI]
	if !ok {
		doc = &document{
			URI:        documentURI,
			LanguageID: languageID,
			Version:    version,
			Text:       text,
			Cart:       cart,
		}
		s.docs[documentURI] = doc
	}
	if version <= doc.Version && doc.Sent {
		s.mu.Unlock()
		return
	}
	doc.LanguageID = languageID
	doc.Version = version
	doc.Text = text
	doc.Cart = cart
	doc.Dirty = true
	doc.LastChange = time.Now()
	s.mu.Unlock()
}

// Flush sends debounced full-text changes and settled cart-info requests. It
// returns true when work was queued.
func (s *Service) Flush(now time.Time) bool {
	type change struct {
		doc  document
		open bool
	}
	var changes []change
	s.mu.Lock()
	if s.client == nil {
		s.mu.Unlock()
		return false
	}
	for _, doc := range s.docs {
		if doc.NeedsOpen {
			doc.NeedsOpen = false
			doc.Dirty = false
			doc.Sent = true
			changes = append(changes, change{doc: *doc, open: true})
			continue
		}
		if doc.Dirty && now.Sub(doc.LastChange) >= documentDebounce {
			doc.Dirty = false
			doc.Sent = true
			changes = append(changes, change{doc: *doc})
		}
	}
	s.mu.Unlock()
	for _, item := range changes {
		doc := item.doc
		if item.open {
			s.enqueueDocument("textDocument/didOpen", &protocol.DidOpenTextDocumentParams{
				TextDocument: protocol.TextDocumentItem{
					URI:        uri.URI(doc.URI),
					LanguageID: protocol.LanguageKind(doc.LanguageID),
					Version:    int32(doc.Version),
					Text:       doc.Text,
				},
			}, doc.URI, doc.Version)
			continue
		}
		s.enqueueDocument("textDocument/didChange", &protocol.DidChangeTextDocumentParams{
			TextDocument: protocol.VersionedTextDocumentIdentifier{
				TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: uri.URI(doc.URI)},
				Version:                int32(doc.Version),
			},
			ContentChanges: []protocol.TextDocumentContentChangeEvent{
				&protocol.TextDocumentContentChangeWholeDocument{Text: doc.Text},
			},
		}, doc.URI, doc.Version)
	}
	return len(changes) > 0
}

// DidSave tells the server a document was written. The server already has the
// text from didChange.
func (s *Service) DidSave(documentURI string) {
	if documentURI == "" {
		return
	}
	s.mu.Lock()
	doc, ok := s.docs[documentURI]
	c := s.client
	var pending *document
	if ok && doc.Dirty {
		doc.Dirty = false
		doc.Sent = true
		snapshot := *doc
		pending = &snapshot
	}
	s.mu.Unlock()
	if !ok || c == nil {
		return
	}
	if pending != nil {
		s.enqueueDocument("textDocument/didChange", &protocol.DidChangeTextDocumentParams{
			TextDocument: protocol.VersionedTextDocumentIdentifier{
				TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: uri.URI(pending.URI)},
				Version:                int32(pending.Version),
			},
			ContentChanges: []protocol.TextDocumentContentChangeEvent{
				&protocol.TextDocumentContentChangeWholeDocument{Text: pending.Text},
			},
		}, pending.URI, pending.Version)
	}
	s.enqueue("textDocument/didSave", &protocol.DidSaveTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(documentURI)},
	})
	s.RequestCartInfo(documentURI)
}

// DidClose stops syncing a document and drops its caches.
func (s *Service) DidClose(documentURI string) {
	if documentURI == "" {
		return
	}
	s.mu.Lock()
	_, ok := s.docs[documentURI]
	delete(s.docs, documentURI)
	delete(s.sentVersions, documentURI)
	if notify := s.sentNotify[documentURI]; notify != nil {
		close(notify)
		delete(s.sentNotify, documentURI)
	}
	for key := range s.latest {
		if key.uri == documentURI {
			delete(s.latest, key)
		}
	}
	c := s.client
	s.mu.Unlock()
	if !ok {
		return
	}
	if c != nil {
		s.enqueue("textDocument/didClose", &protocol.DidCloseTextDocumentParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(documentURI)},
		})
	}
	s.ClearDocument(documentURI)
}

func (s *Service) sendOpen(doc document) {
	s.enqueueDocument("textDocument/didOpen", &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        uri.URI(doc.URI),
			LanguageID: protocol.LanguageKind(doc.LanguageID),
			Version:    int32(doc.Version),
			Text:       doc.Text,
		},
	}, doc.URI, doc.Version)
}

// DocumentVersion returns the last recorded version for a URI.
func (s *Service) DocumentVersion(documentURI string) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, ok := s.docs[documentURI]
	if !ok {
		return 0, false
	}
	return doc.Version, true
}

// OpenDocumentCount reports how many documents are being synced.
func (s *Service) OpenDocumentCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.docs)
}

// String implements fmt.Stringer for diagnostics and tests.
func (s *Service) String() string {
	return fmt.Sprintf("lsp.Service(%s)", s.Status())
}
