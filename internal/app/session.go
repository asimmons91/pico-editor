package app

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/asimmons91/pico-editor/internal/editor"
	"github.com/asimmons91/pico-editor/internal/input"
	"github.com/asimmons91/pico-editor/internal/render"
	"github.com/asimmons91/pico-editor/internal/vim"
)

const (
	statusRow           = GridRows - 1
	editorRows          = GridRows - 1
	statusForeground    = 6
	statusErrorColor    = 8
	searchBackground    = 1
	selectionBackground = 7
	cursorInsertColor   = 10
)

// Session is the Vim-backed editor screen. It owns a list of file buffers, the
// modal state shared by them, and the help overlay.
type Session struct {
	Buffers []*SessionBuffer
	Current int
	Vim     *vim.Vim
	Help    *HelpOverlay

	Status      string
	StatusError bool

	logger *slog.Logger
	quit   bool
}

// SessionBuffer is one open file. M4 extends this with cart code tabs.
type SessionBuffer struct {
	Editor *editor.Editor
}

// NewSession opens path or creates an empty unnamed buffer.
func NewSession(path string, logger *slog.Logger) (*Session, error) {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	var ed *editor.Editor
	if path == "" {
		ed = editor.NewEditor(editor.New(""))
	} else {
		buffer, err := editor.Open(path)
		if err != nil {
			return nil, err
		}
		ed = editor.NewEditor(buffer)
	}
	prepareEditor(ed)
	return &Session{
		Buffers: []*SessionBuffer{{Editor: ed}},
		Vim:     vim.New(),
		logger:  logger,
	}, nil
}

func prepareEditor(ed *editor.Editor) {
	ed.Viewport.Width = GridColumns
	ed.Viewport.Height = editorRows
	ed.Viewport.ScrollOff = 2
	ed.EnsureVisible()
}

// CurrentEditor returns the active editor, or nil if there is no buffer.
func (s *Session) CurrentEditor() *editor.Editor {
	if s.Current < 0 || s.Current >= len(s.Buffers) {
		return nil
	}
	return s.Buffers[s.Current].Editor
}

// HandleEvent routes one input event to the help overlay or Vim.
func (s *Session) HandleEvent(ev input.Event) (quit, changed bool, actions []vim.Action) {
	if s.Help != nil {
		return false, s.handleHelp(ev), nil
	}
	ed := s.CurrentEditor()
	if ed == nil {
		return false, false, nil
	}
	result := s.Vim.Handle(ev, ed)
	s.applyVimResult(result)
	return false, true, result.Actions
}

func (s *Session) applyVimResult(result vim.Result) (quit bool) {
	if result.Notify != "" {
		s.Status = result.Notify
		s.StatusError = result.Error
	}
	return false
}

// CompleteClipboardRead applies an asynchronous clipboard read.
func (s *Session) CompleteClipboardRead(text string, err error) (changed bool, actions []vim.Action) {
	ed := s.CurrentEditor()
	if ed == nil {
		return false, nil
	}
	result := s.Vim.CompleteClipboardRead(text, err, ed)
	s.applyVimResult(result)
	return true, result.Actions
}

// SetStatus replaces the status notification.
func (s *Session) SetStatus(message string, isError bool) {
	s.Status = message
	s.StatusError = isError
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
	if ed != nil && ed.Buffer.Dirty() && !force {
		s.SetStatus("E37: No write since last change (add ! to override)", true)
		return false, true
	}
	if len(s.Buffers) <= 1 {
		s.quit = true
		return true, true
	}
	s.Buffers = append(s.Buffers[:s.Current], s.Buffers[s.Current+1:]...)
	if s.Current >= len(s.Buffers) {
		s.Current = len(s.Buffers) - 1
	}
	prepareEditor(s.CurrentEditor())
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
	if path == "" {
		if ed.Buffer.Dirty() && !force {
			s.SetStatus("E37: No write since last change (add ! to override)", true)
			return false
		}
		ed.EndGroup()
		if err := ed.Reload(); err != nil {
			s.SetStatus(shortError(err), true)
			return false
		}
		prepareEditor(ed)
		s.SetStatus("reloaded", false)
		return true
	}

	root, err := os.Getwd()
	if err != nil {
		root = "."
	}
	if ed.Buffer.Path != "" {
		root = filepath.Dir(ed.Buffer.Path)
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
	for index, buffer := range s.Buffers {
		bufferPath, err := filepath.Abs(buffer.Editor.Buffer.Path)
		if err == nil && bufferPath == absolute {
			s.Current = index
			prepareEditor(buffer.Editor)
			return true
		}
	}
	buffer, err := editor.Open(absolute)
	if err != nil {
		s.SetStatus("E484: Can't open file: "+shortError(err), true)
		return false
	}
	opened := editor.NewEditor(buffer)
	prepareEditor(opened)
	if ed.Buffer.Path == "" && !ed.Buffer.Dirty() {
		// Reuse an empty unnamed buffer instead of leaving it in the list.
		s.Buffers[s.Current].Editor = opened
	} else {
		s.Buffers = append(s.Buffers, &SessionBuffer{Editor: opened})
		s.Current = len(s.Buffers) - 1
	}
	s.SetStatus("", false)
	return true
}

// SwitchBuffer implements :bn, :bp, and :b N.
func (s *Session) SwitchBuffer(number, delta int) (changed bool) {
	if len(s.Buffers) == 0 {
		return false
	}
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
	prepareEditor(s.CurrentEditor())
	return true
}

// SwitchTab is a single-tab stub until M4 adds cart code tabs.
func (s *Session) SwitchTab(delta int) (changed bool) {
	s.SetStatus("only one code tab is available until M4", false)
	return false
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
	title, lines := helpTopic(topic)
	s.Help = &HelpOverlay{Title: title, Lines: lines}
	return true
}

// SaveJob is a validated snapshot waiting for background IO.
type SaveJob struct {
	Buffer      *editor.Buffer
	Snapshot    editor.SaveSnapshot
	QuitAfter   bool
	OnlyIfDirty bool
}

// PrepareSave validates and snapshots the current buffer for async writing.
func (s *Session) PrepareSave(action vim.Action) (*SaveJob, error) {
	ed := s.CurrentEditor()
	if ed == nil {
		return nil, nil
	}
	ed.EndGroup()
	if action.OnlyIfDirty && !ed.Buffer.Dirty() {
		return &SaveJob{Buffer: ed.Buffer, QuitAfter: action.QuitAfter, OnlyIfDirty: true}, nil
	}
	snapshot, err := ed.Buffer.PrepareSave()
	if err != nil {
		return nil, err
	}
	return &SaveJob{
		Buffer:    ed.Buffer,
		Snapshot:  snapshot,
		QuitAfter: action.QuitAfter,
	}, nil
}

// CompleteSave commits a finished save and reports whether to quit.
func (s *Session) CompleteSave(job *SaveJob, writeErr error) (quit, changed bool) {
	if job == nil {
		return false, false
	}
	if job.OnlyIfDirty && job.Snapshot.Data == nil {
		s.SetStatus("", false)
		if job.QuitAfter {
			return s.RequestQuit(true)
		}
		return false, true
	}
	if err := job.Buffer.CommitSave(job.Snapshot, writeErr); err != nil {
		s.SetStatus(shortError(err), true)
		return false, true
	}
	s.SetStatus("written", false)
	if !job.QuitAfter {
		return false, true
	}
	if job.Buffer.Dirty() {
		s.SetStatus("write complete; buffer still has changes", true)
		return false, true
	}
	return s.RequestQuit(true)
}

// Draw composes the current buffer and chrome.
func (s *Session) Draw(surface *render.Surface, cursorVisible bool) {
	surface.Clear(render.DefaultBackground)
	ed := s.CurrentEditor()
	if ed == nil {
		return
	}
	s.drawBuffer(surface, ed)
	if cursorVisible && s.Help == nil {
		s.drawCursor(surface, ed)
	}
	s.drawStatus(surface, ed, cursorVisible)
	if s.Help != nil {
		s.drawHelp(surface)
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
			"P8SCII input modes arrive in M6.",
			"Shift+letter glyph insertion, Ctrl+J/K kana, and Ctrl+P",
			"literal mode will be documented in docs/INPUT.md.",
			"Normal-mode Vim Shift commands are never converted.",
		}
	case "lsp":
		return "Help: lsp", []string{
			"pico-lsp runs in-process from M5.",
			"Diagnostics, semantic tokens, completion, hover,",
			"signature help, and go-to-definition arrive then.",
		}
	case "config":
		return "Help: config", []string{
			"TOML settings and keymaps arrive in M7.",
			"Global: $XDG_CONFIG_HOME/pico-editor/config.toml",
			"Workspace: <root>/.pico-editor.toml",
		}
	case "troubleshooting":
		return "Help: troubleshooting", []string{
			"Logs: $XDG_STATE_HOME/pico-editor/pico-editor.log",
			"Use :q! to discard changes, :w to write.",
			"Mouse, splits, macros, and plugins are out of scope for V1.",
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
			"Esc cancels pending state or leaves a mode.",
		}
	}
}
