package app

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/asimmons91/pico-lsp/pkg/glyphs"

	"github.com/asimmons91/pico-editor/internal/config"
	"github.com/asimmons91/pico-editor/internal/lsp"
)

func lspSettingsFromConfig(settings config.LSPSettings) lsp.Settings {
	out := lsp.DefaultSettings()
	out.Glyphs = make([]glyphs.Definition, 0, len(settings.Glyphs))
	for _, item := range settings.Glyphs {
		definition := glyphs.Definition{
			Name:        item.Name,
			Glyph:       item.Glyph,
			Text:        item.Text,
			Context:     glyphs.Context(item.Context),
			Aliases:     append([]string(nil), item.Aliases...),
			Description: item.Description,
		}
		if item.Byte != nil {
			value := *item.Byte
			definition.Byte = &value
		}
		out.Glyphs = append(out.Glyphs, definition)
	}
	return out
}

// configStamp summarizes both config files so focus refresh only reparses when
// one of them changed.
func configStamp(root string) string {
	var parts []string
	for _, path := range []string{config.GlobalPath(), config.WorkspacePath(root)} {
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			parts = append(parts, path+"|-")
			continue
		}
		parts = append(parts, fmt.Sprintf("%s|%d|%d", path, info.ModTime().UnixNano(), info.Size()))
	}
	return strings.Join(parts, ";")
}

func (g *Game) configRoot() string {
	if g.session != nil && g.session.Workspace != nil {
		return g.session.Workspace.Root
	}
	root, _ := os.Getwd()
	return root
}

// reloadConfig implements the config.reload command and :reload. It always
// reparses both layers; a malformed file keeps the last good settings.
func (g *Game) reloadConfig() bool {
	return g.loadConfig(true)
}

// checkConfigOnFocus reparses when either config file changed since the last
// load. Full file watching remains post-V1.
func (g *Game) checkConfigOnFocus() bool {
	root := g.configRoot()
	if root == "" || configStamp(root) == g.configStamp {
		return false
	}
	return g.loadConfig(true)
}

func (g *Game) loadConfig(notifyWarnings bool) bool {
	root := g.configRoot()
	if root == "" {
		return false
	}
	settings, warnings, err := config.LoadSettings(root)
	if err != nil {
		if g.configErrText != err.Error() {
			g.configErrText = err.Error()
			g.notifyError("Config reload failed", err)
		}
		g.configStamp = configStamp(root)
		return true
	}
	g.configErrText = ""
	g.configStamp = configStamp(root)
	g.settingsErr = nil
	g.settingsWarnings = warnings
	g.applySettings(settings, warnings, notifyWarnings)
	return true
}

func (g *Game) applySettings(settings config.Settings, warnings []config.Warning, notifyWarnings bool) {
	settings, keymapWarnings := sanitizeKeymapCommands(settings, g.commands)
	warnings = append(warnings, keymapWarnings...)
	old := g.settings
	g.settings = settings
	if g.notifications != nil {
		g.notifications.configure(
			time.Duration(settings.Notifications.TimeoutMS)*time.Millisecond,
			settings.Notifications.History,
		)
	}
	if g.blink != nil {
		g.blink.SetInterval(g.now(), time.Duration(settings.Editor.BlinkMS)*time.Millisecond)
	}
	if g.session != nil {
		g.session.ApplySettings(settings)
	}
	if settings.Recovery.Enabled && g.recovery == nil {
		base := g.baseLogger
		if base == nil {
			base = g.logger
		}
		g.recovery = newRecoveryStore(config.RecoveryDir(), base.With("component", "recovery"), g.now)
	}
	if !settings.Recovery.Enabled {
		g.recoveryDue = time.Time{}
	}
	g.recoverySignature = 0
	if notifyWarnings {
		for _, warning := range warnings {
			g.pushNotification(warning.String(), NotifyWarning, true)
		}
	}
	if old.Editor.Scale != settings.Editor.Scale {
		g.pushNotification("restart pico-editor to apply the new window scale", NotifyInfo, true)
	}
	g.applyLSPConfig(old.LSP, settings.LSP)
}

func (g *Game) applyLSPConfig(old, next config.LSPSettings) {
	if !g.lspAllowed || g.session == nil {
		return
	}
	switch {
	case !next.Enabled && g.lsp != nil:
		g.stopLSP()
		g.session.DetachLSP()
		g.pushNotification("pico-lsp disabled", NotifyInfo, true)
	case next.Enabled && g.lsp == nil:
		g.startLSP(g.session, g.lspVersion)
		g.pushNotification("pico-lsp enabled", NotifyInfo, true)
	case next.Enabled && g.lsp != nil:
		if !reflect.DeepEqual(old.Glyphs, next.Glyphs) || old.DebounceMS != next.DebounceMS {
			g.stopLSP()
			g.session.DetachLSP()
			g.startLSP(g.session, g.lspVersion)
		}
	}
}

// DetachLSP clears the language UI and caches after a deliberate stop.
func (s *Session) DetachLSP() {
	s.LSP = nil
	s.LSPStatus = lsp.StatusOff
	s.LSPMessage = ""
	s.LSPMessageError = false
	s.LSPTokens = make(map[string]lsp.TokenSet)
	s.LSPDiagnostics = make(map[string][]lsp.Diagnostic)
	s.LSPCartInfo = make(map[string]lsp.CartInfo)
	s.LSPGlyphs = nil
	s.Completion = nil
	s.Hover = nil
	s.Signature = nil
	s.DiagList = nil
	s.GlyphPicker = nil
	s.glyphPickerPending = false
	s.completionDue = time.Time{}
	s.signatureDue = time.Time{}
	s.tokenDue = time.Time{}
	s.cartInfoDue = time.Time{}
	for _, buffer := range s.Buffers {
		buffer.LSPOpened = false
		buffer.LSPCartAsked = -1
	}
}
