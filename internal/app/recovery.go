package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/asimmons91/pico-editor/internal/cart"
	"github.com/asimmons91/pico-editor/internal/editor"
	"github.com/asimmons91/pico-editor/internal/font"
	"github.com/asimmons91/pico-editor/internal/fsutil"
	"github.com/asimmons91/pico-editor/internal/input"
	"github.com/asimmons91/pico-editor/internal/render"
)

const (
	recoveryMaxEntries = 100
	recoveryMaxBytes   = 1 << 20
)

// RecoveryEntry is one journal item shown by the startup prompt.
type RecoveryEntry struct {
	Path           string    `json:"path"`
	Kind           string    `json:"kind"`
	BaseVersion    int       `json:"base_version"`
	CurrentVersion int       `json:"current_version"`
	OriginalMTime  int64     `json:"original_mtime"`
	OriginalSize   int64     `json:"original_size"`
	Timestamp      time.Time `json:"timestamp"`
	TextFile       string    `json:"text_file"`
}

func recoveryKey(path string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(path)))
	return hex.EncodeToString(sum[:])
}

// recoveryStore owns the on-disk journal. It never writes original files.
type recoveryStore struct {
	dir      string
	logger   *slog.Logger
	now      func() time.Time
	maxBytes int
	pruned   int
}

func newRecoveryStore(dir string, logger *slog.Logger, now func() time.Time) *recoveryStore {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	if now == nil {
		now = time.Now
	}
	return &recoveryStore{dir: dir, logger: logger, now: now, maxBytes: recoveryMaxBytes}
}

// Save writes the full text for one dirty buffer. Carts use FullText so the
// journal re-parses exactly like the original file. A cart splice error skips
// that buffer rather than writing partial bytes.
func (s *recoveryStore) Save(buffer *SessionBuffer) error {
	if s == nil || buffer == nil || buffer.Editor == nil {
		return nil
	}
	ed := buffer.Editor
	if !ed.Buffer.Dirty() || ed.Buffer.Path == "" {
		return nil
	}
	var text string
	if buffer.Cart != nil {
		full, err := buffer.Cart.FullText()
		if err != nil {
			return fmt.Errorf("cart recovery text for %s: %w", ed.Buffer.Path, err)
		}
		text = full
	} else {
		text = ed.Buffer.Text()
	}
	if len(text) > s.maxBytes {
		return fmt.Errorf("recovery entry for %s is larger than %d bytes", ed.Buffer.Path, s.maxBytes)
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create recovery directory: %w", err)
	}
	if err := os.Chmod(s.dir, 0o700); err != nil {
		return fmt.Errorf("secure recovery directory: %w", err)
	}
	key := recoveryKey(ed.Buffer.Path)
	textName := key + ".txt"
	meta := RecoveryEntry{
		Path:           ed.Buffer.Path,
		Kind:           "lua",
		BaseVersion:    ed.Buffer.SavedVersion,
		CurrentVersion: ed.Buffer.Version,
		Timestamp:      s.now(),
		TextFile:       textName,
	}
	if buffer.Cart != nil {
		meta.Kind = "cart"
	}
	if info, err := os.Stat(ed.Buffer.Path); err == nil {
		meta.OriginalMTime = info.ModTime().UnixNano()
		meta.OriginalSize = info.Size()
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("encode recovery metadata: %w", err)
	}
	if err := fsutil.AtomicWrite(filepath.Join(s.dir, textName), []byte(text), 0o600); err != nil {
		return err
	}
	if err := fsutil.AtomicWrite(filepath.Join(s.dir, key+".json"), data, 0o600); err != nil {
		return err
	}
	s.prune()
	return nil
}

// TakePruned reports how many old entries the last Save pruned.
func (s *recoveryStore) TakePruned() int {
	if s == nil {
		return 0
	}
	count := s.pruned
	s.pruned = 0
	return count
}

func (s *recoveryStore) prune() {
	entries, err := s.List()
	if err != nil || len(entries) <= recoveryMaxEntries {
		return
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})
	remove := len(entries) - recoveryMaxEntries
	for index := 0; index < remove; index++ {
		if err := s.Delete(entries[index].Path); err != nil {
			s.logger.Warn("recovery prune", "path", entries[index].Path, "error", err)
			continue
		}
		s.pruned++
	}
}

// Delete removes one path's journal entry. It is safe if no entry exists.
func (s *recoveryStore) Delete(path string) error {
	if s == nil || path == "" {
		return nil
	}
	key := recoveryKey(path)
	first := os.Remove(filepath.Join(s.dir, key+".json"))
	second := os.Remove(filepath.Join(s.dir, key+".txt"))
	if first != nil && !errors.Is(first, fs.ErrNotExist) {
		return first
	}
	if second != nil && !errors.Is(second, fs.ErrNotExist) {
		return second
	}
	return nil
}

// List scans the journal, skipping malformed or incomplete entries.
func (s *recoveryStore) List() ([]RecoveryEntry, error) {
	if s == nil {
		return nil, nil
	}
	items, err := os.ReadDir(s.dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	entries := make([]RecoveryEntry, 0, len(items))
	for _, item := range items {
		if item.IsDir() || !strings.HasSuffix(item.Name(), ".json") {
			continue
		}
		path := filepath.Join(s.dir, item.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			s.logger.Warn("recovery metadata", "path", path, "error", err)
			continue
		}
		var entry RecoveryEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			s.logger.Warn("recovery metadata", "path", path, "error", err)
			continue
		}
		if entry.Path == "" || entry.TextFile == "" {
			continue
		}
		if filepath.Base(entry.TextFile) != entry.TextFile {
			s.logger.Warn("recovery metadata", "path", path, "error", "text file escapes the journal directory")
			continue
		}
		if _, err := os.Stat(filepath.Join(s.dir, entry.TextFile)); err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})
	return entries, nil
}

// Load returns the journal text, refusing entries over the size cap.
func (s *recoveryStore) Load(entry RecoveryEntry) (string, error) {
	path := filepath.Join(s.dir, entry.TextFile)
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.Size() > int64(s.maxBytes) {
		return "", fmt.Errorf("recovery entry is larger than %d bytes", s.maxBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// writeRecovery journals every dirty buffer now. It reports whether a
// notification or prune changed the UI.
func (g *Game) writeRecovery() bool {
	if g.recovery == nil || g.session == nil || !g.settings.Recovery.Enabled {
		return false
	}
	changed := false
	for _, buffer := range g.session.Buffers {
		if buffer == nil || buffer.Editor == nil || buffer.Editor.Buffer.Path == "" {
			continue
		}
		if !buffer.Editor.Buffer.Dirty() {
			continue
		}
		if err := g.recovery.Save(buffer); err != nil {
			g.notifyError("Recovery write failed", err)
			changed = true
			continue
		}
		if g.journaled == nil {
			g.journaled = make(map[string]bool)
		}
		g.journaled[buffer.Editor.Buffer.Path] = true
	}
	if pruned := g.recovery.TakePruned(); pruned > 0 {
		g.pushNotification(fmt.Sprintf("pruned %d old recovery entries", pruned), NotifyWarning, true)
		changed = true
	}
	g.recoveryDue = time.Time{}
	return changed
}

// maybeScheduleRecovery writes once after recovery.interval_ms of dirty idle
// state. A successful write does not reschedule itself; the next edit changes
// the dirty signature and starts a new idle window.
func (g *Game) maybeScheduleRecovery(now time.Time) bool {
	if g.recovery == nil || g.session == nil || !g.settings.Recovery.Enabled {
		g.recoveryDue = time.Time{}
		g.recoverySignature = 0
		return false
	}
	interval := time.Duration(g.settings.Recovery.IntervalMS) * time.Millisecond
	if interval <= 0 {
		return false
	}
	signature := g.dirtySignature()
	if signature == 0 {
		g.recoveryDue = time.Time{}
		g.recoverySignature = 0
		return false
	}
	if signature != g.recoverySignature {
		g.recoverySignature = signature
		g.recoveryDue = now.Add(interval)
		return false
	}
	if g.recoveryDue.IsZero() || now.Before(g.recoveryDue) {
		return false
	}
	changed := g.writeRecovery()
	g.recoveryDue = time.Time{}
	return changed
}

func (g *Game) dirtySignature() int64 {
	if g.session == nil {
		return 0
	}
	signature := int64(0)
	for _, buffer := range g.session.Buffers {
		if buffer == nil || buffer.Editor == nil || !buffer.Editor.Buffer.Dirty() {
			continue
		}
		signature = signature*131 + int64(buffer.Editor.Buffer.Version+1)
	}
	return signature
}

// RecoveryList is the startup modal journal browser.
type RecoveryList struct {
	Entries  []RecoveryEntry
	Selected int
	Top      int
}

// handleRecoveryList consumes keys while the startup journal prompt is open.
func (g *Game) handleRecoveryList(ev input.Event) (quit, changed bool) {
	list := g.recoveryList
	if list == nil || ev.Kind != input.KindKeyDown {
		return false, false
	}
	switch ev.Key {
	case input.KeyUp:
		list.Selected = max(0, list.Selected-1)
		list.ensureVisible()
	case input.KeyDown:
		list.Selected = min(len(list.Entries)-1, list.Selected+1)
		list.ensureVisible()
	case input.KeyPageUp:
		list.Selected = max(0, list.Selected-10)
		list.ensureVisible()
	case input.KeyPageDown:
		list.Selected = min(len(list.Entries)-1, list.Selected+10)
		list.ensureVisible()
	case input.KeyR, input.KeyEnter:
		g.recoverEntry(list.Selected)
	case input.KeyD:
		g.discardRecovery(list.Selected)
	case input.KeyL, input.KeyEsc:
		list.removeSelected()
	}
	if len(list.Entries) == 0 {
		g.recoveryList = nil
	}
	return false, true
}

func (l *RecoveryList) ensureVisible() {
	if l.Selected < l.Top {
		l.Top = l.Selected
	}
	if l.Selected >= l.Top+8 {
		l.Top = l.Selected - 8 + 1
	}
	l.Top = max(0, l.Top)
}

func (l *RecoveryList) removeSelected() {
	if l == nil || l.Selected < 0 || l.Selected >= len(l.Entries) {
		return
	}
	l.Entries = append(l.Entries[:l.Selected], l.Entries[l.Selected+1:]...)
	if l.Selected >= len(l.Entries) {
		l.Selected = max(0, len(l.Entries)-1)
	}
	l.ensureVisible()
}

func (g *Game) recoverEntry(index int) {
	list := g.recoveryList
	if list == nil || index < 0 || index >= len(list.Entries) {
		return
	}
	entry := list.Entries[index]
	if g.recovery == nil {
		return
	}
	text, err := g.recovery.Load(entry)
	if err != nil {
		g.notifyError("Recover failed", err)
		return
	}
	if err := g.session.Recover(entry, text); err != nil {
		g.notifyError("Recover failed", err)
		return
	}
	// Keep the journal on disk until the recovered buffer is saved, closed, or
	// explicitly discarded. Mark it session-owned so cleanup can do that.
	if g.journaled == nil {
		g.journaled = make(map[string]bool)
	}
	g.journaled[entry.Path] = true
	list.removeSelected()
	g.syncKnownBuffers()
}

func (g *Game) discardRecovery(index int) {
	list := g.recoveryList
	if list == nil || index < 0 || index >= len(list.Entries) {
		return
	}
	entry := list.Entries[index]
	if g.recovery != nil {
		if err := g.recovery.Delete(entry.Path); err != nil {
			g.notifyError("Recovery discard failed", err)
			return
		}
		delete(g.journaled, entry.Path)
	}
	list.removeSelected()
}

func (g *Game) drawRecoveryList(surface *render.Surface) {
	list := g.recoveryList
	if list == nil {
		return
	}
	const widthCells = 62
	rows := min(8, max(1, len(list.Entries)))
	width := widthCells * font.NarrowWidth
	height := (rows + 3) * font.LineAdvance
	x := (surface.Width - width) / 2
	y := (surface.Height - height) / 2
	drawPanel(surface, x, y, width, height)
	surface.DrawText(x+font.NarrowWidth, y, " RECOVER UNSAVED WORK ", panelTitle, render.DefaultBackground)
	for row := 0; row < rows; row++ {
		index := list.Top + row
		if index >= len(list.Entries) {
			break
		}
		entry := list.Entries[index]
		rowY := y + (row+1)*font.LineAdvance
		bg := uint8(render.DefaultBackground)
		fg := uint8(7)
		if index == list.Selected {
			bg = selectionBackground
			fg = selectionForeground
			surface.FillRect(x+1, rowY, width-2, font.CellHeight, bg)
		}
		text := fmt.Sprintf("%s  %s", entry.Timestamp.Format("2006-01-02 15:04"), entry.Path)
		surface.DrawText(x+font.NarrowWidth, rowY, clipCells(text, widthCells-2), fg, bg)
	}
	footerY := y + (rows+1)*font.LineAdvance
	surface.DrawText(x+font.NarrowWidth, footerY, "R recover   D discard   L later   Up/Down", 6, render.DefaultBackground)
}

// Recover opens journal text as a dirty buffer. It never writes the original
// file; the user must save explicitly.
func (s *Session) Recover(entry RecoveryEntry, text string) error {
	var buffer *SessionBuffer
	if entry.Kind == "cart" || isCartPath(entry.Path) {
		document, err := cart.OpenData(entry.Path, []byte(text))
		if err != nil {
			return err
		}
		if original, err := cart.Open(entry.Path); err == nil {
			document.MarkSavedCodeText(original.Code.Text())
		} else {
			document.MarkSavedCodeText("")
		}
		if s.Workspace != nil {
			document.SetWorkspaceRoot(s.Workspace.Root)
		}
		buffer = &SessionBuffer{Editor: editor.NewEditor(document.Code), Cart: document}
		buffer.TabViews = make([]cart.ViewState, len(document.Tabs))
	} else {
		original, err := editor.Open(entry.Path)
		if err == nil {
			saved := original.Text()
			if err := original.SetText(text); err != nil {
				return err
			}
			original.MarkSavedText(saved)
			if s.Workspace != nil {
				original.SetWorkspaceRoot(s.Workspace.Root)
			}
			buffer = &SessionBuffer{Editor: editor.NewEditor(original)}
		} else {
			plain := editor.New(entry.Path)
			if err := plain.SetText(text); err != nil {
				return err
			}
			plain.MarkSavedText("")
			if s.Workspace != nil {
				plain.SetWorkspaceRoot(s.Workspace.Root)
			}
			buffer = &SessionBuffer{Editor: editor.NewEditor(plain)}
		}
	}
	index := -1
	for i, candidate := range s.Buffers {
		if candidate.Editor.Buffer.Path == buffer.Editor.Buffer.Path {
			index = i
			break
		}
	}
	if index >= 0 {
		s.closeLSPDocument(s.Buffers[index])
		s.Buffers[index] = buffer
		s.Current = index
	} else if current := s.CurrentBuffer(); current != nil && current.Editor.Buffer.Path == "" && !current.Editor.Buffer.Dirty() {
		s.Buffers[s.Current] = buffer
	} else {
		s.Buffers = append(s.Buffers, buffer)
		s.Current = len(s.Buffers) - 1
	}
	s.Tree.Visible = false
	s.Tree.Focused = false
	s.prepareBuffer(buffer)
	s.SetStatus("recovered unsaved changes", false)
	return nil
}
