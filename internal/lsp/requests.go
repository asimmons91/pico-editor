package lsp

import (
	"context"
	"errors"
	"strings"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"github.com/asimmons91/pico-editor/internal/editor"
)

type requestKind uint8

const (
	requestTokens requestKind = iota + 1
	requestCompletion
	requestHover
	requestSignature
	requestDefinition
	requestCartInfo
	requestGlyphs
)

type requestKey struct {
	kind requestKind
	uri  string
}

func (s *Service) beginRequest(kind requestKind, documentURI string) (uint64, *client, context.Context, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client == nil || s.runCtx == nil {
		return 0, nil, nil, false
	}
	s.requestSeq++
	id := s.requestSeq
	s.latest[requestKey{kind: kind, uri: documentURI}] = id
	return id, s.client, s.runCtx, true
}

func (s *Service) requestCurrent(kind requestKind, documentURI string, id uint64, version int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client == nil || s.latest[requestKey{kind: kind, uri: documentURI}] != id {
		return false
	}
	if version >= 0 {
		doc, ok := s.docs[documentURI]
		if !ok || doc.Version != version {
			return false
		}
	}
	return true
}

func (s *Service) requestFailed(kind requestKind, documentURI string, id uint64, err error) {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return
	}
	if !s.requestCurrent(kind, documentURI, id, -1) {
		return
	}
	if shouldReopenDocument(err) {
		s.reopenDocument(documentURI)
	}
	s.log.Warn("lsp request", "kind", kind, "uri", documentURI, "error", err)
	s.emit(Event{Kind: EventRequestError, URI: documentURI, Err: err})
}

func shouldReopenDocument(err error) bool {
	text := err.Error()
	return strings.Contains(text, "document is not open") ||
		strings.Contains(text, "no analysis for") ||
		strings.Contains(text, "no open document")
}

func (s *Service) reopenDocument(documentURI string) {
	s.mu.Lock()
	doc, ok := s.docs[documentURI]
	c := s.client
	if !ok || c == nil {
		s.mu.Unlock()
		return
	}
	doc.Sent = false
	snapshot := *doc
	s.mu.Unlock()
	s.sendOpen(snapshot)
}

// RequestTokens asks for full semantic tokens for buffer. The buffer is
// snapshotted so the response goroutine never reads mutable editor state.
func (s *Service) RequestTokens(buffer *editor.Buffer, baseLine int, cart bool) {
	if buffer == nil || buffer.Path == "" {
		return
	}
	documentURI := URIForPath(buffer.Path)
	version := buffer.Version
	snapshot := cloneBuffer(buffer)
	id, c, parent, ok := s.beginRequest(requestTokens, documentURI)
	if !ok {
		return
	}
	s.mu.Lock()
	tokenTypes := append([]string(nil), s.tokenTypes...)
	tokenMods := append([]string(nil), s.tokenMods...)
	s.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(parent, requestTimeoutSlow)
		defer cancel()
		if !s.waitDocument(ctx, documentURI, version) {
			return
		}
		result, err := c.server.SemanticTokensFull(ctx, &protocol.SemanticTokensParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(documentURI)},
		})
		if err != nil {
			s.requestFailed(requestTokens, documentURI, id, err)
			return
		}
		if result == nil {
			return
		}
		if !s.requestCurrent(requestTokens, documentURI, id, version) {
			return
		}
		set, err := DecodeTokens(result.Data, tokenTypes, tokenMods, snapshot, baseLine, documentURI, version)
		if err != nil {
			s.requestFailed(requestTokens, documentURI, id, err)
			return
		}
		s.emit(Event{Kind: EventTokens, URI: documentURI, Version: version, Tokens: set})
	}()
}

// RequestCompletion asks for completion items at pos.
func (s *Service) RequestCompletion(buffer *editor.Buffer, baseLine int, pos editor.Pos, manual bool, trigger, prefix string) {
	if buffer == nil || buffer.Path == "" {
		return
	}
	documentURI := URIForPath(buffer.Path)
	version := buffer.Version
	position := PositionFor(buffer, baseLine, pos)
	id, c, parent, ok := s.beginRequest(requestCompletion, documentURI)
	if !ok {
		return
	}
	completionContext := protocol.CompletionContext{TriggerKind: protocol.CompletionTriggerKindInvoked}
	if !manual && (trigger == "." || trigger == ":" || trigger == "(" || trigger == "@" || trigger == "#") {
		completionContext.TriggerKind = protocol.CompletionTriggerKindTriggerCharacter
		completionContext.TriggerCharacter = &trigger
	}
	go func() {
		ctx, cancel := context.WithTimeout(parent, requestTimeoutFeature)
		defer cancel()
		if !s.waitDocument(ctx, documentURI, version) {
			return
		}
		result, err := c.server.Completion(ctx, &protocol.CompletionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(documentURI)},
				Position:     protocol.Position{Line: uint32(position.Line), Character: uint32(position.Character)},
			},
			Context: completionContext,
		})
		if err != nil {
			s.requestFailed(requestCompletion, documentURI, id, err)
			return
		}
		if !s.requestCurrent(requestCompletion, documentURI, id, version) {
			return
		}
		items := convertCompletionItems(result)
		s.emit(Event{
			Kind:      EventCompletion,
			URI:       documentURI,
			Version:   version,
			RequestID: id,
			Position:  position,
			Completion: &CompletionResult{
				Items:    items,
				Manual:   manual,
				Trigger:  trigger,
				Prefix:   prefix,
				Document: documentURI,
				Position: position,
			},
		})
	}()
}

// RequestHover asks for hover content at pos.
func (s *Service) RequestHover(buffer *editor.Buffer, baseLine int, pos editor.Pos) {
	if buffer == nil || buffer.Path == "" {
		return
	}
	documentURI := URIForPath(buffer.Path)
	version := buffer.Version
	position := PositionFor(buffer, baseLine, pos)
	id, c, parent, ok := s.beginRequest(requestHover, documentURI)
	if !ok {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(parent, requestTimeoutFeature)
		defer cancel()
		if !s.waitDocument(ctx, documentURI, version) {
			return
		}
		result, err := c.server.Hover(ctx, &protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(documentURI)},
				Position:     protocol.Position{Line: uint32(position.Line), Character: uint32(position.Character)},
			},
		})
		if err != nil {
			s.requestFailed(requestHover, documentURI, id, err)
			return
		}
		if !s.requestCurrent(requestHover, documentURI, id, version) {
			return
		}
		text := ""
		if result != nil {
			text = hoverText(result.Contents)
		}
		s.emit(Event{Kind: EventHover, URI: documentURI, Version: version, RequestID: id, Position: position, Hover: &Hover{Text: text}})
	}()
}

// RequestSignature asks for signature help at pos.
func (s *Service) RequestSignature(buffer *editor.Buffer, baseLine int, pos editor.Pos, retrigger bool) {
	if buffer == nil || buffer.Path == "" {
		return
	}
	documentURI := URIForPath(buffer.Path)
	version := buffer.Version
	position := PositionFor(buffer, baseLine, pos)
	id, c, parent, ok := s.beginRequest(requestSignature, documentURI)
	if !ok {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(parent, requestTimeoutFeature)
		defer cancel()
		if !s.waitDocument(ctx, documentURI, version) {
			return
		}
		result, err := c.server.SignatureHelp(ctx, &protocol.SignatureHelpParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(documentURI)},
				Position:     protocol.Position{Line: uint32(position.Line), Character: uint32(position.Character)},
			},
			Context: protocol.SignatureHelpContext{
				TriggerKind: protocol.SignatureHelpTriggerKindInvoked,
				IsRetrigger: retrigger,
			},
		})
		if err != nil {
			s.requestFailed(requestSignature, documentURI, id, err)
			return
		}
		if !s.requestCurrent(requestSignature, documentURI, id, version) {
			return
		}
		s.emit(Event{Kind: EventSignature, URI: documentURI, Version: version, RequestID: id, Position: position, Signature: convertSignature(result)})
	}()
}

// RequestDefinition asks for definition locations at pos.
func (s *Service) RequestDefinition(buffer *editor.Buffer, baseLine int, pos editor.Pos) {
	if buffer == nil || buffer.Path == "" {
		return
	}
	documentURI := URIForPath(buffer.Path)
	version := buffer.Version
	position := PositionFor(buffer, baseLine, pos)
	id, c, parent, ok := s.beginRequest(requestDefinition, documentURI)
	if !ok {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(parent, requestTimeoutSlow)
		defer cancel()
		if !s.waitDocument(ctx, documentURI, version) {
			return
		}
		result, err := c.server.Definition(ctx, &protocol.DefinitionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(documentURI)},
				Position:     protocol.Position{Line: uint32(position.Line), Character: uint32(position.Character)},
			},
		})
		if err != nil {
			s.requestFailed(requestDefinition, documentURI, id, err)
			return
		}
		if !s.requestCurrent(requestDefinition, documentURI, id, version) {
			return
		}
		s.emit(Event{Kind: EventDefinition, URI: documentURI, Version: version, RequestID: id, Position: position, Locations: convertDefinition(result)})
	}()
}

type cartInfoWire struct {
	RootURI           string `json:"rootUri"`
	Tokens            int    `json:"tokens"`
	TokenLimit        int    `json:"tokenLimit"`
	Chars             int    `json:"chars"`
	CharLimit         int    `json:"charLimit"`
	Compressed        int    `json:"compressed"`
	CompressedLimit   int    `json:"compressedLimit"`
	CompressedWarning int    `json:"compressedWarning"`
	CompressedStatus  string `json:"compressedStatus"`
}

// RequestCartInfo asks for cart budget data. cart-not-found is reported as an
// unavailable result rather than an error toast.
func (s *Service) RequestCartInfo(documentURI string) {
	if documentURI == "" {
		return
	}
	id, c, parent, ok := s.beginRequest(requestCartInfo, documentURI)
	if !ok {
		return
	}
	version, _ := s.DocumentVersion(documentURI)
	go func() {
		ctx, cancel := context.WithTimeout(parent, requestTimeoutSlow)
		defer cancel()
		if version > 0 && !s.waitDocument(ctx, documentURI, version) {
			return
		}
		var wire cartInfoWire
		params := map[string]any{"textDocument": map[string]any{"uri": documentURI}}
		_, err := c.conn.Call(ctx, "pico8/cartInfo", params, &wire)
		if err != nil {
			if isCartNotFound(err) {
				s.emit(Event{Kind: EventCartInfo, URI: documentURI})
				return
			}
			s.requestFailed(requestCartInfo, documentURI, id, err)
			return
		}
		if !s.requestCurrent(requestCartInfo, documentURI, id, -1) {
			return
		}
		info := CartInfo(wire)
		s.emit(Event{Kind: EventCartInfo, URI: documentURI, CartInfo: &info})
	}()
}

func isCartNotFound(err error) bool {
	var rpcErr *jsonrpc2.Error
	if errors.As(err, &rpcErr) {
		return rpcErr.Code == -32001 || strings.HasPrefix(rpcErr.Message, "cart-not-found")
	}
	return strings.HasPrefix(err.Error(), "-32001")
}

type glyphsWire struct {
	Glyphs []struct {
		Name        string   `json:"name"`
		Text        string   `json:"text"`
		Byte        int      `json:"byte"`
		Aliases     []string `json:"aliases"`
		Category    string   `json:"category"`
		Description string   `json:"description"`
		Context     string   `json:"context"`
		Custom      bool     `json:"custom"`
	} `json:"glyphs"`
}

// RequestGlyphs asks for the P8SCII catalog used by the M6 picker.
func (s *Service) RequestGlyphs() {
	id, c, parent, ok := s.beginRequest(requestGlyphs, "")
	if !ok {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(parent, requestTimeoutSlow)
		defer cancel()
		var wire glyphsWire
		_, err := c.conn.Call(ctx, "pico8/glyphs", map[string]any{}, &wire)
		if err != nil {
			s.requestFailed(requestGlyphs, "", id, err)
			return
		}
		if !s.requestCurrent(requestGlyphs, "", id, -1) {
			return
		}
		glyphs := make([]Glyph, 0, len(wire.Glyphs))
		for _, entry := range wire.Glyphs {
			glyphs = append(glyphs, Glyph{
				Name:        entry.Name,
				Text:        entry.Text,
				Byte:        entry.Byte,
				Aliases:     append([]string(nil), entry.Aliases...),
				Category:    entry.Category,
				Description: entry.Description,
				Context:     entry.Context,
				Custom:      entry.Custom,
			})
		}
		s.emit(Event{Kind: EventGlyphs, Glyphs: glyphs})
	}()
}

func convertCompletionItems(result protocol.CompletionResult) []CompletionItem {
	var items []protocol.CompletionItem
	switch value := result.(type) {
	case protocol.CompletionItemSlice:
		items = value
	case *protocol.CompletionList:
		if value != nil {
			items = value.Items
		}
	}
	out := make([]CompletionItem, 0, len(items))
	for _, item := range items {
		converted := CompletionItem{
			Label:         item.Label,
			Detail:        optionalString(item.Detail),
			Documentation: plainText(item.Documentation),
			InsertText:    optionalString(item.InsertText),
		}
		if edit := convertCompletionEdit(item.TextEdit); edit != nil {
			converted.TextEdit = edit
		}
		for _, additional := range item.AdditionalTextEdits {
			converted.Additional = append(converted.Additional, TextEdit{
				Range:   convertRange(additional.Range),
				NewText: additional.NewText,
			})
		}
		out = append(out, converted)
	}
	return out
}

func convertCompletionEdit(edit protocol.CompletionItemTextEdit) *TextEdit {
	switch value := edit.(type) {
	case *protocol.TextEdit:
		if value == nil {
			return nil
		}
		return &TextEdit{Range: convertRange(value.Range), NewText: value.NewText}
	case *protocol.InsertReplaceEdit:
		if value == nil {
			return nil
		}
		return &TextEdit{Range: convertRange(value.Replace), NewText: value.NewText}
	default:
		return nil
	}
}

func convertSignature(help *protocol.SignatureHelp) *SignatureHelp {
	if help == nil || len(help.Signatures) == 0 {
		return nil
	}
	out := &SignatureHelp{Signatures: make([]Signature, 0, len(help.Signatures))}
	if help.ActiveSignature != nil {
		out.ActiveSignature = int(*help.ActiveSignature)
	}
	if active, ok := help.ActiveParameter.Get(); ok {
		out.ActiveParameter = int(active)
	}
	for _, signature := range help.Signatures {
		converted := Signature{
			Label:         signature.Label,
			Documentation: plainText(signature.Documentation),
		}
		for _, parameter := range signature.Parameters {
			converted.Parameters = append(converted.Parameters, convertParameter(parameter, signature.Label))
		}
		if active, ok := signature.ActiveParameter.Get(); ok {
			converted.ActiveParam = int(active)
			converted.ActiveParamSet = true
		}
		out.Signatures = append(out.Signatures, converted)
	}
	return out
}

func convertParameter(parameter protocol.ParameterInformation, signatureLabel string) SignatureParameter {
	converted := SignatureParameter{Start: -1, End: -1, Document: plainText(parameter.Documentation)}
	switch label := parameter.Label.(type) {
	case protocol.String:
		converted.Label = string(label)
		if index := strings.Index(signatureLabel, converted.Label); index >= 0 {
			converted.Start = index
			converted.End = index + len(converted.Label)
		}
	case protocol.ParameterInformationLabelTuple:
		converted.Start = int(label[0])
		converted.End = int(label[1])
		converted.Label = sliceUTF16(signatureLabel, converted.Start, converted.End)
	}
	return converted
}

func convertDefinition(result protocol.DefinitionResult) []Location {
	switch value := result.(type) {
	case *protocol.Location:
		if value == nil {
			return nil
		}
		return []Location{{URI: value.URI.String(), Range: convertRange(value.Range)}}
	case protocol.LocationSlice:
		out := make([]Location, 0, len(value))
		for _, location := range value {
			out = append(out, Location{URI: location.URI.String(), Range: convertRange(location.Range)})
		}
		return out
	case protocol.DefinitionLinkSlice:
		out := make([]Location, 0, len(value))
		for _, link := range value {
			locationRange := link.TargetSelectionRange
			if locationRange.Start.Line == 0 && locationRange.Start.Character == 0 &&
				locationRange.End.Line == 0 && locationRange.End.Character == 0 {
				locationRange = link.TargetRange
			}
			out = append(out, Location{URI: link.TargetURI.String(), Range: convertRange(locationRange)})
		}
		return out
	default:
		return nil
	}
}

func cloneBuffer(source *editor.Buffer) *editor.Buffer {
	if source == nil {
		return nil
	}
	copy := *source
	copy.Lines = append([]string(nil), source.Lines...)
	return &copy
}

func sliceUTF16(text string, start, end int) string {
	runes := []rune(text)
	units := 0
	first := 0
	last := len(runes)
	for index, r := range runes {
		size := utf16Len(r)
		if units == start {
			first = index
		}
		units += size
		if units == end {
			last = index + 1
			break
		}
	}
	if first > last {
		return ""
	}
	return string(runes[first:last])
}

func utf16Len(r rune) int {
	if r > 0xffff {
		return 2
	}
	return 1
}
