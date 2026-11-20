package app

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/asimmons91/pico-editor/internal/cart"
	"github.com/asimmons91/pico-editor/internal/editor"
	"github.com/asimmons91/pico-editor/internal/input"
	"github.com/asimmons91/pico-editor/internal/lsp"
	"github.com/asimmons91/pico-editor/internal/vim"
)

// AttachLSP connects the session to a running or starting language service.
func (s *Session) AttachLSP(service *lsp.Service) {
	s.LSP = service
	s.LSPStatus = service.Status()
	if s.LSPTokens == nil {
		s.LSPTokens = make(map[string]lsp.TokenSet)
	}
	if s.LSPDiagnostics == nil {
		s.LSPDiagnostics = make(map[string][]lsp.Diagnostic)
	}
	if s.LSPCartInfo == nil {
		s.LSPCartInfo = make(map[string]lsp.CartInfo)
	}
	for _, buffer := range s.Buffers {
		s.ensureLSPDocument(buffer)
	}
	s.requestActiveCartInfo()
}

// PollLSP flushes document changes, applies async results, and runs debounced
// completion/signature requests. It runs on the Update goroutine.
func (s *Session) PollLSP(now time.Time) bool {
	if s.LSP == nil {
		return false
	}
	changed := s.LSP.Flush(now)
	for _, event := range s.LSP.Poll() {
		if s.applyLSPEvent(event) {
			changed = true
		}
	}
	if !s.completionDue.IsZero() && !now.Before(s.completionDue) {
		s.completionDue = time.Time{}
		if s.Vim.Mode == vim.ModeInsert {
			s.requestCompletion(s.completionManual, s.completionTrigger)
		}
	}
	if !s.signatureDue.IsZero() && !now.Before(s.signatureDue) {
		s.signatureDue = time.Time{}
		if s.Vim.Mode == vim.ModeInsert {
			s.requestSignature()
		}
	}
	return changed
}

func (s *Session) applyLSPEvent(event lsp.Event) bool {
	switch event.Kind {
	case lsp.EventStatus:
		s.LSPStatus = event.Status
		switch event.Status {
		case lsp.StatusOff:
			s.LSPTokens = make(map[string]lsp.TokenSet)
			s.LSPDiagnostics = make(map[string][]lsp.Diagnostic)
			s.LSPCartInfo = make(map[string]lsp.CartInfo)
			s.Completion = nil
			s.Signature = nil
			s.Hover = nil
		case lsp.StatusReady:
			s.LSPMessage = ""
			s.LSPMessageError = false
			for _, buffer := range s.Buffers {
				s.ensureLSPDocument(buffer)
				s.requestTokens(buffer)
			}
			s.requestActiveCartInfo()
			s.LSP.RequestGlyphs()
		}
		return true
	case lsp.EventFailure:
		s.LSPStatus = lsp.StatusFailed
		s.LSPTokens = make(map[string]lsp.TokenSet)
		s.LSPDiagnostics = make(map[string][]lsp.Diagnostic)
		s.LSPCartInfo = make(map[string]lsp.CartInfo)
		s.Completion = nil
		s.Signature = nil
		s.Hover = nil
		if event.Err != nil {
			s.LSPMessage = "pico-lsp: " + shortError(event.Err)
			s.LSPMessageError = true
		}
		return true
	case lsp.EventRequestError:
		if event.Err != nil {
			s.SetStatus("lsp: "+shortError(event.Err), true)
		}
		return true
	case lsp.EventMessage:
		s.SetStatus(event.Message, false)
		return true
	case lsp.EventDiagnostics:
		if len(event.Diagnostics) == 0 {
			delete(s.LSPDiagnostics, event.URI)
		} else {
			s.LSPDiagnostics[event.URI] = append([]lsp.Diagnostic(nil), event.Diagnostics...)
		}
		return true
	case lsp.EventTokens:
		s.LSPTokens[event.URI] = event.Tokens
		return true
	case lsp.EventCartInfo:
		if event.CartInfo == nil {
			delete(s.LSPCartInfo, event.URI)
		} else {
			s.LSPCartInfo[event.URI] = *event.CartInfo
		}
		return true
	case lsp.EventGlyphs:
		s.LSPGlyphs = append([]lsp.Glyph(nil), event.Glyphs...)
		return true
	case lsp.EventSemanticRefresh:
		for _, buffer := range s.Buffers {
			s.requestTokens(buffer)
		}
		return true
	case lsp.EventCompletion:
		return s.applyCompletionEvent(event)
	case lsp.EventHover:
		if !s.eventAtCursor(event) {
			return false
		}
		if event.Hover == nil || event.Hover.Text == "" {
			s.SetStatus("no hover information", false)
			return true
		}
		s.Hover = &HoverPopup{Text: event.Hover.Text}
		return true
	case lsp.EventSignature:
		if s.Vim.Mode != vim.ModeInsert || !s.eventAtCursor(event) {
			return false
		}
		if event.Signature == nil || len(event.Signature.Signatures) == 0 {
			s.Signature = nil
		} else {
			s.Signature = &SignaturePopup{Help: *event.Signature}
		}
		return true
	case lsp.EventDefinition:
		if !s.eventAtCursor(event) {
			return false
		}
		return s.applyDefinitionEvent(event)
	default:
		return false
	}
}

func (s *Session) applyCompletionEvent(event lsp.Event) bool {
	if s.Vim.Mode != vim.ModeInsert {
		return false
	}
	buffer := s.CurrentBuffer()
	if buffer == nil || event.Completion == nil || s.activeURI() != event.URI {
		return false
	}
	ed := buffer.Editor
	baseLine := s.baseLine(buffer)
	if lsp.PositionFor(ed.Buffer, baseLine, ed.Cursor.Pos) != event.Position {
		return false
	}
	result := event.Completion
	if len(result.Items) == 0 {
		s.Completion = nil
		if result.Manual {
			s.requestSignature()
		}
		return true
	}
	items := result.Items
	if len(items) > 10 {
		items = items[:10]
	}
	s.Completion = &CompletionPopup{
		Items:    items,
		Selected: 0,
		Manual:   result.Manual,
		Trigger:  result.Trigger,
		Prefix:   result.Prefix,
		Anchor:   event.Position,
	}
	return true
}

func (s *Session) eventAtCursor(event lsp.Event) bool {
	buffer := s.CurrentBuffer()
	if buffer == nil {
		return false
	}
	return lsp.PositionFor(buffer.Editor.Buffer, s.baseLine(buffer), buffer.Editor.Cursor.Pos) == event.Position
}

func (s *Session) applyDefinitionEvent(event lsp.Event) bool {
	if len(event.Locations) == 0 {
		s.SetStatus("no definition", true)
		return true
	}
	if s.jumpToLocation(event.Locations[0]) {
		return true
	}
	return true
}

// afterInput syncs the edited document and schedules LSP work after Vim has
// handled an event.
func (s *Session) afterInput(ev input.Event) {
	if s.LSP == nil {
		return
	}
	buffer := s.CurrentBuffer()
	if buffer == nil {
		return
	}
	s.syncLSPDocument(buffer)
	if s.Vim.Mode != vim.ModeInsert {
		s.Completion = nil
		s.Signature = nil
		s.completionDue = time.Time{}
		s.signatureDue = time.Time{}
		return
	}
	now := time.Now()
	switch ev.Kind {
	case input.KindText:
		if len(ev.Runes) == 0 {
			return
		}
		last := ev.Runes[len(ev.Runes)-1]
		if isCompletionTrigger(last) {
			s.scheduleCompletion(now, string(last), false)
		} else if s.Completion != nil {
			s.Completion = nil
		}
		if last == '(' || last == ',' {
			s.signatureDue = now.Add(100 * time.Millisecond)
			s.signatureRetrigger = s.Signature != nil
		}
	case input.KindKeyDown:
		switch ev.Key {
		case input.KeyLeft, input.KeyRight, input.KeyUp, input.KeyDown, input.KeyHome, input.KeyEnd:
			if s.Completion != nil {
				s.scheduleCompletion(now, "", false)
			}
			if s.Signature != nil {
				s.signatureDue = now.Add(100 * time.Millisecond)
				s.signatureRetrigger = true
			}
		case input.KeyBackspace, input.KeyDelete:
			if s.Completion != nil {
				s.scheduleCompletion(now, "", false)
			}
			if s.Signature != nil {
				s.signatureDue = now.Add(100 * time.Millisecond)
				s.signatureRetrigger = true
			}
		}
	}
}

func (s *Session) scheduleCompletion(now time.Time, trigger string, manual bool) {
	s.completionDue = now.Add(100 * time.Millisecond)
	s.completionTrigger = trigger
	s.completionManual = manual
}

func (s *Session) requestCompletion(manual bool, trigger string) {
	buffer := s.CurrentBuffer()
	if buffer == nil || s.LSP == nil {
		return
	}
	ed := buffer.Editor
	s.LSP.RequestCompletion(ed.Buffer, s.baseLine(buffer), ed.Cursor.Pos, manual, trigger, s.completionPrefix(ed))
}

func (s *Session) requestSignature() {
	buffer := s.CurrentBuffer()
	if buffer == nil || s.LSP == nil {
		return
	}
	ed := buffer.Editor
	s.LSP.RequestSignature(ed.Buffer, s.baseLine(buffer), ed.Cursor.Pos, s.signatureRetrigger)
	s.signatureRetrigger = false
}

func (s *Session) completionPrefix(ed *editor.Editor) string {
	if ed == nil {
		return ""
	}
	pos := ed.Cursor.Pos
	if pos.Line < 0 || pos.Line >= len(ed.Buffer.Lines) {
		return ""
	}
	line := []rune(ed.Buffer.Lines[pos.Line])
	if pos.Rune < 0 || pos.Rune > len(line) {
		return ""
	}
	start := pos.Rune
	for start > 0 && isIdentifierRune(line[start-1]) {
		start--
	}
	return string(line[start:pos.Rune])
}

func isCompletionTrigger(r rune) bool {
	return isIdentifierRune(r) || r == '.' || r == ':' || r == '('
}

func isIdentifierRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// handleLSPInput consumes keys owned by open LSP popups. It returns true when
// the event must not reach Vim.
func (s *Session) handleLSPInput(ev input.Event) bool {
	if s.Hover != nil {
		s.Hover = nil
		if ev.Kind == input.KindKeyDown && ev.Key == input.KeyEsc {
			return true
		}
	}
	if s.Vim.Mode != vim.ModeInsert {
		return false
	}
	if s.Completion != nil && ev.Kind == input.KindText {
		// The popup's text edits refer to the document before this keystroke.
		// Let Vim apply the text; afterInput requests a fresh list instead of
		// accepting stale ranges.
		s.Completion = nil
		return false
	}
	if ev.Kind != input.KindKeyDown {
		return false
	}
	ctrl := ev.Mods&input.ModCtrl != 0
	shift := ev.Mods&input.ModShift != 0
	if ctrl && ev.Key == input.KeySpace {
		s.Completion = nil
		s.requestCompletion(true, "")
		return true
	}
	if s.Completion != nil {
		switch {
		case ctrl && ev.Key == input.KeyN && shift:
			s.moveCompletion(-1)
			return true
		case ctrl && ev.Key == input.KeyN:
			s.moveCompletion(1)
			return true
		case ev.Key == input.KeyEnter, ev.Key == input.KeyTab:
			if ev.Key == input.KeyTab && ev.Mods != 0 {
				return false
			}
			s.acceptCompletion()
			return true
		case ev.Key == input.KeyEsc:
			s.Completion = nil
			return true
		}
		switch ev.Key {
		case input.KeyLeft, input.KeyRight, input.KeyUp, input.KeyDown, input.KeyHome, input.KeyEnd, input.KeyBackspace, input.KeyDelete:
			s.Completion = nil
			s.scheduleCompletion(time.Now(), "", false)
			return false
		}
	}
	if s.Signature != nil && ev.Key == input.KeyEsc {
		s.Signature = nil
		return true
	}
	return false
}

// RequestHover asks for hover at the cursor.
func (s *Session) RequestHover() {
	buffer := s.CurrentBuffer()
	if buffer == nil || s.LSP == nil {
		return
	}
	ed := buffer.Editor
	s.LSP.RequestHover(ed.Buffer, s.baseLine(buffer), ed.Cursor.Pos)
}

// RequestDefinition asks for a definition at the cursor.
func (s *Session) RequestDefinition() {
	buffer := s.CurrentBuffer()
	if buffer == nil || s.LSP == nil {
		return
	}
	ed := buffer.Editor
	s.LSP.RequestDefinition(ed.Buffer, s.baseLine(buffer), ed.Cursor.Pos)
}

// ShowDiagnostics opens the :diag list for the active document.
func (s *Session) ShowDiagnostics() bool {
	documentURI := s.activeURI()
	diagnostics := s.LSPDiagnostics[documentURI]
	if len(diagnostics) == 0 {
		s.SetStatus("no diagnostics", false)
		return true
	}
	sorted := append([]lsp.Diagnostic(nil), diagnostics...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Range.Start.Line != sorted[j].Range.Start.Line {
			return sorted[i].Range.Start.Line < sorted[j].Range.Start.Line
		}
		return sorted[i].Range.Start.Character < sorted[j].Range.Start.Character
	})
	s.DiagList = &DiagnosticList{}
	for _, diagnostic := range sorted {
		s.DiagList.Items = append(s.DiagList.Items, DiagnosticItem{
			URI:      diagnostic.URI,
			Range:    diagnostic.Range,
			Severity: diagnostic.Severity,
			Message:  diagnostic.Message,
		})
	}
	return true
}

func (s *Session) handleDiagnosticList(ev input.Event) bool {
	list := s.DiagList
	if list == nil {
		return false
	}
	if ev.Kind == input.KindKeyDown {
		switch ev.Key {
		case input.KeyEsc, input.KeyQ:
			s.DiagList = nil
			return true
		case input.KeyUp, input.KeyK:
			if list.Selected > 0 {
				list.Selected--
			}
			return true
		case input.KeyDown, input.KeyJ:
			if list.Selected+1 < len(list.Items) {
				list.Selected++
			}
			return true
		case input.KeyPageUp:
			list.Selected = max(0, list.Selected-10)
			return true
		case input.KeyPageDown:
			list.Selected = min(max(0, len(list.Items)-1), list.Selected+10)
			return true
		case input.KeyEnter:
			item := list.Items[list.Selected]
			s.DiagList = nil
			s.jumpToLocation(lsp.Location{URI: item.URI, Range: item.Range})
			return true
		}
	}
	return true
}

// JumpToPath applies a cross-file jump requested by the Vim jump list.
func (s *Session) JumpToPath(path string, pos editor.Pos) bool {
	if path == "" {
		if ed := s.CurrentEditor(); ed != nil {
			ed.SetCursor(pos)
		}
		return true
	}
	if !s.switchToPath(path) {
		return false
	}
	buffer := s.CurrentBuffer()
	if buffer == nil {
		return false
	}
	ed := buffer.Editor
	if buffer.Cart != nil {
		line := pos.Line
		if tab, ok := tabForLine(buffer.Cart.Tabs, line); ok {
			s.activateTab(buffer, tab.Index)
		}
	}
	ed.SetCursor(pos)
	ed.EnsureVisible()
	return true
}

func (s *Session) jumpToLocation(location lsp.Location) bool {
	path, ok := lsp.PathForURI(location.URI)
	if !ok {
		s.SetStatus("definition is outside the workspace", true)
		return false
	}
	if !s.withinWorkspace(path) {
		s.SetStatus("definition is outside the workspace", true)
		return false
	}
	if ed := s.CurrentEditor(); ed != nil {
		s.Vim.RecordJump(ed)
	}
	if !s.switchToPath(path) {
		return false
	}
	buffer := s.CurrentBuffer()
	if buffer == nil {
		return false
	}
	baseLine := s.baseLine(buffer)
	line := location.Range.Start.Line - baseLine
	if buffer.Cart != nil && len(buffer.Cart.Tabs) > 0 {
		tab, ok := tabForLine(buffer.Cart.Tabs, line)
		if ok {
			s.activateTab(buffer, tab.Index)
			buffer = s.CurrentBuffer()
			baseLine = s.baseLine(buffer)
		}
	}
	position, ok := lsp.EditorPos(buffer.Editor.Buffer, baseLine, location.Range.Start)
	if !ok {
		s.SetStatus("definition is outside the code view", true)
		return false
	}
	buffer.Editor.SetCursor(position)
	buffer.Editor.EnsureVisible()
	return true
}

func (s *Session) activateTab(buffer *SessionBuffer, index int) {
	if buffer == nil || buffer.Cart == nil || index < 0 || index >= len(buffer.Cart.Tabs) {
		return
	}
	s.saveActiveView(buffer)
	buffer.ActiveTab = index
	tab := buffer.Cart.Tabs[index]
	buffer.Editor.SetLineRange(tab.StartLine, tab.EndLine)
	restoreView(buffer.Editor, buffer.TabViews[index])
}

func tabForLine(tabs []cart.Tab, line int) (cart.Tab, bool) {
	for _, tab := range tabs {
		if line >= tab.StartLine && line < tab.EndLine {
			return tab, true
		}
	}
	return cart.Tab{}, false
}

func (s *Session) switchToPath(path string) bool {
	absolute, err := filepath.Abs(path)
	if err != nil {
		s.SetStatus(shortError(err), true)
		return false
	}
	absolute = filepath.Clean(absolute)
	current := s.CurrentBuffer()
	if current != nil && current.Editor.Buffer.Path != "" {
		if currentPath, err := filepath.Abs(current.Editor.Buffer.Path); err == nil && filepath.Clean(currentPath) == absolute {
			return true
		}
	}
	if !s.OpenPath(absolute, false) {
		return false
	}
	return true
}

func (s *Session) withinWorkspace(path string) bool {
	root := ""
	if s.Workspace != nil {
		root = s.Workspace.Root
	}
	if root == "" {
		return true
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (s *Session) activeURI() string {
	buffer := s.CurrentBuffer()
	if buffer == nil {
		return ""
	}
	return lsp.URIForPath(buffer.Editor.Buffer.Path)
}

func (s *Session) baseLine(buffer *SessionBuffer) int {
	if buffer == nil || buffer.Cart == nil {
		return 0
	}
	return buffer.Cart.BaseLine
}

func (s *Session) documentText(buffer *SessionBuffer) (string, error) {
	if buffer == nil {
		return "", fmt.Errorf("no buffer")
	}
	if buffer.Cart != nil {
		return buffer.Cart.FullText()
	}
	return buffer.Editor.Buffer.Text(), nil
}

func (s *Session) ensureLSPDocument(buffer *SessionBuffer) {
	if s.LSP == nil || buffer == nil || buffer.Editor.Buffer.Path == "" {
		return
	}
	if buffer.LSPOpened {
		s.syncLSPDocument(buffer)
		return
	}
	text, err := s.documentText(buffer)
	if err != nil {
		s.logger.Warn("lsp document text", "path", buffer.Editor.Buffer.Path, "error", err)
		s.SetStatus("lsp: cart text unavailable", true)
		return
	}
	language := "lua"
	if buffer.Cart != nil {
		language = "pico8"
	}
	buffer.LSPVersion = buffer.Editor.Buffer.Version
	buffer.LSPOpened = true
	s.LSP.DidOpen(lsp.URIForPath(buffer.Editor.Buffer.Path), language, buffer.Editor.Buffer.Version, text, buffer.Cart != nil)
}

func (s *Session) syncLSPDocument(buffer *SessionBuffer) {
	if s.LSP == nil || buffer == nil || buffer.Editor.Buffer.Path == "" {
		return
	}
	if !buffer.LSPOpened {
		s.ensureLSPDocument(buffer)
		return
	}
	if buffer.LSPVersion == buffer.Editor.Buffer.Version {
		return
	}
	text, err := s.documentText(buffer)
	if err != nil {
		s.logger.Warn("lsp document text", "path", buffer.Editor.Buffer.Path, "error", err)
		s.SetStatus("lsp: cart text unavailable", true)
		return
	}
	language := "lua"
	if buffer.Cart != nil {
		language = "pico8"
	}
	buffer.LSPVersion = buffer.Editor.Buffer.Version
	s.LSP.DidChange(lsp.URIForPath(buffer.Editor.Buffer.Path), language, buffer.Editor.Buffer.Version, text, buffer.Cart != nil)
}

func (s *Session) syncLSPCurrent() {
	s.syncLSPDocument(s.CurrentBuffer())
}

func (s *Session) closeLSPDocument(buffer *SessionBuffer) {
	if s.LSP == nil || buffer == nil || !buffer.LSPOpened {
		return
	}
	buffer.LSPOpened = false
	s.LSP.DidClose(lsp.URIForPath(buffer.Editor.Buffer.Path))
}

// LSPDidSave notifies the server after a successful write.
func (s *Session) LSPDidSave(buffer *editor.Buffer) {
	if s.LSP == nil || buffer == nil || buffer.Path == "" {
		return
	}
	for _, candidate := range s.Buffers {
		if candidate.Editor.Buffer != buffer {
			continue
		}
		s.LSP.DidSave(lsp.URIForPath(buffer.Path))
		return
	}
}

func (s *Session) reloadLSPDocument(buffer *SessionBuffer) {
	if s.LSP == nil || buffer == nil || buffer.Editor.Buffer.Path == "" {
		return
	}
	s.closeLSPDocument(buffer)
	s.ensureLSPDocument(buffer)
}

func (s *Session) requestTokens(buffer *SessionBuffer) {
	if s.LSP == nil || buffer == nil || buffer.Editor.Buffer.Path == "" {
		return
	}
	s.syncLSPDocument(buffer)
	s.LSP.RequestTokens(buffer.Editor.Buffer, s.baseLine(buffer), buffer.Cart != nil)
}

func (s *Session) requestActiveCartInfo() {
	buffer := s.CurrentBuffer()
	if s.LSP == nil || buffer == nil || buffer.Editor.Buffer.Path == "" {
		return
	}
	if s.LSP.Status() != lsp.StatusReady {
		return
	}
	if buffer.LSPCartAsked == buffer.Editor.Buffer.Version {
		return
	}
	buffer.LSPCartAsked = buffer.Editor.Buffer.Version
	s.LSP.RequestCartInfo(lsp.URIForPath(buffer.Editor.Buffer.Path))
}

// ActiveDocumentURI exposes the current document URI for status and tests.
func (s *Session) ActiveDocumentURI() string {
	return s.activeURI()
}

// LSPDiagnosticsFor returns a copy of a document's diagnostics.
func (s *Session) LSPDiagnosticsFor(documentURI string) []lsp.Diagnostic {
	return append([]lsp.Diagnostic(nil), s.LSPDiagnostics[documentURI]...)
}

// DiagnosticCounts returns error and warning counts for a document URI.
func (s *Session) DiagnosticCounts(documentURI string) (errors, warnings int) {
	for _, diagnostic := range s.LSPDiagnostics[documentURI] {
		switch diagnostic.Severity {
		case lsp.SeverityError:
			errors++
		case lsp.SeverityWarning:
			warnings++
		}
	}
	return errors, warnings
}
