package app

import (
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/asimmons91/pico-editor/internal/config"
	"github.com/asimmons91/pico-editor/internal/input"
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
	lspPresent := func(*Game) bool {
		return g.session != nil && g.session.LSP != nil
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
			quit, changed := g.session.RequestQuit(false)
			if !quit {
				g.writeRecovery()
			}
			return quit, changed
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
		{ID: "diag.list", Title: "LSP: diagnostics list", Category: "LSP", Key: ":diag", Enabled: lspPresent, Run: func(g *Game) (bool, bool) {
			return false, g.session.ShowDiagnostics()
		}},
		{ID: "messages.show", Title: "Messages: show history", Category: "Messages", Key: "ctrl+shift+m", Enabled: hasSession, Run: func(g *Game) (bool, bool) {
			return false, g.showMessages()
		}},
		{ID: "lsp.restart", Title: "LSP: restart pico-lsp", Category: "LSP", Key: ":lsp restart", Enabled: lspPresent, Run: func(g *Game) (bool, bool) {
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
			return false, g.reloadConfig()
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

// sanitizeKeymapCommands removes bindings whose IDs are not in the registry.
// Config validation owns key syntax; the app owns the stable command list.
func sanitizeKeymapCommands(settings config.Settings, commands []Command) (config.Settings, []config.Warning) {
	known := make(map[string]bool, len(commands))
	for _, command := range commands {
		known[command.ID] = true
	}
	settings = settings.Clone()
	defaults := config.DefaultKeys()
	var warnings []config.Warning
	for mode, bindings := range settings.Keys {
		for key, commandID := range bindings {
			if known[commandID] {
				continue
			}
			if fallback, ok := defaults[mode][key]; ok {
				bindings[key] = fallback
				warnings = append(warnings, config.Warning{
					Key:     "keys." + mode + "." + key,
					Message: fmt.Sprintf("unknown command %q; restored %s", commandID, fallback),
				})
			} else {
				delete(bindings, key)
				warnings = append(warnings, config.Warning{
					Key:     "keys." + mode + "." + key,
					Message: fmt.Sprintf("unknown command %q removed", commandID),
				})
			}
		}
	}
	return settings, warnings
}

// runCommand executes one registry command with panic recovery so a bug in a
// command cannot take down the frame loop. Fatal render/init failures still
// flow through the normal doFrame path.
func (g *Game) runCommand(command Command) (quit, changed bool) {
	g.logger.Debug("command", "id", command.ID)
	defer func() {
		if recovered := recover(); recovered != nil {
			g.logger.Error("command panic", "command", command.ID, "panic", recovered, "stack", string(debug.Stack()))
			g.notifyError("Command failed", fmt.Errorf("%s: %v", command.ID, recovered))
			quit, changed = false, true
		}
	}()
	if !g.commandEnabled(command) {
		return false, false
	}
	return command.Run(g)
}

// tryKeymap looks up one key event in the active mode's keymap and runs the
// bound command. It runs before Vim so a rebound global shortcut wins, but the
// keymap validator prevents Vim grammar keys from entering the table.
func (g *Game) tryKeymap(event input.Event) (handled, quit bool) {
	if g.session == nil || event.Kind != input.KindKeyDown || event.Repeat {
		return false, false
	}
	mode := keymapModeForVim(g.session.Vim.Mode)
	key := input.FormatKey(event.Key, event.Mods)
	commandID, ok := g.settings.CommandFor(mode, key)
	if !ok {
		return false, false
	}
	for _, command := range g.commands {
		if command.ID != commandID {
			continue
		}
		quit, _ := g.runCommand(command)
		return true, quit
	}
	g.notifyError("Unknown command", fmt.Errorf("%s is bound to %s but is not registered", key, commandID))
	return true, false
}

func keymapModeForVim(mode vim.Mode) string {
	switch mode {
	case vim.ModeInsert:
		return "insert"
	case vim.ModeVisualChar, vim.ModeVisualLine, vim.ModeVisualBlock:
		return "visual"
	case vim.ModeCommandLine:
		return "command"
	default:
		return "normal"
	}
}

// displayKey returns the active-mode binding for a command, falling back to
// the registry's default label.
func (g *Game) displayKey(command Command) string {
	if g.session != nil {
		if key := g.settings.KeyFor(keymapModeForVim(g.session.Vim.Mode), command.ID); key != "" {
			return key
		}
		if strings.HasPrefix(command.Key, ":") {
			return command.Key
		}
		if command.ID == "input.puny" {
			// Ctrl+P is owned by the P8SCII input layer, not the keymap.
			return command.Key
		}
		return ""
	}
	return command.Key
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
