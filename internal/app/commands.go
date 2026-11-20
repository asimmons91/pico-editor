package app

import (
	"strings"

	"github.com/asimmons91/pico-editor/internal/lsp"
	"github.com/asimmons91/pico-editor/internal/vim"
)

// Command is one discoverable application command. The ID is stable: keymaps
// and documentation use it.
type Command struct {
	ID       string
	Title    string
	Category string
	Key      string
	Enabled  func(*Game) bool
	Run      func(*Game) (quit, changed bool)
}

func (g *Game) buildCommands() []Command {
	hasSession := func(*Game) bool { return g.session != nil }
	currentBuffer := func(*Game) bool {
		return g.session != nil && g.session.CurrentBuffer() != nil
	}
	currentCart := func(*Game) bool {
		if g.session == nil {
			return false
		}
		buffer := g.session.CurrentBuffer()
		return buffer != nil && buffer.Cart != nil
	}
	multiBuffer := func(*Game) bool {
		return g.session != nil && len(g.session.Buffers) > 1
	}
	insertMode := func(*Game) bool {
		return g.session != nil && g.session.Vim.Mode == vim.ModeInsert
	}
	lspReady := func(*Game) bool {
		return g.session != nil && g.session.LSP != nil && g.session.LSPStatus == lsp.StatusReady
	}

	return []Command{
		{ID: "file.save", Title: "File: save", Category: "File", Key: "ctrl+s", Enabled: currentBuffer, Run: func(g *Game) (bool, bool) {
			quit, changed := g.startSaveFor(g.session.CurrentBuffer(), vim.Action{Kind: vim.ActionSave}, SaveOptions{})
			return quit, changed
		}},
		{ID: "file.close", Title: "File: close", Category: "File", Key: ":q", Enabled: currentBuffer, Run: func(g *Game) (bool, bool) {
			if g.saveBusy {
				g.session.SetStatus("save in progress; close again when it finishes", true)
				return false, true
			}
			return g.session.RequestQuit(false)
		}},
		{ID: "file.reload", Title: "File: reload", Category: "File", Key: ":e", Enabled: currentBuffer, Run: func(g *Game) (bool, bool) {
			return false, g.session.OpenPath("", false)
		}},
		{ID: "buffer.next", Title: "Buffer: next", Category: "Buffer", Key: ":bn", Enabled: multiBuffer, Run: func(g *Game) (bool, bool) {
			return false, g.session.SwitchBuffer(0, 1)
		}},
		{ID: "buffer.prev", Title: "Buffer: previous", Category: "Buffer", Key: ":bp", Enabled: multiBuffer, Run: func(g *Game) (bool, bool) {
			return false, g.session.SwitchBuffer(0, -1)
		}},
		{ID: "buffer.list", Title: "Buffer: list", Category: "Buffer", Key: ":ls", Enabled: hasSession, Run: func(g *Game) (bool, bool) {
			return false, g.session.ShowBufferList()
		}},
		{ID: "cart.tab.next", Title: "Cart: next code tab", Category: "Cart", Key: "ctrl+tab", Enabled: currentCart, Run: func(g *Game) (bool, bool) {
			return false, g.session.SwitchTab(1)
		}},
		{ID: "cart.tab.prev", Title: "Cart: previous code tab", Category: "Cart", Key: "ctrl+shift+tab", Enabled: currentCart, Run: func(g *Game) (bool, bool) {
			return false, g.session.SwitchTab(-1)
		}},
		{ID: "tree.toggle", Title: "Tree: toggle file tree", Category: "Tree", Key: "ctrl+shift+b", Enabled: hasSession, Run: func(g *Game) (bool, bool) {
			return false, g.session.ToggleTree()
		}},
		{ID: "tree.refresh", Title: "Tree: refresh workspace", Category: "Tree", Enabled: hasSession, Run: func(g *Game) (bool, bool) {
			return false, g.session.RefreshTree()
		}},
		{ID: "palette.show", Title: "Palette: show commands", Category: "Palette", Key: "ctrl+shift+p", Enabled: hasSession, Run: func(g *Game) (bool, bool) {
			g.openPalette()
			return false, true
		}},
		{ID: "help.open", Title: "Help: keys", Category: "Help", Key: "f1", Enabled: hasSession, Run: func(g *Game) (bool, bool) {
			return false, g.session.OpenHelp("")
		}},
		{ID: "help.input", Title: "Help: input", Category: "Help", Key: ":help input", Enabled: hasSession, Run: func(g *Game) (bool, bool) {
			return false, g.session.OpenHelp("input")
		}},
		{ID: "diag.list", Title: "LSP: diagnostics list", Category: "LSP", Key: ":diag", Enabled: hasSession, Run: func(g *Game) (bool, bool) {
			return false, g.session.ShowDiagnostics()
		}},
		{ID: "messages.show", Title: "Messages: show history", Category: "Messages", Key: "ctrl+shift+m", Enabled: hasSession, Run: func(g *Game) (bool, bool) {
			g.session.SetStatus("message history arrives in M7", false)
			return false, true
		}},
		{ID: "lsp.restart", Title: "LSP: restart pico-lsp", Category: "LSP", Key: ":lsp restart", Enabled: lspReady, Run: func(g *Game) (bool, bool) {
			g.restartLSP()
			return false, true
		}},
		{ID: "glyph.pick", Title: "Glyph: pick", Category: "Glyph", Key: "ctrl+shift+g", Enabled: lspReady, Run: func(g *Game) (bool, bool) {
			return false, g.session.OpenGlyphPicker()
		}},
		{ID: "input.puny", Title: "Input: toggle puny", Category: "Input", Key: "ctrl+p", Enabled: insertMode, Run: func(g *Game) (bool, bool) {
			return false, g.session.TogglePuny()
		}},
		{ID: "config.reload", Title: "Config: reload settings", Category: "Config", Enabled: hasSession, Run: func(g *Game) (bool, bool) {
			g.session.SetStatus("settings arrive in M7", false)
			return false, true
		}},
		{ID: "app.quit", Title: "App: quit", Category: "App", Key: "ctrl+q", Enabled: hasSession, Run: func(g *Game) (bool, bool) {
			return g.requestQuitAll()
		}},
	}
}

func (g *Game) commandEnabled(command Command) bool {
	if command.Enabled == nil {
		return true
	}
	return command.Enabled(g)
}

func commandSearchText(command Command) string {
	return strings.ToLower(command.Title + " " + command.Category + " " + command.ID)
}

func fuzzyMatch(pattern, text string) bool {
	if pattern == "" {
		return true
	}
	index := 0
	for _, r := range text {
		if index < len(pattern) && r == rune(pattern[index]) {
			index++
		}
	}
	return index == len(pattern)
}
