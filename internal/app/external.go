package app

import (
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/asimmons91/pico-editor/internal/cart"
	"github.com/asimmons91/pico-editor/internal/vim"
)

// ExternalConflict describes an open buffer whose file changed or vanished on
// disk and needs a user decision.
type ExternalConflict struct {
	Buffer  *SessionBuffer
	Deleted bool
	Err     error
}

// RefreshFromDisk refreshes the workspace and open buffers when the app gains
// focus. Clean changed files reload automatically; the first dirty or deleted
// conflict is returned for the app to show a dialog.
func (s *Session) RefreshFromDisk() *ExternalConflict {
	if s.Workspace != nil {
		if err := s.Workspace.Refresh(); err != nil {
			s.ShowError("Refresh failed", err)
		} else {
			if s.Workspace.Truncated {
				s.SetStatus("file tree truncated at 20000 entries", true)
			} else if len(s.Workspace.Warnings) > 0 {
				s.workspaceStatus(s.Workspace.Warnings)
			}
		}
	}
	for index, buffer := range s.Buffers {
		s.refreshOwner(buffer)
		var err error
		deleted := false
		if buffer.Cart != nil {
			err = buffer.Cart.CheckExternal()
			deleted = errors.Is(err, cart.ErrDeleted)
		} else {
			err = buffer.Editor.Buffer.CheckExternal()
			deleted = errors.Is(err, fs.ErrNotExist)
		}
		if err == nil {
			continue
		}
		if !buffer.Editor.Buffer.Dirty() {
			if deleted {
				return &ExternalConflict{Buffer: buffer, Deleted: true, Err: err}
			}
			if buffer.Cart != nil {
				if reloadErr := s.reloadBufferAtPreserving(buffer, index); reloadErr != nil {
					return &ExternalConflict{Buffer: buffer, Err: err}
				}
			} else {
				if reloadErr := buffer.Editor.Reload(); reloadErr != nil {
					return &ExternalConflict{Buffer: buffer, Err: err}
				}
				s.reloadLSPDocument(buffer)
				buffer.Editor.Buffer.SetWorkspaceRoot(s.Workspace.Root)
				if index == s.Current {
					s.prepareBuffer(buffer)
				}
			}
			continue
		}
		return &ExternalConflict{Buffer: buffer, Deleted: deleted, Err: err}
	}
	return nil
}

func (s *Session) refreshOwner(buffer *SessionBuffer) {
	if buffer.Cart != nil || s.Workspace == nil || buffer.Editor.Buffer.Path == "" {
		return
	}
	path, err := filepath.Abs(buffer.Editor.Buffer.Path)
	if err != nil {
		return
	}
	if ref, ok := s.Workspace.ResolveInclude(path); ok {
		buffer.Owner = ref.Path
	} else {
		buffer.Owner = ""
	}
}

func (g *Game) showExternalConflict(conflict *ExternalConflict) {
	if conflict == nil || conflict.Buffer == nil {
		return
	}
	if conflict.Deleted {
		g.showSaveDialog(DialogDeleted, conflict.Buffer, vim.Action{Kind: vim.ActionSave}, SaveOptions{}, conflict.Err)
		return
	}
	g.showSaveDialog(DialogConflict, conflict.Buffer, vim.Action{Kind: vim.ActionSave}, SaveOptions{}, conflict.Err)
}

// Click routes a canvas-space left click to the dialog-free UI.
func (s *Session) Click(x, y int) bool {
	if s.Dialog != nil {
		return false
	}
	if s.Tree.Visible && s.ClickTree(x, y) {
		return true
	}
	if s.ClickFileTabs(x, y) {
		return true
	}
	return s.ClickCartTabs(x, y)
}
