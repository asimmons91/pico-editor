// Package app owns the application state and the Ebitengine game loop.
package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/asimmons91/pico-editor/internal/cart"
	"github.com/asimmons91/pico-editor/internal/config"
	"github.com/asimmons91/pico-editor/internal/editor"
	"github.com/asimmons91/pico-editor/internal/input"
	"github.com/asimmons91/pico-editor/internal/lsp"
	"github.com/asimmons91/pico-editor/internal/render"
	"github.com/asimmons91/pico-editor/internal/vim"
)

// Config carries the dependencies the game needs. Later milestones extend it
// rather than rewiring main.
type Config struct {
	// Logger receives diagnostics. A nil logger discards them.
	Logger *slog.Logger
	// Clipboard defaults to the host clipboard when nil.
	Clipboard input.Clipboard
	// Now supplies wall-clock time for cursor blink. Nil uses time.Now.
	Now func() time.Time
	// DebugCharset forces the M1 charset screen instead of the editor.
	DebugCharset bool
	// File opens a .lua or .p8 file, or a workspace directory, in the
	// Vim-backed editor. It is mutually exclusive with DebugCharset.
	File string
	// Workspace overrides the workspace root. Nil derives it from File or the
	// current directory.
	Workspace string
	// EnableLSP starts the in-process pico-lsp service.
	EnableLSP bool
	// Version is the editor version sent to pico-lsp in clientInfo.
	Version string
}

type clipboardOp uint8

const (
	clipboardRead clipboardOp = iota
	clipboardWrite
)

type clipboardResult struct {
	kind clipboardOp
	text string
	err  error
}

type saveResult struct {
	job *SaveJob
	err error
}

type pendingSave struct {
	buffer  *SessionBuffer
	action  vim.Action
	options SaveOptions
	saveAll bool
}

type dialogContext struct {
	kind    DialogKind
	pending *pendingSave
}

// disableRecoveryForTests is set by the package TestMain so one test's journal
// cannot appear in another test's startup prompt.
var disableRecoveryForTests bool

// Game is the root Ebitengine game. It owns no package-level state.
type Game struct {
	logger           *slog.Logger
	baseLogger       *slog.Logger
	step             func() error
	fatalErr         error
	source           input.Source
	clipboard        input.Clipboard
	clipboardResults chan clipboardResult
	renderer         *Renderer
	charset          *Charset
	session          *Session
	saveResults      chan saveResult
	saveBusy         bool
	pendingQuit      bool
	saveAll          bool
	saveAllQuit      bool
	forceQuit        bool
	saveAllQueue     []*SessionBuffer
	dialogContext    *dialogContext
	mouseDown        bool
	startupErr       error
	blink            *Blink
	scale            int
	now              func() time.Time
	firstFrame       bool
	screenWidth      int
	screenHeight     int
	lsp              *lsp.Service
	lspCtx           context.Context
	lspCancel        context.CancelFunc
	lspAllowed       bool
	lspVersion       string
	commands         []Command
	palette          *Palette
	settings         config.Settings
	settingsErr      error
	settingsWarnings []config.Warning
	configStamp      string
	configErrText    string
	notifications    *NotificationCenter
	recovery         *recoveryStore
	recoveryList     *RecoveryList
	pendingRecovery  *RecoveryList
	recoveryDue      time.Time
	knownBuffers     map[string]bool
}

// New returns a game configured by cfg.
func New(cfg Config) *Game {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	baseLogger := logger
	logger = logger.With("component", "app")
	clipboard := cfg.Clipboard
	if clipboard == nil {
		clipboard = input.NewSystemClipboard()
	}
	charset := NewCharset()
	if cfg.DebugCharset {
		charset.Overlay = true
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	settings := config.DefaultSettings()
	game := &Game{
		logger:           logger,
		baseLogger:       baseLogger,
		clipboard:        clipboard,
		clipboardResults: make(chan clipboardResult, 8),
		saveResults:      make(chan saveResult, 4),
		renderer:         NewRenderer(CanvasWidth, CanvasHeight, logger),
		charset:          charset,
		blink:            NewBlinkInterval(now(), time.Duration(settings.Editor.BlinkMS)*time.Millisecond),
		scale:            DefaultScale,
		now:              now,
		settings:         settings,
		notifications:    newNotificationCenter(now),
		lspAllowed:       cfg.EnableLSP,
		lspVersion:       cfg.Version,
		knownBuffers:     make(map[string]bool),
	}
	game.source = input.NewEbitenSource(game.mapCursor)
	game.step = game.updateFrame
	if cfg.DebugCharset {
		if cfg.File != "" {
			game.startupErr = fmt.Errorf("cannot combine --debug-charset with a file argument")
		}
	} else {
		root := cfg.Workspace
		path := cfg.File
		if path != "" {
			absolute, err := filepath.Abs(path)
			if err != nil {
				game.startupErr = err
			} else if info, statErr := os.Stat(absolute); statErr != nil {
				game.startupErr = statErr
			} else if info.IsDir() {
				root = absolute
				path = ""
			} else {
				path = absolute
				if root == "" {
					root = filepath.Dir(absolute)
				}
			}
		}
		if root == "" {
			root, _ = os.Getwd()
		}
		if root != "" {
			loaded, warnings, err := config.LoadSettings(root)
			game.settings = loaded
			game.settingsWarnings = warnings
			game.settingsErr = err
			game.configStamp = configStamp(root)
			game.notifications.configure(
				time.Duration(loaded.Notifications.TimeoutMS)*time.Millisecond,
				loaded.Notifications.History,
			)
			game.blink.SetInterval(now(), time.Duration(loaded.Editor.BlinkMS)*time.Millisecond)
		}
		if game.startupErr == nil {
			session, openErr := NewSessionAtOptions(root, path, logger, SessionOptions{Settings: game.settings})
			if openErr != nil {
				game.startupErr = openErr
			} else {
				game.session = session
				session.notify = game.pushNotification
				if session.Status != "" {
					severity := NotifyInfo
					if session.StatusError {
						severity = NotifyError
					}
					game.notifications.Push(session.Status, severity, true)
				}
				for _, warning := range game.settingsWarnings {
					game.notifications.Push(warning.String(), NotifyWarning, true)
				}
				if game.settingsErr != nil {
					game.logger.Error("config", "error", game.settingsErr)
				}
				if cfg.EnableLSP && game.settings.LSP.Enabled {
					game.startLSP(session, cfg.Version)
				}
				if game.settings.Recovery.Enabled && !disableRecoveryForTests {
					game.initRecovery()
				}
			}
		}
	}
	game.commands = game.buildCommands()
	if game.settingsErr != nil && game.session != nil {
		game.showConfigError()
	} else {
		game.activateRecovery()
	}
	game.syncKnownBuffers()
	game.redraw()
	return game
}

// InitialScale resolves the configured scale against the current monitor. main
// calls it before creating the window; tests do not need Ebitengine.
func (g *Game) InitialScale() int {
	requested := g.settings.Editor.Scale
	if monitor := ebiten.Monitor(); monitor != nil {
		width, height := monitor.Size()
		return ResolveScale(requested, width, height)
	}
	return ResolveScale(requested, 0, 0)
}

func (g *Game) pushNotification(message string, severity NotificationSeverity, toast bool) {
	if g.notifications != nil {
		g.notifications.Push(message, severity, toast)
	}
}

func (g *Game) notifyError(title string, err error) {
	if err == nil {
		return
	}
	g.logger.Error(title, "error", err)
	g.pushNotification(title+": "+shortError(err), NotifyError, true)
}

func (g *Game) initRecovery() {
	if g.session == nil {
		return
	}
	store := g.recovery
	if store == nil {
		store = newRecoveryStore(config.RecoveryDir(), g.baseLogger.With("component", "recovery"), g.now)
	}
	entries, err := store.List()
	if err != nil {
		g.logger.Warn("recovery list", "error", err)
		return
	}
	g.recovery = store
	if len(entries) > 0 {
		g.pendingRecovery = &RecoveryList{Entries: entries}
	}
}

func (g *Game) activateRecovery() {
	if g.pendingRecovery == nil {
		return
	}
	g.recoveryList = g.pendingRecovery
	g.pendingRecovery = nil
}

func (g *Game) showConfigError() {
	if g.session == nil || g.settingsErr == nil {
		return
	}
	g.session.ShowDialog(DialogConfigError, "Config error",
		g.settingsErr.Error()+"\nUse the built-in defaults or quit?",
		[]DialogOption{{Label: "Use defaults"}, {Label: "Quit"}}, 0)
}

func (g *Game) syncKnownBuffers() {
	if g.session == nil {
		return
	}
	current := make(map[string]bool, len(g.session.Buffers))
	for _, buffer := range g.session.Buffers {
		if buffer == nil || buffer.Editor == nil {
			continue
		}
		path := buffer.Editor.Buffer.Path
		if path == "" {
			continue
		}
		current[path] = true
	}
	if g.recovery != nil {
		for path := range g.knownBuffers {
			if !current[path] {
				if err := g.recovery.Delete(path); err != nil {
					g.logger.Warn("recovery delete", "path", path, "error", err)
				}
			}
		}
	}
	g.knownBuffers = current
}

func (g *Game) startLSP(session *Session, version string) {
	service := lsp.New(lsp.Options{
		WorkspaceRoot: session.Workspace.Root,
		Settings:      lspSettingsFromConfig(g.settings.LSP),
		Log:           g.baseLogger,
		Version:       version,
		Debounce:      time.Duration(g.settings.LSP.DebounceMS) * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	g.lsp = service
	g.lspCtx = ctx
	g.lspCancel = cancel
	session.AttachLSP(service)
	go func() {
		if err := service.Start(ctx); err != nil {
			g.logger.Warn("lsp start", "error", err)
		}
	}()
}

func (g *Game) stopLSP() {
	if g.lsp == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	_ = g.lsp.Stop(ctx)
	cancel()
	if g.lspCancel != nil {
		g.lspCancel()
	}
	g.lsp = nil
	g.lspCtx = nil
	g.lspCancel = nil
}

func (g *Game) restartLSP() {
	if g.lsp == nil || g.lspCtx == nil {
		return
	}
	service := g.lsp
	ctx := g.lspCtx
	go func() {
		if err := service.Restart(ctx); err != nil {
			g.logger.Warn("lsp restart", "error", err)
		}
	}()
}

// StartupError reports a file or flag problem found while constructing the
// game. Callers check it before starting the Ebitengine loop.
func (g *Game) StartupError() error {
	return g.startupErr
}

// Update advances one tick. Ebitengine calls Update on its game goroutine, so
// frame panics are recovered here and returned as an error to stop RunGame.
func (g *Game) Update() (err error) {
	if g.fatalErr != nil {
		return g.fatalErr
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			g.fatalErr = g.reportPanic("update", recovered)
			err = g.fatalErr
		}
	}()
	return g.step()
}

func (g *Game) updateFrame() error {
	err := g.update()
	if errors.Is(err, ebiten.Termination) {
		g.clearOpenRecovery()
		g.stopLSP()
	}
	return err
}

// clearOpenRecovery deletes journal entries for buffers that are closing on a
// normal or forced quit. A crash never reaches this path, so journals remain.
func (g *Game) clearOpenRecovery() {
	if g.recovery == nil || g.session == nil {
		return
	}
	for _, buffer := range g.session.Buffers {
		if buffer == nil || buffer.Editor == nil || buffer.Editor.Buffer.Path == "" {
			continue
		}
		if err := g.recovery.Delete(buffer.Editor.Buffer.Path); err != nil {
			g.logger.Warn("recovery delete", "path", buffer.Editor.Buffer.Path, "error", err)
		}
	}
}

func (g *Game) update() error {
	now := g.now()
	changed := g.blink.Update(now)
	if g.notifications != nil && g.notifications.Tick(now) {
		changed = true
	}
	saveChanged := false
	if quit, polled := g.pollSave(); quit {
		return ebiten.Termination
	} else if polled {
		changed = true
		saveChanged = true
	}
	if g.shouldTerminate() {
		return ebiten.Termination
	}
	if clipboardChanged, quit := g.pollClipboard(); quit {
		return ebiten.Termination
	} else if clipboardChanged {
		changed = true
	}
	if g.session != nil && g.session.PollLSP(time.Now()) {
		changed = true
	}
	if quit, closeChanged := g.handleWindowClose(ebiten.IsWindowBeingClosed()); quit {
		return ebiten.Termination
	} else if closeChanged {
		changed = true
	}
	events := g.source.Poll()
	if len(events) > 0 {
		wasVisible := g.blink.Visible()
		g.blink.Reset(now)
		if !wasVisible {
			changed = true
		}
	}
	for _, event := range events {
		quit, eventChanged := g.handleEvent(event)
		changed = changed || eventChanged
		if quit {
			g.logger.Info("quit requested")
			return ebiten.Termination
		}
	}
	if g.session != nil {
		if g.session.EndBatch() {
			changed = true
		}
	}
	if len(events) > 0 || saveChanged {
		g.syncKnownBuffers()
	}
	if g.maybeScheduleRecovery(now) {
		changed = true
	}
	if g.shouldTerminate() {
		return ebiten.Termination
	}
	if changed {
		g.redraw()
	}
	return nil
}

func (g *Game) handleEvent(event input.Event) (quit, changed bool) {
	switch event.Kind {
	case input.KindKeyDown:
		g.logger.Debug("key", "key", input.FormatKey(event.Key, event.Mods), "repeat", event.Repeat)
	case input.KindText:
		g.logger.Debug("text", "runes", len(event.Runes))
	case input.KindFocus:
		g.logger.Debug("focus", "focused", event.Focused)
	}
	if g.recoveryList != nil && event.Kind == input.KindKeyDown {
		return g.handleRecoveryList(event)
	}
	if g.palette != nil {
		if g.session != nil && g.session.DialogOpen() {
			// An async save conflict can open a dialog after the palette was
			// shown; let the dialog take the next event.
			g.palette = nil
		} else if event.Kind == input.KindFocus {
			// Do not swallow focus refresh while the palette is open.
			g.palette = nil
		} else {
			return g.handlePalette(event)
		}
	}
	switch event.Kind {
	case input.KindFocus:
		if g.session == nil {
			g.charset.AddLog(fmt.Sprintf("focus %t", event.Focused))
			return false, true
		}
		if !event.Focused {
			return false, g.writeRecovery()
		}
		if g.session.DialogOpen() || g.saveBusy {
			return false, false
		}
		g.checkConfigOnFocus()
		conflict := g.session.RefreshFromDisk()
		changed = true
		g.session.ForceCartInfo()
		if conflict != nil && !g.session.DialogOpen() {
			g.showExternalConflict(conflict)
		}
		return false, changed
	case input.KindText:
		if g.session != nil {
			return g.handleSessionEvent(event)
		}
		g.charset.AddLog(fmt.Sprintf("text %q", string(event.Runes)))
		return false, true
	case input.KindMouse:
		if g.session == nil {
			return false, false
		}
		pressed := event.Buttons&input.MouseLeft != 0
		if pressed && !g.mouseDown {
			changed = g.session.Click(event.X, event.Y)
		}
		g.mouseDown = pressed
		return false, changed
	case input.KindResize:
		return false, false
	case input.KindKeyDown:
		return g.handleKey(event)
	default:
		return false, false
	}
}

func (g *Game) handleWindowClose(closing bool) (quit, changed bool) {
	if !closing {
		return false, false
	}
	if g.session == nil {
		// The charset debug screen has no unsaved state; let the platform close
		// request terminate the game.
		return true, false
	}
	if g.session.CanQuitAll() {
		if g.saveBusy {
			g.pendingQuit = true
			g.forceQuit = false
			return false, true
		}
		return true, false
	}
	if g.session.DialogOpen() {
		return false, false
	}
	g.showQuitDialog()
	return false, true
}

func (g *Game) handleSessionEvent(event input.Event) (quit, changed bool) {
	_, changed, actions := g.session.HandleEvent(event)
	if dialogChanged := g.handleDialogResult(); dialogChanged {
		changed = true
	}
	if quit, actionChanged := g.applyActions(actions); quit {
		return true, true
	} else {
		changed = changed || actionChanged
	}
	return false, changed
}

func (g *Game) handleKey(event input.Event) (quit, changed bool) {
	key := event.Key
	if g.session == nil && key == input.KeyQ && event.Mods&input.ModCtrl != 0 && !event.Repeat {
		return true, false
	}
	if g.session != nil {
		if g.session.GlyphPicker != nil {
			return g.handleSessionEvent(event)
		}
		if g.session.DialogOpen() {
			return g.handleSessionEvent(event)
		}
		if !event.Repeat && (key == input.KeyEsc || key == input.KeyEnter) && g.notifications != nil && g.notifications.HasError() && g.session.CanDismissNotification() {
			if g.notifications.Dismiss() {
				return false, true
			}
		}
		if handled, quit := g.tryKeymap(event); handled {
			return quit, true
		}
		if event.Mods&input.ModCtrl != 0 && !event.Repeat {
			switch {
			case key == input.KeyV && event.Mods&input.ModShift != 0 && event.Mods&(input.ModAlt|input.ModMeta) == 0:
				changed, actions := g.session.ClipboardPaste()
				quit, actionChanged := g.applyActions(actions)
				return quit, changed || actionChanged
			case key == input.KeyC && event.Mods&input.ModShift != 0 && event.Mods&(input.ModAlt|input.ModMeta) == 0:
				changed, actions := g.session.VisualClipboardYank()
				quit, actionChanged := g.applyActions(actions)
				return quit, changed || actionChanged
			case key == input.KeyTab && event.Mods&(input.ModAlt|input.ModMeta) == 0 && g.session.Vim.Mode == vim.ModeInsert:
				// Ctrl+Tab remains available while inserting so a counted
				// insert can cross tabs; the keymap owns normal-mode tab
				// switching.
				delta := 1
				if event.Mods&input.ModShift != 0 {
					delta = -1
				}
				g.session.QueueTabSwitch(delta)
				return false, true
			}
		}
		return g.handleSessionEvent(event)
	}

	if event.Repeat {
		if event.Mods == 0 {
			switch key {
			case input.KeyLeft:
				g.charset.Move(-1, 0)
				return false, true
			case input.KeyRight:
				g.charset.Move(1, 0)
				return false, true
			case input.KeyUp:
				g.charset.Move(0, -1)
				return false, true
			case input.KeyDown:
				g.charset.Move(0, 1)
				return false, true
			}
		}
		g.charset.AddLog(fmt.Sprintf("key %s repeat", input.FormatKey(key, event.Mods)))
		return false, true
	}

	if event.Mods == 0 {
		switch {
		case key >= input.Key1 && key <= input.Key4:
			g.setScale(int(key-input.Key1) + 1)
			g.charset.AddLog(fmt.Sprintf("scale %d", g.scale))
			return false, true
		case key == input.KeyW:
			g.charset.Overlay = !g.charset.Overlay
			g.charset.AddLog(fmt.Sprintf("overlay %t", g.charset.Overlay))
			return false, true
		case key == input.KeyP:
			g.charset.CycleBackground()
			g.charset.AddLog(fmt.Sprintf("background %d", g.charset.Background))
			return false, true
		case key == input.KeyLeft:
			g.charset.Move(-1, 0)
			return false, true
		case key == input.KeyRight:
			g.charset.Move(1, 0)
			return false, true
		case key == input.KeyUp:
			g.charset.Move(0, -1)
			return false, true
		case key == input.KeyDown:
			g.charset.Move(0, 1)
			return false, true
		case key == input.KeyEsc:
			g.charset.Log = nil
			return false, true
		}
	}

	g.charset.AddLog(fmt.Sprintf("key %s", input.FormatKey(key, event.Mods)))
	return false, true
}

func (g *Game) applyActions(actions []vim.Action) (quit, changed bool) {
	for _, action := range actions {
		changed = true
		switch action.Kind {
		case vim.ActionSave:
			if saveQuit, _ := g.startSaveFor(g.session.CurrentBuffer(), action, SaveOptions{}); saveQuit {
				return true, true
			}
		case vim.ActionQuit:
			if g.saveBusy {
				g.pendingQuit = true
				g.forceQuit = false
				return false, true
			}
			sessionQuit, _ := g.session.RequestQuit(action.Force)
			if sessionQuit {
				return true, true
			}
			g.writeRecovery()
		case vim.ActionOpen:
			g.session.OpenPath(action.Path, action.Force)
		case vim.ActionSwitchBuffer:
			g.session.SwitchBuffer(action.Number, action.Delta)
		case vim.ActionListBuffers:
			g.session.ShowBufferList()
		case vim.ActionSwitchTab:
			g.session.SwitchTab(action.Delta)
		case vim.ActionHelp:
			g.session.OpenHelp(action.Topic)
		case vim.ActionClipboardRead:
			g.startClipboardRead()
		case vim.ActionClipboardWrite:
			g.startClipboardWrite(action.Text)
		case vim.ActionDiagnostics:
			g.session.ShowDiagnostics()
		case vim.ActionHover:
			g.session.RequestHover()
		case vim.ActionDefinition:
			g.session.RequestDefinition()
		case vim.ActionLSPRestart:
			g.restartLSP()
		case vim.ActionJump:
			g.session.JumpToPath(action.Path, action.Pos)
		case vim.ActionGlyphPick:
			g.session.OpenGlyphPicker()
		case vim.ActionInputMode:
			g.session.SetInputMode(action.Topic)
		case vim.ActionMessages:
			g.showMessages()
		case vim.ActionConfigReload:
			g.reloadConfig()
		}
	}
	return false, changed
}

func (g *Game) startSaveFor(buffer *SessionBuffer, action vim.Action, options SaveOptions) (quit, changed bool) {
	if buffer == nil {
		return false, false
	}
	if g.saveBusy {
		g.session.SetStatus("save already in progress", true)
		return false, true
	}
	job, err := g.session.PrepareSaveBuffer(buffer, action, options)
	if err != nil {
		g.handleSaveError(buffer, action, options, err)
		return false, true
	}
	if job == nil {
		return false, false
	}
	if job.Empty() {
		return g.session.CompleteSave(job, nil)
	}
	g.saveBusy = true
	go func() {
		writeErr := job.Write()
		g.saveResults <- saveResult{job: job, err: writeErr}
	}()
	return false, true
}

func (g *Game) handleSaveError(buffer *SessionBuffer, action vim.Action, options SaveOptions, err error) {
	g.logger.Error("save", "path", buffer.Editor.Buffer.Path, "error", err)
	switch {
	case errors.Is(err, cart.ErrReadOnly), errors.Is(err, editor.ErrReadOnly):
		g.cancelSaveAll()
		g.session.ShowErrorDetail("Cannot save", "save", buffer.Editor.Buffer.Path, err)
	case errors.Is(err, cart.ErrDeleted), errors.Is(err, fs.ErrNotExist) &&
		(errors.Is(err, editor.ErrExternalChange) || errors.Is(err, cart.ErrExternalChange)):
		g.showSaveDialog(DialogDeleted, buffer, action, options, err)
	case errors.Is(err, cart.ErrOutsideRoot), errors.Is(err, editor.ErrOutsideRoot):
		g.showSaveDialog(DialogOutsideRoot, buffer, action, options, err)
	case errors.Is(err, cart.ErrExternalChange), errors.Is(err, editor.ErrExternalChange):
		g.showSaveDialog(DialogConflict, buffer, action, options, err)
	default:
		g.cancelSaveAll()
		g.session.SetStatus(shortError(err), true)
	}
}

func (g *Game) showSaveDialog(kind DialogKind, buffer *SessionBuffer, action vim.Action, options SaveOptions, err error) {
	path := g.session.bufferName(buffer)
	pending := &pendingSave{
		buffer:  buffer,
		action:  action,
		options: options,
		saveAll: g.saveAll,
	}
	var title, message string
	var choices []DialogOption
	var defaultChoice int
	switch kind {
	case DialogDeleted:
		title = "File deleted"
		message = path + " was deleted on disk. Recreate it with the current buffer?"
		choices = []DialogOption{{Label: "Recreate"}, {Label: "Cancel"}}
		defaultChoice = 1
	case DialogOutsideRoot:
		title = "Outside workspace"
		message = err.Error() + ". Save anyway?"
		choices = []DialogOption{{Label: "Save anyway"}, {Label: "Cancel"}}
		defaultChoice = 1
	default:
		title = "File changed"
		message = path + " changed on disk. Overwrite it, reload from disk, or cancel?"
		choices = []DialogOption{{Label: "Overwrite"}, {Label: "Reload"}, {Label: "Cancel"}}
		defaultChoice = 2
	}
	g.dialogContext = &dialogContext{kind: kind, pending: pending}
	g.session.ShowDialog(kind, title, message, choices, defaultChoice)
}

func (g *Game) handleDialogResult() bool {
	if g.session == nil {
		return false
	}
	kind, choice, ok := g.session.TakeDialogResult()
	if !ok {
		return false
	}
	context := g.dialogContext
	g.dialogContext = nil
	switch kind {
	case DialogConflict:
		if context == nil || context.pending == nil {
			return true
		}
		pending := context.pending
		switch choice {
		case 0:
			options := pending.options
			options.AllowExternalChange = true
			options.Backup = true
			g.startSaveFor(pending.buffer, pending.action, options)
		case 1:
			reloaded, ok := g.reloadSessionBuffer(pending.buffer)
			if !ok {
				g.cancelSaveAll()
				return true
			}
			if pending.saveAll {
				g.nextSaveAll()
			} else if pending.action.QuitAfter {
				if quit, _ := g.session.CloseBuffer(reloaded.Editor.Buffer, true); quit {
					g.pendingQuit = true
					g.forceQuit = false
				}
			}
		default:
			g.cancelSaveAll()
		}
	case DialogDeleted:
		if context != nil && context.pending != nil && choice == 0 {
			pending := context.pending
			options := pending.options
			options.AllowExternalChange = true
			options.Backup = false
			g.startSaveFor(pending.buffer, pending.action, options)
		} else {
			g.cancelSaveAll()
		}
	case DialogOutsideRoot:
		if context != nil && context.pending != nil && choice == 0 {
			pending := context.pending
			options := pending.options
			options.AllowOutsideRoot = true
			g.startSaveFor(pending.buffer, pending.action, options)
		} else {
			g.cancelSaveAll()
		}
	case DialogQuit:
		switch choice {
		case 0:
			g.startSaveAll()
		case 1:
			g.cancelSaveAll()
			g.pendingQuit = true
			g.forceQuit = true
		default:
			g.cancelSaveAll()
			g.writeRecovery()
		}
	case DialogConfigError:
		if choice == 1 {
			g.pendingQuit = true
			g.forceQuit = true
			g.saveAll = false
			return true
		}
		g.settingsErr = nil
		g.activateRecovery()
	}
	return true
}

func (g *Game) reloadSessionBuffer(buffer *SessionBuffer) (*SessionBuffer, bool) {
	if buffer == nil {
		return nil, false
	}
	index := g.session.bufferIndex(buffer)
	if index < 0 {
		return nil, false
	}
	if err := g.session.reloadBufferAtPreserving(buffer, index); err != nil {
		g.session.ShowErrorDetail("Reload failed", "reload", buffer.Editor.Buffer.Path, err)
		return nil, false
	}
	return g.session.Buffers[index], true
}

func (g *Game) shouldTerminate() bool {
	if !g.pendingQuit || g.saveBusy {
		return false
	}
	if g.session != nil && !g.forceQuit && !g.session.CanQuitAll() {
		if g.saveAllQuit {
			// A buffer was dirtied after the previous queue drained. Save again
			// instead of exiting with unsaved changes.
			g.saveAllQuit = false
			g.saveAll = true
			g.pendingQuit = false
			g.saveAllQueue = g.session.unsavedBuffers()
			g.nextSaveAll()
			return false
		}
		// A deferred quit raced with a new edit; stay open rather than lose it.
		g.pendingQuit = false
		return false
	}
	return true
}

func (g *Game) requestQuitAll() (quit, changed bool) {
	if g.session == nil {
		return true, false
	}
	if g.session.CanQuitAll() {
		if g.saveBusy {
			g.pendingQuit = true
			g.forceQuit = false
			return false, true
		}
		return true, true
	}
	if g.session.DialogOpen() {
		return false, false
	}
	g.showQuitDialog()
	return false, true
}

func (g *Game) showQuitDialog() {
	g.dialogContext = &dialogContext{kind: DialogQuit}
	g.session.ShowDialog(DialogQuit, "Unsaved changes",
		"One or more buffers have unsaved changes.",
		[]DialogOption{{Label: "Save all"}, {Label: "Discard all"}, {Label: "Cancel"}}, 2)
}

func (g *Game) startSaveAll() {
	if g.session == nil {
		g.pendingQuit = true
		g.forceQuit = false
		return
	}
	g.saveAll = true
	g.saveAllQuit = false
	g.forceQuit = false
	g.saveAllQueue = g.session.unsavedBuffers()
	g.nextSaveAll()
}

func (g *Game) nextSaveAll() {
	if !g.saveAll || g.saveBusy {
		return
	}
	for len(g.saveAllQueue) > 0 {
		buffer := g.saveAllQueue[0]
		g.saveAllQueue = g.saveAllQueue[1:]
		if buffer == nil || !buffer.Editor.Buffer.Dirty() {
			continue
		}
		if g.session.bufferIndex(buffer) < 0 {
			continue
		}
		_, changed := g.startSaveFor(buffer, vim.Action{Kind: vim.ActionSave}, SaveOptions{})
		if changed {
			return
		}
		g.cancelSaveAll()
		return
	}
	if !g.session.CanQuitAll() {
		// Edits can land while asynchronous saves drain. Rebuild the queue so
		// Save all never quits with a dirty buffer.
		g.saveAllQueue = g.session.unsavedBuffers()
		if len(g.saveAllQueue) > 0 {
			g.nextSaveAll()
			return
		}
	}
	g.saveAll = false
	g.saveAllQuit = true
	g.pendingQuit = true
	g.forceQuit = false
}

func (g *Game) cancelSaveAll() {
	g.saveAll = false
	g.saveAllQuit = false
	g.saveAllQueue = nil
	g.forceQuit = false
}

func (g *Game) pollSave() (quit, changed bool) {
	if g.saveResults == nil || g.session == nil {
		return false, false
	}
	select {
	case result := <-g.saveResults:
		g.saveBusy = false
		quit, _ := g.session.CompleteSave(result.job, result.err)
		if result.err == nil && result.job != nil && result.job.Buffer != nil && !result.job.Buffer.Dirty() && g.recovery != nil {
			if err := g.recovery.Delete(result.job.Buffer.Path); err != nil {
				g.logger.Warn("recovery delete", "path", result.job.Buffer.Path, "error", err)
			}
		}
		if !quit && g.saveAll {
			if result.err != nil {
				g.cancelSaveAll()
			} else {
				g.nextSaveAll()
			}
		}
		return quit, true
	default:
		return false, false
	}
}

func (g *Game) startClipboardRead() {
	clipboard := g.clipboard
	results := g.clipboardResults
	go func() {
		result := clipboardResult{kind: clipboardRead}
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					result.err = fmt.Errorf("clipboard panic: %v", recovered)
				}
			}()
			result.text, result.err = clipboard.ReadText()
		}()
		results <- result
	}()
}

func (g *Game) startClipboardWrite(text string) {
	clipboard := g.clipboard
	results := g.clipboardResults
	go func() {
		result := clipboardResult{kind: clipboardWrite}
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					result.err = fmt.Errorf("clipboard panic: %v", recovered)
				}
			}()
			result.err = clipboard.WriteText(text)
		}()
		results <- result
	}()
}

func (g *Game) pollClipboard() (changed, quit bool) {
	if g.clipboardResults == nil {
		return false, false
	}
	for {
		select {
		case result := <-g.clipboardResults:
			changed = true
			if g.session == nil {
				continue
			}
			if result.kind == clipboardRead {
				_, actions := g.session.CompleteClipboardRead(input.NormalizeNewlines(result.text), result.err)
				if actionQuit, _ := g.applyActions(actions); actionQuit {
					return true, true
				}
			} else if result.err != nil {
				g.session.SetStatus("clipboard: "+shortError(result.err), true)
			}
		default:
			return changed, false
		}
	}
}

func shortError(err error) string {
	if errors.Is(err, input.ErrUnavailable) {
		return "clipboard unavailable"
	}
	return truncate(err.Error(), 48)
}

func truncate(text string, limit int) string {
	if limit < 0 {
		limit = 0
	}
	count := 0
	for index := range text {
		if count == limit {
			return fmt.Sprintf("%q...", text[:index])
		}
		count++
	}
	return fmt.Sprintf("%q", text)
}

func (g *Game) setScale(scale int) {
	g.scale = scale
	ebiten.SetWindowSize(CanvasWidth*scale, CanvasHeight*scale)
}

func (g *Game) redraw() {
	if g.session != nil {
		if notification, ok := g.notifications.Active(); ok {
			g.session.DisplayMessage = notification.Message
			g.session.DisplayMessageError = notification.Severity == NotifyError
		} else {
			g.session.DisplayMessage = ""
			g.session.DisplayMessageError = false
		}
		g.session.Draw(&g.renderer.Surface, g.blink.Visible())
	} else {
		g.charset.CursorVisible = g.blink.Visible()
		g.charset.Draw(&g.renderer.Surface)
	}
	if g.recoveryList != nil {
		g.drawRecoveryList(&g.renderer.Surface)
	}
	if g.palette != nil {
		g.drawPalette(&g.renderer.Surface)
	}
	g.drawNotifications(&g.renderer.Surface)
	g.renderer.MarkDirty()
}

// Draw renders the 256x256 canvas. The renderer owns the only Ebitengine image
// that receives surface pixels.
func (g *Game) Draw(screen *ebiten.Image) {
	g.doFrame("draw", func() {
		g.renderer.Upload()
		g.renderer.Present(screen)
	})
}

// DrawFinalScreen scales the canvas by an integer factor, centers it, and
// letterboxes the remainder. Nearest-neighbor sampling keeps every canvas
// pixel sharp at any window size.
func (g *Game) DrawFinalScreen(screen ebiten.FinalScreen, offscreen *ebiten.Image, _ ebiten.GeoM) {
	g.doFrame("final screen", func() {
		g.drawFinalScreen(screen, offscreen)
		g.markFirstFrame()
	})
}

func (g *Game) markFirstFrame() {
	if g.firstFrame {
		return
	}
	g.firstFrame = true
	g.logger.Info("first-frame")
}

func (g *Game) drawFinalScreen(screen ebiten.FinalScreen, offscreen *ebiten.Image) {
	screen.Fill(render.Palette[render.DefaultBackground])

	bounds := screen.Bounds()
	g.screenWidth = bounds.Dx()
	g.screenHeight = bounds.Dy()
	scale := ScaleFor(bounds.Dx(), bounds.Dy())
	destination := CanvasRect(bounds.Dx(), bounds.Dy(), scale)

	options := &ebiten.DrawImageOptions{Filter: ebiten.FilterNearest}
	options.GeoM.Scale(float64(scale), float64(scale))
	options.GeoM.Translate(
		float64(bounds.Min.X+destination.Min.X),
		float64(bounds.Min.Y+destination.Min.Y),
	)
	screen.DrawImage(offscreen, options)
}

// doFrame runs a frame callback, recovering any panic so it becomes a fatal
// error for the next Update. Once a frame has failed, later frames are no-ops.
func (g *Game) doFrame(where string, action func()) {
	if g.fatalErr != nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			g.fatalErr = g.reportPanic(where, recovered)
		}
	}()
	action()
}

// Layout fixes the logical canvas at 256x256 device-independent pixels.
func (g *Game) Layout(_, _ int) (width, height int) {
	defer func() {
		if recovered := recover(); recovered != nil {
			g.fatalErr = g.reportPanic("layout", recovered)
			width, height = CanvasWidth, CanvasHeight
		}
	}()
	return CanvasWidth, CanvasHeight
}

// reportPanic logs a frame panic with its stack and returns the fatal error
// that Update hands back to Ebitengine, which stops RunGame and exits non-zero.
func (g *Game) reportPanic(where string, recovered any) error {
	g.logger.Error("panic",
		"where", where,
		"panic", recovered,
		"stack", string(debug.Stack()),
	)
	return fmt.Errorf("panic in %s: %v", where, recovered)
}
