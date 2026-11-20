package app

import (
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/asimmons91/pico-editor/internal/cart"
	"github.com/asimmons91/pico-editor/internal/editor"
	"github.com/asimmons91/pico-editor/internal/vim"
	"github.com/asimmons91/pico-editor/internal/workspace"
)

const (
	luaCodeRows  = GridRows - 2 // rows 1..40
	cartCodeRows = GridRows - 3 // rows 2..40
	luaCodeTop   = 1
	cartCodeTop  = 2
)

// NewSessionAt opens root as the workspace and path as the first buffer. An
// empty path opens an empty buffer and focuses the file tree.
func NewSessionAt(root, path string, logger *slog.Logger) (*Session, error) {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	workspaceTree, err := workspace.OpenWithLogger(root, logger)
	if err != nil {
		return nil, err
	}
	session := &Session{
		Vim:       vim.New(),
		Workspace: workspaceTree,
		logger:    logger,
	}
	session.Tree.Init(session)
	if workspaceTree.Truncated {
		session.SetStatus("file tree truncated at 20000 entries", true)
	} else if len(workspaceTree.Warnings) > 0 {
		session.workspaceStatus(workspaceTree.Warnings)
	}
	if path == "" {
		empty := editor.NewEditor(editor.New(""))
		empty.Buffer.SetWorkspaceRoot(workspaceTree.Root)
		session.Buffers = []*SessionBuffer{{Editor: empty}}
		session.prepareBuffer(session.Buffers[0])
		session.Tree.Visible = true
		session.Tree.Focused = true
		return session, nil
	}
	buffer, err := session.openSessionBuffer(path)
	if err != nil {
		return nil, err
	}
	session.Buffers = []*SessionBuffer{buffer}
	session.prepareBuffer(buffer)
	session.Tree.Selected = buffer.Editor.Buffer.Path
	return session, nil
}

func isCartPath(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".p8")
}

func (s *Session) openSessionBuffer(path string) (*SessionBuffer, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	absolute = filepath.Clean(absolute)
	if isCartPath(absolute) {
		document, err := cart.Open(absolute)
		if err != nil {
			return nil, err
		}
		document.SetWorkspaceRoot(s.Workspace.Root)
		opened := &SessionBuffer{
			Editor: editor.NewEditor(document.Code),
			Cart:   document,
		}
		opened.TabViews = make([]cart.ViewState, len(document.Tabs))
		return opened, nil
	}
	buffer, err := editor.Open(absolute)
	if err != nil {
		return nil, err
	}
	buffer.SetWorkspaceRoot(s.Workspace.Root)
	opened := &SessionBuffer{Editor: editor.NewEditor(buffer)}
	if ref, ok := s.Workspace.ResolveInclude(absolute); ok {
		opened.Owner = ref.Path
	}
	return opened, nil
}

func (s *Session) prepareBuffer(buffer *SessionBuffer) {
	if buffer == nil {
		return
	}
	ed := buffer.Editor
	ed.Viewport.Width = GridColumns
	ed.Viewport.ScrollOff = 2
	if buffer.Cart == nil {
		ed.Viewport.Height = luaCodeRows
		ed.ClearLineRange()
		ed.EnsureVisible()
		return
	}
	ed.Viewport.Height = cartCodeRows
	if buffer.ActiveTab < 0 {
		buffer.ActiveTab = 0
	}
	tabs := buffer.Cart.Tabs
	if len(tabs) == 0 {
		tabs = []cart.Tab{{Index: 0, Name: "Tab 1"}}
	}
	if buffer.ActiveTab >= len(tabs) {
		buffer.ActiveTab = len(tabs) - 1
	}
	if len(buffer.TabViews) < len(tabs) {
		buffer.TabViews = append(buffer.TabViews, make([]cart.ViewState, len(tabs)-len(buffer.TabViews))...)
	}
	tab := tabs[buffer.ActiveTab]
	ed.SetLineRange(tab.StartLine, tab.EndLine)
	ed.EnsureVisible()
	buffer.TabViews[buffer.ActiveTab] = captureView(ed)
	buffer.SyncedVersion = ed.Buffer.Version
}

func captureView(ed *editor.Editor) cart.ViewState {
	return cart.ViewState{
		Cursor:      ed.Cursor.Pos,
		DesiredCell: ed.Cursor.DesiredCell,
		TopLine:     ed.Viewport.TopLine,
		LeftCell:    ed.Viewport.LeftCell,
	}
}

func restoreView(ed *editor.Editor, view cart.ViewState) {
	ed.SetCursor(view.Cursor)
	ed.Cursor.DesiredCell = view.DesiredCell
	ed.Viewport.TopLine = view.TopLine
	ed.Viewport.LeftCell = view.LeftCell
	ed.EnsureVisible()
}

// saveActiveView stores the cursor and scroll position of the active cart tab.
func (s *Session) saveActiveView(buffer *SessionBuffer) {
	if buffer == nil || buffer.Cart == nil {
		return
	}
	if buffer.ActiveTab >= 0 && buffer.ActiveTab < len(buffer.TabViews) {
		buffer.TabViews[buffer.ActiveTab] = captureView(buffer.Editor)
	}
}

// syncCart refreshes tab ranges after an edit and keeps per-tab view state
// aligned with shifted separator lines.
func (s *Session) syncCart(buffer *SessionBuffer) {
	if buffer == nil || buffer.Cart == nil {
		return
	}
	if buffer.SyncedVersion == buffer.Editor.Buffer.Version {
		return
	}
	oldTabs := append([]cart.Tab(nil), buffer.Cart.Tabs...)
	if err := buffer.Cart.SyncTabs(); err != nil {
		s.logger.Warn("cart tab sync", "path", buffer.Cart.Path, "error", err)
		return
	}
	tabs := buffer.Cart.Tabs
	if len(tabs) == 0 {
		return
	}
	if len(tabs) > len(oldTabs) {
		extra := len(tabs) - len(oldTabs)
		if len(buffer.TabViews) < len(tabs) {
			buffer.TabViews = append(buffer.TabViews, make([]cart.ViewState, len(tabs)-len(buffer.TabViews))...)
		}
		active := min(buffer.ActiveTab, len(oldTabs)-1)
		if active >= 0 && active+1 < len(buffer.TabViews) {
			copy(buffer.TabViews[active+1+extra:], buffer.TabViews[active+1:len(buffer.TabViews)-extra])
			for index := active + 1; index <= active+extra && index < len(buffer.TabViews); index++ {
				buffer.TabViews[index] = cart.ViewState{}
			}
		}
	} else if len(tabs) < len(oldTabs) {
		extra := len(oldTabs) - len(tabs)
		active := min(buffer.ActiveTab, len(tabs)-1)
		if active >= 0 && active+1+extra <= len(buffer.TabViews) {
			buffer.TabViews = append(buffer.TabViews[:active+1], buffer.TabViews[active+1+extra:]...)
		}
		if len(buffer.TabViews) > len(tabs) {
			buffer.TabViews = buffer.TabViews[:len(tabs)]
		}
	}
	for index := range buffer.TabViews {
		if index >= len(oldTabs) || index >= len(tabs) {
			continue
		}
		delta := tabs[index].StartLine - oldTabs[index].StartLine
		if delta == 0 {
			continue
		}
		view := &buffer.TabViews[index]
		view.Cursor.Line = max(0, view.Cursor.Line+delta)
		view.TopLine = max(0, view.TopLine+delta)
	}
	if buffer.ActiveTab >= len(tabs) {
		buffer.ActiveTab = len(tabs) - 1
	}
	tab := tabs[buffer.ActiveTab]
	buffer.Editor.SetLineRange(tab.StartLine, tab.EndLine)
	buffer.Editor.EnsureVisible()
	buffer.TabViews[buffer.ActiveTab] = captureView(buffer.Editor)
	buffer.SyncedVersion = buffer.Editor.Buffer.Version
}

func (s *Session) syncCurrentCart() {
	s.syncCart(s.CurrentBuffer())
}

func (s *Session) currentTab() (cart.Tab, bool) {
	buffer := s.CurrentBuffer()
	if buffer == nil || buffer.Cart == nil || len(buffer.Cart.Tabs) == 0 {
		return cart.Tab{}, false
	}
	index := min(max(buffer.ActiveTab, 0), len(buffer.Cart.Tabs)-1)
	return buffer.Cart.Tabs[index], true
}

func (s *Session) bufferName(buffer *SessionBuffer) string {
	if buffer == nil || buffer.Editor.Buffer.Path == "" {
		return "[No Name]"
	}
	return filepath.Base(buffer.Editor.Buffer.Path)
}

// reloadBufferAtPreserving keeps the active cart tab and its cursor/scroll
// state when an external change reloads a clean buffer.
func (s *Session) reloadBufferAtPreserving(buffer *SessionBuffer, index int) error {
	s.saveActiveView(buffer)
	active := buffer.ActiveTab
	view := cart.ViewState{}
	if active >= 0 && active < len(buffer.TabViews) {
		view = buffer.TabViews[active]
	}
	fresh, err := s.openSessionBuffer(buffer.Editor.Buffer.Path)
	if err != nil {
		return err
	}
	if fresh.Cart != nil && len(fresh.Cart.Tabs) > 0 {
		if active >= len(fresh.Cart.Tabs) {
			active = len(fresh.Cart.Tabs) - 1
		}
		fresh.ActiveTab = max(0, active)
		if fresh.ActiveTab < len(fresh.TabViews) {
			fresh.TabViews[fresh.ActiveTab] = view
		}
	}
	s.Buffers[index] = fresh
	s.prepareBuffer(fresh)
	if fresh.Cart != nil && len(fresh.Cart.Tabs) > 0 {
		if fresh.ActiveTab < len(fresh.TabViews) {
			fresh.TabViews[fresh.ActiveTab] = view
		}
		tab := fresh.Cart.Tabs[fresh.ActiveTab]
		fresh.Editor.SetLineRange(tab.StartLine, tab.EndLine)
		restoreView(fresh.Editor, fresh.TabViews[fresh.ActiveTab])
	}
	return nil
}

func (s *Session) unsavedBuffers() []*SessionBuffer {
	var dirty []*SessionBuffer
	for _, buffer := range s.Buffers {
		if buffer.Editor.Buffer.Dirty() {
			dirty = append(dirty, buffer)
		}
	}
	return dirty
}

func (s *Session) workspaceStatus(warnings []string) {
	if len(warnings) == 0 {
		return
	}
	s.SetStatus(warnings[0], true)
}
