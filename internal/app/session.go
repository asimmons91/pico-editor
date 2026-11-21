package app

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/asimmons91/pico-editor/internal/cart"
	"github.com/asimmons91/pico-editor/internal/config"
	"github.com/asimmons91/pico-editor/internal/editor"
	"github.com/asimmons91/pico-editor/internal/font"
	"github.com/asimmons91/pico-editor/internal/fsutil"
	"github.com/asimmons91/pico-editor/internal/input"
	"github.com/asimmons91/pico-editor/internal/lsp"
	"github.com/asimmons91/pico-editor/internal/render"
	"github.com/asimmons91/pico-editor/internal/vim"
	"github.com/asimmons91/pico-editor/internal/workspace"
)

const (
	statusRow  = GridRows - 1
	fileTabRow = 0
	cartTabRow = 1
)

// Session is the Vim-backed editor screen. It owns open buffers, the file
// tree, dialogs, the modal state shared by buffers, and the help overlay.
type Session struct {
	Buffers []*SessionBuffer
	Current int
	Vim     *vim.Vim
	P8      *input.P8Input
	Help    *HelpOverlay
	Tree    TreeState
	Dialog  *Dialog

	Workspace *workspace.Workspace

	// Settings is immutable for one configuration generation.
	Settings config.Settings

	LSP *lsp.Service

	Status      string
	StatusError bool

	// DisplayMessage is the active toast for the status row. The app sets it
	// from NotificationCenter before each draw; it replaces rightmost segments.
	DisplayMessage      string
	DisplayMessageError bool

	notify         func(message string, severity NotificationSeverity, toast bool)
	forgetRecovery func(path string)

	LSPStatus       lsp.Status
	LSPMessage      string
	LSPMessageError bool
	LSPTokens       map[string]lsp.TokenSet
	LSPDiagnostics  map[string][]lsp.Diagnostic
	LSPCartInfo     map[string]lsp.CartInfo
	LSPGlyphs       []lsp.Glyph

	Completion  *CompletionPopup
	Hover       *HoverPopup
	Signature   *SignaturePopup
	DiagList    *DiagnosticList
	GlyphPicker *GlyphPicker

	logger         *slog.Logger
	quit           bool
	suppressText   rune
	pendingTabStep int
	pendingTabBuf  *SessionBuffer
	pendingTabAt   int

	completionDue      time.Time
	completionTrigger  string
	completionManual   bool
	signatureDue       time.Time
	signatureRetrigger bool
	cartInfoDue        time.Time
	tokenDue           time.Time
	glyphPickerPending bool
}

// SessionBuffer is one open file. Cart is non-nil for .p8 buffers; its Code
// buffer is Editor.Buffer.
type SessionBuffer struct {
	Editor        *editor.Editor
	Cart          *cart.Document
	ActiveTab     int
	TabViews      []cart.ViewState
	Owner         string
	SyncedVersion int
	LSPVersion    int
	LSPOpened     bool
	LSPCartAsked  int
}

// NewSession opens path or creates an empty unnamed buffer. The workspace root
// is the file's parent, or the current directory when path is empty.
func NewSession(path string, logger *slog.Logger) (*Session, error) {
	root, err := os.Getwd()
	if err != nil {
		root = "."
	}
	if path != "" {
		absolute, absErr := filepath.Abs(path)
		if absErr != nil {
			return nil, absErr
		}
		root = filepath.Dir(absolute)
	}
	return NewSessionAt(root, path, logger)
}

// CurrentBuffer returns the active session buffer, or nil if there is none.
func (s *Session) CurrentBuffer() *SessionBuffer {
	if s.Current < 0 || s.Current >= len(s.Buffers) {
		return nil
	}
	return s.Buffers[s.Current]
}

// EndBatch finishes frame-local input suppression and applies any tab switch
// deferred so this batch's text events reach the original insert session.
func (s *Session) EndBatch() (changed bool) {
	s.Vim.EndBatch()
	s.suppressText = 0
	if s.pendingTabStep == 0 {
		return false
	}
	step := s.pendingTabStep
	buffer := s.pendingTabBuf
	start := s.pendingTabAt
	s.pendingTabStep = 0
	s.pendingTabBuf = nil
	if buffer == nil || s.CurrentBuffer() != buffer || buffer.ActiveTab != start {
		return false
	}
	return s.SwitchTab(step)
}

// QueueTabSwitch defers Ctrl+Tab until the current input batch is delivered.
func (s *Session) QueueTabSwitch(delta int) {
	buffer := s.CurrentBuffer()
	if buffer == nil {
		return
	}
	if s.pendingTabStep == 0 || s.pendingTabBuf != buffer || s.pendingTabAt != buffer.ActiveTab {
		s.pendingTabBuf = buffer
		s.pendingTabAt = buffer.ActiveTab
		s.pendingTabStep = 0
	}
	s.pendingTabStep += delta
}

// CurrentEditor returns the active editor, or nil if there is no buffer.
func (s *Session) CurrentEditor() *editor.Editor {
	if s.Current < 0 || s.Current >= len(s.Buffers) {
		return nil
	}
	return s.Buffers[s.Current].Editor
}

// HandleEvent routes one input event to a dialog, the file tree, the help
// overlay, or Vim.
func (s *Session) HandleEvent(ev input.Event) (quit, changed bool, actions []vim.Action) {
	if ev.Kind == input.KindText && s.suppressText != 0 {
		if len(ev.Runes) > 0 && unicode.ToLower(ev.Runes[0]) == s.suppressText {
			s.suppressText = 0
			ev.Runes = ev.Runes[1:]
			if len(ev.Runes) == 0 {
				return false, false, nil
			}
		} else {
			s.suppressText = 0
		}
	}
	if s.glyphPickerPending && (ev.Kind == input.KindKeyDown || ev.Kind == input.KindText) {
		// Any input after "loading glyph catalog" cancels the deferred open.
		s.glyphPickerPending = false
	}
	if s.Dialog != nil {
		s.clearP8Pending()
		return false, s.handleDialog(ev), nil
	}
	if s.GlyphPicker != nil {
		s.clearP8Pending()
		return false, s.handleGlyphPicker(ev), nil
	}
	if s.Help != nil {
		s.clearP8Pending()
		return false, s.handleHelp(ev), nil
	}
	if s.DiagList != nil {
		s.clearP8Pending()
		return false, s.handleDiagnosticList(ev), nil
	}
	if s.Tree.Visible && s.Tree.Focused {
		s.clearP8Pending()
		return false, s.handleTree(ev), nil
	}
	if handled := s.handleLSPInput(ev); handled {
		s.clearP8Pending()
		return false, true, nil
	}
	if s.Vim.Mode == vim.ModeInsert {
		if s.P8 != nil {
			switch ev.Kind {
			case input.KindKeyDown:
				handled := false
				if ev.Repeat {
					handled = s.P8.HandleRepeat(ev.Key, ev.Mods)
				} else {
					handled = s.P8.HandleKey(ev.Key, ev.Mods)
				}
				if handled {
					return false, true, nil
				}
			case input.KindText:
				runes, handled := s.P8.TransformText(ev.Runes, ev.Mods)
				if handled {
					return false, true, nil
				}
				ev.Runes = runes
			}
		}
	} else if s.P8 != nil {
		s.P8.Reset()
	}
	if ev.Kind == input.KindText && s.blocksTabSeparator(ev) {
		s.SetStatus("cannot create a -->8 tab separator in insert mode", true)
		return false, true, nil
	}
	ed := s.CurrentEditor()
	if ed == nil {
		return false, false, nil
	}
	result := s.Vim.Handle(ev, ed)
	s.applyVimResult(result)
	s.syncCurrentCart()
	s.afterInput(ev)
	if s.P8 != nil && s.Vim.Mode != vim.ModeInsert {
		s.P8.Reset()
	}
	return false, true, result.Actions
}

// blocksTabSeparator prevents insert-mode text from turning a visible code
// line into a hidden -->8 separator. Creating tabs by typing a separator is not
// part of the M4 tab model; rejecting the keystroke avoids silently moving the
// cursor or reordering text across tabs.
func (s *Session) blocksTabSeparator(ev input.Event) bool {
	if s.Vim.Mode != vim.ModeInsert || len(ev.Runes) == 0 {
		return false
	}
	buffer := s.CurrentBuffer()
	if buffer == nil || buffer.Cart == nil {
		return false
	}
	ed := buffer.Editor
	pos := ed.Cursor.Pos
	line := ""
	runes := []rune(nil)
	if !ed.WindowEmpty() {
		if pos.Line < 0 || pos.Line >= len(ed.Buffer.Lines) {
			return false
		}
		line = ed.Buffer.Lines[pos.Line]
		runes = []rune(line)
		if pos.Rune < 0 || pos.Rune > len(runes) {
			return false
		}
	}
	inserted := string(ev.Runes)
	if strings.Contains(inserted, "\n-->8") {
		return true
	}
	result := inserted
	if len(runes) > 0 {
		result = string(runes[:pos.Rune]) + inserted + string(runes[pos.Rune:])
	}
	for _, candidate := range strings.Split(result, "\n") {
		if strings.HasPrefix(candidate, "-->8") {
			return true
		}
	}
	return false
}

func (s *Session) clearP8Pending() {
	if s.P8 != nil {
		s.P8.ClearPending()
	}
}

func (s *Session) forgetPath(path string) {
	if s.forgetRecovery != nil {
		s.forgetRecovery(path)
	}
}

func (s *Session) applyVimResult(result vim.Result) (quit bool) {
	if result.Notify != "" {
		s.SetStatus(result.Notify, result.Error)
	}
	return false
}

// CompleteClipboardRead applies an asynchronous clipboard read.
func (s *Session) CompleteClipboardRead(text string, err error) (changed bool, actions []vim.Action) {
	ed := s.CurrentEditor()
	if ed == nil {
		return false, nil
	}
	text = strings.TrimPrefix(text, "\ufeff")
	if target := s.Vim.PendingPasteTarget(); target != nil && s.bufferForEditor(target) == nil {
		s.Vim.CancelPendingPaste()
		s.SetStatus("clipboard paste cancelled: buffer closed", true)
		return true, nil
	}
	s.clearLSPTransients()
	result := s.Vim.CompleteClipboardRead(text, err, ed)
	s.applyVimResult(result)
	if err == nil {
		if count := unknownGlyphCount(text); count > 0 {
			s.SetStatus(fmt.Sprintf("pasted %d unknown rune(s); preserved", count), true)
		}
	}
	s.syncAllBuffers()
	s.tokenDue = time.Now().Add(120 * time.Millisecond)
	s.cartInfoDue = time.Now().Add(300 * time.Millisecond)
	return true, result.Actions
}

func (s *Session) bufferForEditor(ed *editor.Editor) *SessionBuffer {
	for _, buffer := range s.Buffers {
		if buffer.Editor == ed {
			return buffer
		}
	}
	return nil
}

func (s *Session) syncAllBuffers() {
	for _, buffer := range s.Buffers {
		s.syncCart(buffer)
		s.syncLSPDocument(buffer)
	}
}

func unknownGlyphCount(text string) int {
	count := 0
	for cluster := range font.Clusters(text) {
		if !cluster.Known {
			count++
		}
	}
	return count
}

// ClipboardPaste requests a Ctrl+Shift+V paste in insert or visual mode.
func (s *Session) ClipboardPaste() (bool, []vim.Action) {
	ed := s.CurrentEditor()
	if ed == nil || s.Vim == nil {
		return false, nil
	}
	result := s.Vim.ClipboardPaste(ed)
	s.applyVimResult(result)
	s.syncCurrentCart()
	return true, result.Actions
}

// VisualClipboardYank writes the visual selection to the system clipboard.
func (s *Session) VisualClipboardYank() (bool, []vim.Action) {
	ed := s.CurrentEditor()
	if ed == nil || s.Vim == nil {
		return false, nil
	}
	result := s.Vim.VisualClipboardYank(ed)
	s.applyVimResult(result)
	s.syncCurrentCart()
	return true, result.Actions
}

// CanDismissNotification reports whether Esc/Enter should dismiss a persistent
// error toast instead of reaching Vim or an overlay.
func (s *Session) CanDismissNotification() bool {
	if s.Dialog != nil || s.GlyphPicker != nil || s.Help != nil || s.DiagList != nil || s.Tree.Focused {
		return false
	}
	if s.Vim.Mode != vim.ModeNormal {
		return false
	}
	_, _, _, active := s.Vim.Prompt()
	return !active
}

// SetStatus replaces the status notification and forwards it to the toast
// center when one is attached.
func (s *Session) SetStatus(message string, isError bool) {
	s.Status = message
	s.StatusError = isError
	if message != "" && s.notify != nil {
		severity := NotifyInfo
		if isError {
			severity = NotifyError
		}
		s.notify(message, severity, true)
	}
}

// NotifyRoutine records a message in history without a toast. LSP request
// noise uses this path so routine traffic never interrupts editing.
func (s *Session) NotifyRoutine(message string, severity NotificationSeverity) {
	s.Status = message
	s.StatusError = severity == NotifyError
	if message != "" && s.notify != nil {
		s.notify(message, severity, false)
	}
}

// ClearStatus removes the status notification.
func (s *Session) ClearStatus() {
	s.Status = ""
	s.StatusError = false
}

// RequestQuit closes the current buffer, or exits when it is the last one.
// force discards unsaved changes.
func (s *Session) RequestQuit(force bool) (quit, changed bool) {
	ed := s.CurrentEditor()
	if ed == nil {
		s.quit = true
		return true, true
	}
	return s.CloseBuffer(ed.Buffer, force)
}

// CloseBuffer closes the buffer identified by pointer. Async save completion
// uses this so a save-quit can never discard a buffer the user switched to
// while the write was in flight.
func (s *Session) CloseBuffer(buffer *editor.Buffer, force bool) (quit, changed bool) {
	index := -1
	for i, candidate := range s.Buffers {
		if candidate.Editor.Buffer == buffer {
			index = i
			break
		}
	}
	if index < 0 {
		return false, false
	}
	if buffer.Dirty() && !force {
		s.SetStatus("E37: No write since last change (add ! to override)", true)
		return false, true
	}
	if len(s.Buffers) == 1 {
		s.closeLSPDocument(s.Buffers[0])
		s.quit = true
		return true, true
	}
	s.closeLSPDocument(s.Buffers[index])
	s.Buffers = append(s.Buffers[:index], s.Buffers[index+1:]...)
	if index < s.Current {
		s.Current--
	}
	if s.Current >= len(s.Buffers) {
		s.Current = len(s.Buffers) - 1
	}
	s.prepareBuffer(s.CurrentBuffer())
	return false, true
}

// CanQuitAll reports whether every buffer is clean.
func (s *Session) CanQuitAll() bool {
	for _, buffer := range s.Buffers {
		if buffer.Editor.Buffer.Dirty() {
			return false
		}
	}
	return true
}

// Quitting reports that the session asked to terminate.
func (s *Session) Quitting() bool {
	return s.quit
}

// OpenPath implements :e. An empty path reloads the current file.
func (s *Session) OpenPath(path string, force bool) (changed bool) {
	ed := s.CurrentEditor()
	if ed == nil {
		return false
	}
	s.endInsertMode()
	if path == "" {
		if ed.Buffer.Dirty() && !force {
			s.SetStatus("E37: No write since last change (add ! to override)", true)
			return false
		}
		current := s.CurrentBuffer()
		reloadPath := ed.Buffer.Path
		if current != nil && current.Cart != nil {
			active := current.ActiveTab
			fresh, err := s.openSessionBuffer(current.Cart.Path)
			if err != nil {
				s.ShowErrorDetail("Reload failed", "reload", current.Cart.Path, err)
				return false
			}
			if active < len(fresh.Cart.Tabs) {
				fresh.ActiveTab = active
			}
			s.closeLSPDocument(current)
			s.Buffers[s.Current] = fresh
			s.prepareBuffer(fresh)
		} else {
			ed.EndGroup()
			if err := ed.Reload(); err != nil {
				s.SetStatus(shortError(err), true)
				return false
			}
			ed.Buffer.SetWorkspaceRoot(s.Workspace.Root)
			s.reloadLSPDocument(s.CurrentBuffer())
			s.prepareBuffer(s.CurrentBuffer())
			s.refreshOwner(s.CurrentBuffer())
		}
		s.forgetPath(reloadPath)
		s.SetStatus("reloaded", false)
		return true
	}

	root := s.Workspace.Root
	if root == "" {
		root, _ = os.Getwd()
	}
	target := path
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	absolute, err := filepath.Abs(target)
	if err != nil {
		s.SetStatus(shortError(err), true)
		return false
	}
	absolute = filepath.Clean(absolute)
	for index, buffer := range s.Buffers {
		bufferPath, err := filepath.Abs(buffer.Editor.Buffer.Path)
		if err != nil || bufferPath != absolute {
			continue
		}
		if force {
			fresh, err := s.openSessionBuffer(absolute)
			if err != nil {
				s.ShowErrorDetail("Open failed", "open", absolute, err)
				return false
			}
			s.closeLSPDocument(s.Buffers[index])
			s.Buffers[index] = fresh
			s.forgetPath(absolute)
		}
		s.Current = index
		s.prepareBuffer(s.CurrentBuffer())
		return true
	}
	opened, err := s.openSessionBuffer(absolute)
	if err != nil {
		s.logger.Error("open", "path", absolute, "error", err)
		s.ShowErrorDetail("Open failed", "open", absolute, err)
		return false
	}
	if ed.Buffer.Path == "" && !ed.Buffer.Dirty() {
		// Reuse an empty unnamed buffer instead of leaving it in the list.
		s.closeLSPDocument(s.Buffers[s.Current])
		s.Buffers[s.Current] = opened
	} else if force && ed.Buffer.Dirty() {
		// :e! abandons the current buffer's changes rather than leaving a
		// second dirty buffer behind.
		s.forgetPath(ed.Buffer.Path)
		s.closeLSPDocument(s.Buffers[s.Current])
		s.Buffers[s.Current] = opened
	} else {
		s.Buffers = append(s.Buffers, opened)
		s.Current = len(s.Buffers) - 1
	}
	s.prepareBuffer(opened)
	s.SetStatus("", false)
	return true
}

// SwitchBuffer implements :bn, :bp, and :b N.
func (s *Session) SwitchBuffer(number, delta int) (changed bool) {
	if len(s.Buffers) == 0 {
		return false
	}
	s.endInsertMode()
	index := s.Current
	switch {
	case number > 0:
		index = number - 1
		if index < 0 || index >= len(s.Buffers) {
			s.SetStatus("E86: Buffer does not exist", true)
			return false
		}
	case delta != 0:
		if len(s.Buffers) == 1 {
			s.SetStatus("E85: There is no other file to edit", true)
			return false
		}
		index = (index + delta) % len(s.Buffers)
		if index < 0 {
			index += len(s.Buffers)
		}
	}
	s.Current = index
	s.prepareBuffer(s.CurrentBuffer())
	s.clearLSPTransients()
	s.requestActiveCartInfo()
	return true
}

// SwitchTab implements :tabn/:tabp and Ctrl+Tab for cart code tabs.
func (s *Session) SwitchTab(delta int) (changed bool) {
	buffer := s.CurrentBuffer()
	if buffer == nil || buffer.Cart == nil || len(buffer.Cart.Tabs) == 0 {
		s.SetStatus("E85: no code tabs", true)
		return false
	}
	if len(buffer.Cart.Tabs) == 1 {
		s.SetStatus("E85: only one code tab", true)
		return false
	}
	if delta == 0 {
		return false
	}
	s.endInsertMode()
	s.saveActiveView(buffer)
	next := (buffer.ActiveTab + delta) % len(buffer.Cart.Tabs)
	if next < 0 {
		next += len(buffer.Cart.Tabs)
	}
	buffer.ActiveTab = next
	tab := buffer.Cart.Tabs[next]
	buffer.Editor.SetLineRange(tab.StartLine, tab.EndLine)
	restoreView(buffer.Editor, buffer.TabViews[next])
	s.clearLSPTransients()
	s.SetStatus("", false)
	return true
}

// ShowBufferList opens the :ls overlay.
func (s *Session) ShowBufferList() bool {
	var lines []string
	for index, buffer := range s.Buffers {
		marker := " "
		if index == s.Current {
			marker = "%"
		}
		modified := " "
		if buffer.Editor.Buffer.Dirty() {
			modified = "*"
		}
		name := buffer.Editor.Buffer.Path
		if name == "" {
			name = "[No Name]"
		}
		lines = append(lines, fmt.Sprintf("%d%s%s %s", index+1, marker, modified, name))
	}
	s.Help = &HelpOverlay{
		Title: "Buffers",
		Lines: lines,
	}
	return true
}

// OpenHelp opens the :help overlay.
func (s *Session) OpenHelp(topic string) bool {
	if topic == "" {
		topic = "keys"
	}
	s.glyphPickerPending = false
	if ed := s.CurrentEditor(); ed != nil && s.Vim != nil {
		s.Vim.ResetOperator()
		s.Vim.ResetPending(ed)
	}
	title, lines := helpTopic(topic)
	s.Help = &HelpOverlay{Title: title, Lines: lines}
	return true
}

// SaveOptions are the M4 conflict overrides selected by dialogs.
type SaveOptions struct {
	AllowExternalChange bool
	AllowOutsideRoot    bool
	Backup              bool
}

// SaveJob is a validated snapshot waiting for background IO.
type SaveJob struct {
	Buffer       *editor.Buffer
	Cart         *cart.Document
	Snapshot     editor.SaveSnapshot
	CartSnapshot cart.SaveSnapshot
	QuitAfter    bool
	OnlyIfDirty  bool
	Backup       bool
}

// Empty reports a dirty-check-only job that needs no write.
func (j *SaveJob) Empty() bool {
	return j.OnlyIfDirty && j.Snapshot.Data == nil && j.CartSnapshot.Data == nil
}

// Write performs the write described by the job, including an optional backup
// of conflicting on-disk bytes.
func (j *SaveJob) Write() error {
	if j.Backup {
		if _, err := fsutil.Backup(j.Buffer.Path); err != nil {
			return err
		}
	}
	if j.Cart != nil {
		return j.CartSnapshot.Write()
	}
	return j.Snapshot.Write()
}

// PrepareSave validates and snapshots the current buffer for async writing.
func (s *Session) PrepareSave(action vim.Action) (*SaveJob, error) {
	return s.PrepareSaveBuffer(s.CurrentBuffer(), action, SaveOptions{})
}

// PrepareSaveBuffer validates and snapshots a specific buffer.
func (s *Session) PrepareSaveBuffer(buffer *SessionBuffer, action vim.Action, options SaveOptions) (*SaveJob, error) {
	if buffer == nil {
		return nil, nil
	}
	ed := buffer.Editor
	ed.EndGroup()
	if action.OnlyIfDirty && !ed.Buffer.Dirty() {
		return &SaveJob{Buffer: ed.Buffer, Cart: buffer.Cart, QuitAfter: action.QuitAfter, OnlyIfDirty: true}, nil
	}
	if buffer.Cart != nil {
		snapshot, err := buffer.Cart.PrepareSave(cart.SaveOptions{
			AllowExternalChange: options.AllowExternalChange,
			AllowOutsideRoot:    options.AllowOutsideRoot,
		})
		if err != nil {
			return nil, err
		}
		return &SaveJob{
			Buffer:       ed.Buffer,
			Cart:         buffer.Cart,
			CartSnapshot: snapshot,
			QuitAfter:    action.QuitAfter,
			Backup:       options.Backup,
		}, nil
	}
	snapshot, err := ed.Buffer.PrepareSaveWithOptions(editor.SaveOptions{
		AllowExternalChange: options.AllowExternalChange,
		AllowOutsideRoot:    options.AllowOutsideRoot,
	})
	if err != nil {
		return nil, err
	}
	return &SaveJob{
		Buffer:    ed.Buffer,
		Snapshot:  snapshot,
		QuitAfter: action.QuitAfter,
		Backup:    options.Backup,
	}, nil
}

// CompleteSave commits a finished save and reports whether to quit.
func (s *Session) CompleteSave(job *SaveJob, writeErr error) (quit, changed bool) {
	if job == nil {
		return false, false
	}
	if job.Empty() {
		s.SetStatus("", false)
		if job.QuitAfter {
			return s.CloseBuffer(job.Buffer, true)
		}
		return false, true
	}
	var err error
	if job.Cart != nil {
		err = job.Cart.CommitSave(job.CartSnapshot, writeErr)
	} else {
		err = job.Buffer.CommitSave(job.Snapshot, writeErr)
	}
	if err != nil {
		s.SetStatus(shortError(err), true)
		return false, true
	}
	if !job.Empty() {
		s.LSPDidSave(job.Buffer)
	}
	s.SetStatus("written", false)
	if !job.QuitAfter {
		return false, true
	}
	if job.Buffer.Dirty() {
		s.SetStatus("write complete; buffer still has changes", true)
		return false, true
	}
	return s.CloseBuffer(job.Buffer, true)
}

// Draw composes the current buffer and chrome.
func (s *Session) Draw(surface *render.Surface, cursorVisible bool) {
	surface.Clear(render.DefaultBackground)
	ed := s.CurrentEditor()
	if ed == nil {
		return
	}
	buffer := s.CurrentBuffer()
	top := luaCodeTop
	height := luaCodeRows
	var first, last int
	if buffer != nil && buffer.Cart != nil {
		top = cartCodeTop
		height = cartCodeRows
		if tab, ok := s.currentTab(); ok {
			first, last = tab.StartLine, tab.EndLine
		}
	} else {
		first, last = 0, len(ed.Buffer.Lines)
	}
	s.drawBuffer(surface, ed, top, first, last, height)
	if cursorVisible && s.Help == nil && s.Dialog == nil && s.DiagList == nil && !s.Tree.Focused {
		s.drawCursor(surface, ed, top, first, last)
	}
	s.drawFileTabs(surface)
	s.drawCartTabs(surface)
	s.drawStatus(surface, ed, cursorVisible)
	if s.Tree.Visible {
		s.drawTree(surface)
	}
	s.drawSignature(surface)
	s.drawCompletion(surface, ed, top)
	s.drawHover(surface)
	if s.DiagList != nil {
		s.drawDiagnosticList(surface)
	}
	if s.GlyphPicker != nil {
		s.drawGlyphPicker(surface)
	}
	if s.Help != nil {
		s.drawHelp(surface)
	}
	if s.Dialog != nil {
		s.drawDialog(surface)
	}
}

func (s *Session) handleHelp(ev input.Event) bool {
	if ev.Kind != input.KindKeyDown {
		return false
	}
	if ev.Key == input.KeyEsc || (ev.Key == input.KeyQ && ev.Mods == 0) {
		s.Help = nil
		return true
	}
	switch ev.Key {
	case input.KeyUp:
		s.Help.Top = max(0, s.Help.Top-1)
	case input.KeyDown:
		s.Help.Top++
	case input.KeyPageUp:
		s.Help.Top = max(0, s.Help.Top-10)
	case input.KeyPageDown:
		s.Help.Top += 10
	}
	return true
}

// HelpOverlay is a simple scrollable text panel used by :help and :ls.
type HelpOverlay struct {
	Title string
	Lines []string
	Top   int
}

func helpTopic(topic string) (string, []string) {
	switch topic {
	case "input":
		return "Help: input", []string{
			"Insert mode is always authentic PICO-8 input.",
			"Shift+letter inserts glyphs 0x80-0x99 (A=█ B=▒ C=🐱 ...).",
			"Ctrl+J hiragana: type romaji (ka, kya, sha, ltsu).",
			"Ctrl+K katakana: same romaji, plus Shift+0..9 symbols.",
			"Ctrl+P puny: unshifted=uppercase ASCII, Shift=standard letters.",
			"Ctrl+J/K/P are mutually exclusive; Esc leaves the mode.",
			"Ctrl+Shift+G or :glyph opens the searchable glyph picker.",
			"Ctrl+Shift+V pastes; Ctrl+Shift+C copies in visual mode.",
			"Normal-mode Vim Shift commands are never converted.",
			"See docs/INPUT.md for tables and the uppercase workflow.",
		}
	case "lsp":
		return "Help: lsp", []string{
			"pico-lsp runs in-process via internal/lsp.",
			"K hover; gd or Ctrl+] definition; Ctrl+O/I jump back/forward.",
			"Ctrl+Space completion; Ctrl+N / Ctrl+Shift+N select;",
			"Enter/Tab accept; Esc closes the popup first.",
			"Typing ( or , opens signature help.",
			":diag lists diagnostics; :lsp restart restarts pico-lsp.",
			"Status shows E/W, T/C/Z, and LSP state.",
		}
	case "config":
		return "Help: config", []string{
			"TOML settings load in two layers:",
			"Global: $XDG_CONFIG_HOME/pico-editor/config.toml",
			"Workspace: <root>/.pico-editor.toml",
			"Workspace values win; arrays replace, tables merge.",
			"Edit [keys.<mode>] to bind commands, \"\" to unbind.",
			":reload or Config: reload settings applies changes live.",
			"Scale changes need a restart. See docs/CONFIG.md and KEYMAP.md.",
		}
	case "troubleshooting":
		return "Help: troubleshooting", []string{
			"Logs: $XDG_STATE_HOME/pico-editor/pico-editor.log (rotates at 1MB).",
			"Use --log-level debug and --log-file - for stderr output.",
			"Recovery journals live in $XDG_STATE_HOME/pico-editor/recovery.",
			"If the window is blurry, fixed scales 1..8 avoid fractional scaling.",
			"Clipboard uses XWayland on GNOME Wayland and falls back to registers.",
			"If pico-lsp fails, run :lsp restart and check the log.",
			"See docs/TROUBLESHOOTING.md for graphics, IME, and recovery help.",
		}
	default:
		return "Help: keys", []string{
			"i a I A o O   enter insert mode",
			"h j k l w b e 0 ^ $   motions",
			"d c y p x X r J ~ > <   operators",
			"iw aw i( a( i{ a{ i\" a\"   text objects",
			"v V Ctrl+V   visual char/line/block",
			"/ ? n N * #   search",
			":w :q :q! :wq :x :e :s   ex commands",
			"u Ctrl+R .   undo, redo, repeat",
			"gg G Ctrl+O Ctrl+I zz zt zb   jumps/scroll",
			"K hover; gd Ctrl+] definition; :diag; :lsp restart",
			"Ctrl+Space completion; Ctrl+N Ctrl+Shift+N select",
			"Ctrl+Shift+P palette; Ctrl+Shift+M messages",
			"Ctrl+Shift+G glyph picker; F1 help",
			"Ctrl+Shift+B tree; Ctrl+S saves; Ctrl+Tab tabs",
			":reload config; :messages history; :help config",
			"Esc cancels pending state or leaves a mode.",
		}
	}
}
