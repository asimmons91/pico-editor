package vim

import (
	"unicode"
	"unicode/utf8"

	"github.com/asimmons91/pico-editor/internal/editor"
	"github.com/asimmons91/pico-editor/internal/input"
)

// ActionKind identifies work the application owns, such as file IO, buffer
// switching, and clipboard access. Vim never performs those operations itself.
type ActionKind uint8

const (
	ActionSave ActionKind = iota + 1
	ActionQuit
	ActionOpen
	ActionSwitchBuffer
	ActionListBuffers
	ActionSwitchTab
	ActionHelp
	ActionClipboardWrite
	ActionClipboardRead
)

func (k ActionKind) String() string {
	switch k {
	case ActionSave:
		return "save"
	case ActionQuit:
		return "quit"
	case ActionOpen:
		return "open"
	case ActionSwitchBuffer:
		return "switch-buffer"
	case ActionListBuffers:
		return "list-buffers"
	case ActionSwitchTab:
		return "switch-tab"
	case ActionHelp:
		return "help"
	case ActionClipboardWrite:
		return "clipboard-write"
	case ActionClipboardRead:
		return "clipboard-read"
	default:
		return "none"
	}
}

// Action is one application-level effect requested by an ex command or a
// clipboard register. The app applies actions on the Update goroutine.
type Action struct {
	Kind ActionKind
	// Path is set for ActionOpen.
	Path string
	// Number is the 1-based buffer number for ActionSwitchBuffer.
	Number int
	// Delta is the buffer or tab step for ActionSwitchBuffer/ActionSwitchTab.
	Delta int
	// Force requests discarding changes for ActionQuit/ActionOpen.
	Force bool
	// OnlyIfDirty implements :x for ActionSave.
	OnlyIfDirty bool
	// QuitAfter implements :wq and :x.
	QuitAfter bool
	// Topic is set for ActionHelp.
	Topic string
	// Text and Register are set for clipboard actions. Register is '+' or '*'.
	Text     string
	Register byte
}

// Result is the state delta produced by one input event.
type Result struct {
	Mode    Mode
	Status  string
	Notify  string
	Error   bool
	Bell    bool
	Actions []Action
}

type pendingKind uint8

const (
	pendNone pendingKind = iota
	pendG
	pendZ
	pendFind
	pendReplace
	pendTextObject
	pendRegister
)

type pendingState struct {
	kind  pendingKind
	inner bool
	find  findSpec
	count int
}

type findSpec struct {
	char    rune
	forward bool
	till    bool
	line    int
	raw     int
	hasRaw  bool
}

type insertEntry uint8

const (
	entryInsert insertEntry = iota
	entryAppend
	entryFirstNonBlank
	entryAppendEOL
	entryOpenBelow
	entryOpenAbove
)

type insertState struct {
	active       bool
	count        int
	entry        insertEntry
	start        editor.Pos
	startHistory int
	block        *blockInsert
	pendingDot   *changeRecord
}

type blockInsert struct {
	first       int
	last        int
	cell        int
	appendMode  bool
	originals   []string
	historyLen  int
	changeBlock bool
	left        int
	right       int
}

type commandKind uint8

const (
	commandEx commandKind = iota
	commandSearchForward
	commandSearchBackward
)

type commandLineState struct {
	active       bool
	kind         commandKind
	text         []rune
	cursor       int
	savedCursor  editor.Pos
	savedTopLine int
	historyPos   int
	draft        string
	preRange     string
}

type searchState struct {
	pattern   string
	forward   bool
	highlight bool
	history   []string
}

type visualState struct {
	anchor editor.Pos
	cursor editor.Pos
	kind   editor.SelectionKind
}

// Vim is one session's modal editing state. It survives buffer switches, so
// registers, search history, and the last change are not tied to a file.
type Vim struct {
	Mode     Mode
	Count    int
	Operator Operator
	Register byte

	opCount    int
	pending    pendingState
	lastFind   findSpec
	lastFindOK bool
	registers  registerStore
	insert     insertState
	cmdline    commandLineState
	search     searchState
	exHistory  []string
	visual     visualState
	lastVisual visualState
	lastChange changeRecord
	changeSet  bool
	jumps      jumpList

	lastClipboardRegister byte
	clipboardBusy         bool
	pendingPaste          *pasteRequest
	lastVisualRange       [2]int
	hasVisualRange        bool

	actions      []Action
	notify       string
	notifyError  bool
	bell         bool
	replaying    bool
	suppressText int
}

type pasteRequest struct {
	before       bool
	count        int
	register     byte
	visual       bool
	editor       *editor.Editor
	pos          editor.Pos
	selection    editor.Selection
	hasSelection bool
}

// New returns an empty normal-mode session.
func New() *Vim {
	return &Vim{Mode: ModeNormal, registers: newRegisterStore()}
}

// Handle consumes one normalized input event and returns the resulting mode,
// notifications, and application actions.
func (v *Vim) Handle(ev input.Event, ed *editor.Editor) Result {
	v.actions = nil
	v.notify = ""
	v.notifyError = false
	v.bell = false
	if ed == nil {
		return v.finish()
	}

	// A physical key emits both a key event and committed text. When a
	// printable key is consumed as a normal/visual command, its text must not
	// leak into a mode the command just entered (i, cw, :, /, r, f, ...).
	if ev.Kind == input.KindText && v.suppressText > 0 {
		if len(ev.Runes) <= v.suppressText {
			v.suppressText -= len(ev.Runes)
			return v.finish()
		}
		ev.Runes = ev.Runes[v.suppressText:]
		v.suppressText = 0
	}

	switch v.Mode {
	case ModeInsert:
		v.handleInsert(ev, ed)
	case ModeCommandLine:
		v.handleCommandLine(ev, ed)
	default:
		v.handleNormal(ev, ed)
	}

	if v.Mode != ModeInsert {
		v.clampNormal(ed)
	}
	return v.finish()
}

// EndBatch clears any unconsumed text suppression at a frame boundary. The
// app calls it after draining one Poll result.
func (v *Vim) EndBatch() {
	v.suppressText = 0
}

func (v *Vim) finish() Result {
	result := Result{
		Mode:    v.Mode,
		Status:  v.Mode.String(),
		Notify:  v.notify,
		Error:   v.notifyError,
		Bell:    v.bell,
		Actions: append([]Action(nil), v.actions...),
	}
	v.actions = nil
	return result
}

func (v *Vim) queue(action Action) {
	v.actions = append(v.actions, action)
}

func (v *Vim) message(text string) {
	v.notify = text
	v.notifyError = true
}

func (v *Vim) info(text string) {
	v.notify = text
	v.notifyError = false
}

func (v *Vim) ringBell() {
	v.bell = true
}

func (v *Vim) handleNormal(ev input.Event, ed *editor.Editor) {
	if v.pending.kind != pendNone {
		v.handlePending(ev, ed)
		return
	}
	if v.Operator != OpNone {
		v.handleOperatorPending(ev, ed)
		return
	}

	switch ev.Kind {
	case input.KindText:
		// Normal and visual modes ignore text events. Character-reading
		// commands are resolved above from key events, which carry layout
		// independent US key identities.
		return
	case input.KindKeyDown:
	default:
		return
	}

	key, mods := ev.Key, ev.Mods
	if isEscape(key, mods) {
		v.cancel(ed)
		return
	}
	if mods&input.ModCtrl != 0 {
		if key == input.KeyV {
			if v.Mode == ModeVisualBlock {
				v.leaveVisual(ed)
			} else {
				v.enterVisual(ed, ModeVisualBlock)
			}
			return
		}
		v.handleSpecialKey(key, mods, ed)
		return
	}
	switch key {
	case input.KeyEnter, input.KeySpace, input.KeyBackspace, input.KeyLeft,
		input.KeyRight, input.KeyUp, input.KeyDown, input.KeyHome, input.KeyEnd,
		input.KeyPageUp, input.KeyPageDown:
		v.handleSpecialKey(key, mods, ed)
		return
	}

	r, printable := runeForKey(key, mods)
	if !printable {
		v.handleSpecialKey(key, mods, ed)
		return
	}
	v.suppressText++
	if r >= '0' && r <= '9' {
		if r == '0' && v.Count == 0 {
			v.executeNormalRune(r, ed)
			return
		}
		v.pushCount(int(r - '0'))
		return
	}

	if v.Mode.Visual() {
		v.handleVisualRune(r, ed)
		return
	}
	v.executeNormalRune(r, ed)
}

func (v *Vim) executeNormalRune(r rune, ed *editor.Editor) {
	switch r {
	case 'i':
		v.enterInsert(ed, false, false)
	case 'a':
		v.enterInsert(ed, true, false)
	case 'I':
		v.enterInsert(ed, false, true)
	case 'A':
		v.enterInsertLineEnd(ed)
	case 'o':
		v.openLine(ed, true)
	case 'O':
		v.openLine(ed, false)
	case 'v':
		v.enterVisual(ed, ModeVisualChar)
	case 'V':
		v.enterVisual(ed, ModeVisualLine)
	case ':':
		v.openEx(ed, "")
	case '.':
		v.repeatChange(ed)
	case 'u':
		v.undo(ed)
	case 'g':
		v.pending = pendingState{kind: pendG}
	case 'z':
		v.pending = pendingState{kind: pendZ}
	case '"':
		v.pending = pendingState{kind: pendRegister}
	case 'd', 'c', 'y':
		v.setOperator(ed, operatorFor(r))
	case 'D':
		v.setOperatorMotion(ed, OpDelete, '$')
	case 'C':
		v.setOperatorMotion(ed, OpChange, '$')
	case 'Y':
		v.setOperatorMotion(ed, OpYank, '$')
	case 'x':
		v.deleteClusters(ed, true, false)
	case 'X':
		v.deleteClusters(ed, false, false)
	case 's':
		v.deleteClusters(ed, true, true)
	case 'S':
		v.changeWholeLines(ed)
	case 'r':
		v.pending = pendingState{kind: pendReplace, count: v.takeCount()}
	case 'J':
		v.joinLines(ed)
	case '~':
		v.toggleCase(ed)
	case '>':
		v.setOperator(ed, OpShiftRight)
	case '<':
		v.setOperator(ed, OpShiftLeft)
	case 'p':
		v.paste(ed, false)
	case 'P':
		v.paste(ed, true)
	case '/':
		v.openSearch(ed, true)
	case '?':
		v.openSearch(ed, false)
	case 'h', 'j', 'k', 'l', 'w', 'W', 'b', 'B', 'e', 'E', '0', '^', '$',
		'f', 'F', 't', 'T', ';', ',', '%', '{', '}', 'H', 'M', 'L',
		'+', '-', 'G', 'n', 'N', '*', '#':
		v.executeMotionKey(r, ed)
	default:
		v.ringBell()
	}
}

func (v *Vim) handleSpecialKey(key input.Key, mods input.Mods, ed *editor.Editor) {
	ctrl := mods&input.ModCtrl != 0
	switch {
	case key == input.KeyO && ctrl:
		v.jump(ed, -1)
	case (key == input.KeyI || key == input.KeyTab) && ctrl:
		v.jump(ed, 1)
	case key == input.KeyD && ctrl:
		v.executeMotionKey(ctrlRune('d'), ed)
	case key == input.KeyU && ctrl:
		v.executeMotionKey(ctrlRune('u'), ed)
	case key == input.KeyF && ctrl:
		v.executeMotionKey(ctrlRune('f'), ed)
	case key == input.KeyB && ctrl:
		v.executeMotionKey(ctrlRune('b'), ed)
	case key == input.KeyR && ctrl:
		v.redo(ed)
	case key == input.KeyEnter:
		v.executeMotionKey('\n', ed)
	case key == input.KeyBackspace:
		v.wrappedLeft(ed)
	case key == input.KeySpace:
		v.wrappedRight(ed)
	case key == input.KeyLeft:
		v.executeMotionKey('h', ed)
	case key == input.KeyRight:
		v.executeMotionKey('l', ed)
	case key == input.KeyUp:
		v.executeMotionKey('k', ed)
	case key == input.KeyDown:
		v.executeMotionKey('j', ed)
	case key == input.KeyHome:
		v.executeMotionKey('0', ed)
	case key == input.KeyEnd:
		v.executeMotionKey('$', ed)
	case key == input.KeyPageDown:
		v.executeMotionKey(ctrlRune('f'), ed)
	case key == input.KeyPageUp:
		v.executeMotionKey(ctrlRune('b'), ed)
	default:
		v.ringBell()
	}
}

func (v *Vim) handlePending(ev input.Event, ed *editor.Editor) {
	switch ev.Kind {
	case input.KindKeyDown:
	case input.KindText:
		if (v.pending.kind == pendFind || v.pending.kind == pendReplace) && len(ev.Runes) == 1 {
			v.resolvePendingRune(ev.Runes[0], ed)
		}
		return
	default:
		return
	}
	if isEscape(ev.Key, ev.Mods) {
		v.cancel(ed)
		return
	}
	r, ok := runeForKey(ev.Key, ev.Mods)
	if !ok {
		v.cancel(ed)
		v.ringBell()
		return
	}
	v.suppressText++
	v.resolvePendingRune(r, ed)
}

func (v *Vim) resolvePendingRune(r rune, ed *editor.Editor) {
	pending := v.pending
	v.pending = pendingState{}
	switch pending.kind {
	case pendRegister:
		v.Register = registerByte(r)
	case pendG:
		switch r {
		case 'g':
			v.doubleGMotion(ed)
		case 'v':
			v.reselectVisual(ed)
		default:
			v.cancel(ed)
			v.ringBell()
		}
	case pendZ:
		v.scrollCursor(ed, r)
	case pendFind:
		spec := pending.find
		spec.char = r
		spec.hasRaw = false
		v.lastFind = spec
		v.lastFindOK = true
		if v.Operator != OpNone {
			v.applyOperatorFind(ed, spec, pending.count)
		} else {
			v.findMotion(ed, spec, pending.count)
		}
	case pendReplace:
		v.replaceClusters(ed, r, pending.count)
	case pendTextObject:
		v.applyTextObject(ed, r, pending.inner)
	}
}

func (v *Vim) handleOperatorPending(ev input.Event, ed *editor.Editor) {
	if ev.Kind == input.KindText {
		// A pending f/F/t/T/r is handled by handlePending before this
		// function is reached. Anything else is ignored.
		return
	}
	if ev.Kind != input.KindKeyDown {
		return
	}
	if isEscape(ev.Key, ev.Mods) {
		v.cancel(ed)
		return
	}
	switch ev.Key {
	case input.KeySpace:
		v.applyOperatorMotion(ed, 'l')
		return
	case input.KeyBackspace:
		v.applyOperatorMotion(ed, 'h')
		return
	case input.KeyLeft:
		v.applyOperatorMotion(ed, 'h')
		return
	case input.KeyRight:
		v.applyOperatorMotion(ed, 'l')
		return
	case input.KeyUp:
		v.applyOperatorMotion(ed, 'k')
		return
	case input.KeyDown:
		v.applyOperatorMotion(ed, 'j')
		return
	case input.KeyHome:
		v.applyOperatorMotion(ed, '0')
		return
	case input.KeyEnd:
		v.applyOperatorMotion(ed, '$')
		return
	case input.KeyPageDown:
		v.applyOperatorMotion(ed, ctrlRune('f'))
		return
	case input.KeyPageUp:
		v.applyOperatorMotion(ed, ctrlRune('b'))
		return
	}
	if ev.Mods&input.ModCtrl != 0 {
		switch ev.Key {
		case input.KeyD:
			v.applyOperatorMotion(ed, ctrlRune('d'))
		case input.KeyU:
			v.applyOperatorMotion(ed, ctrlRune('u'))
		case input.KeyF:
			v.applyOperatorMotion(ed, ctrlRune('f'))
		case input.KeyB:
			v.applyOperatorMotion(ed, ctrlRune('b'))
		default:
			v.cancel(ed)
			v.ringBell()
		}
		return
	}
	r, ok := runeForKey(ev.Key, ev.Mods)
	if !ok {
		key := ev.Key
		if key == input.KeyEnter {
			r, ok = '\n', true
		}
	}
	if !ok {
		v.cancel(ed)
		v.ringBell()
		return
	}
	v.suppressText++
	if r >= '0' && r <= '9' {
		if r == '0' && v.Count == 0 {
			v.applyOperatorMotion(ed, '0')
			return
		}
		v.pushCount(int(r - '0'))
		return
	}
	switch r {
	case 'i', 'a':
		v.pending = pendingState{kind: pendTextObject, inner: r == 'i'}
		return
	case 'g':
		v.pending = pendingState{kind: pendG}
		return
	case 'f', 'F', 't', 'T':
		spec := findSpec{forward: r == 'f' || r == 't', till: r == 't' || r == 'T'}
		v.pending = pendingState{kind: pendFind, find: spec, count: v.takeCount()}
		return
	case ';', ',':
		if !v.lastFindOK {
			v.cancel(ed)
			v.ringBell()
			return
		}
		spec := v.lastFind
		if r == ',' {
			spec.forward = !spec.forward
		}
		v.applyOperatorFind(ed, spec, v.takeCount())
		return
	case 'd', 'c', 'y':
		if operatorFor(r) != v.Operator {
			v.cancel(ed)
			v.ringBell()
			return
		}
		v.applyLineOperator(ed)
		return
	case '>':
		if v.Operator != OpShiftRight {
			v.cancel(ed)
			v.ringBell()
			return
		}
		v.applyLineOperator(ed)
		return
	case '<':
		if v.Operator != OpShiftLeft {
			v.cancel(ed)
			v.ringBell()
			return
		}
		v.applyLineOperator(ed)
		return
	}
	v.applyOperatorMotion(ed, r)
}

func (v *Vim) setOperator(ed *editor.Editor, op Operator) {
	v.Operator = op
	v.opCount = v.takeCount()
	v.pending = pendingState{}
}

func (v *Vim) setOperatorMotion(ed *editor.Editor, op Operator, motion rune) {
	v.Operator = op
	v.opCount = v.takeCount()
	v.applyOperatorMotion(ed, motion)
}

func (v *Vim) takeCount() int {
	count := v.Count
	v.Count = 0
	if count < 1 {
		return 1
	}
	return count
}

func (v *Vim) pushCount(digit int) {
	if v.Count > 1_000_000 {
		v.Count = 1_000_000
		return
	}
	v.Count = v.Count*10 + digit
}

func (v *Vim) cancel(ed *editor.Editor) {
	v.Count = 0
	v.opCount = 0
	v.Operator = OpNone
	v.pending = pendingState{}
	v.Register = 0
	if v.Mode.Visual() {
		v.saveVisual(ed)
		v.leaveVisual(ed)
	}
}

// EndInsertMode finishes an insert session. The app uses it before switching
// buffers or cart tabs so counted inserts and dot replay cannot span tabs.
func (v *Vim) EndInsertMode(ed *editor.Editor) {
	if v.Mode == ModeInsert {
		v.finishInsert(ed)
	}
}

// ResetPending drops an armed operator, count, or visual selection without
// changing Vim mode. Dialogs call it when they take input.
func (v *Vim) ResetPending(ed *editor.Editor) {
	v.cancel(ed)
}

func (v *Vim) clampNormal(ed *editor.Editor) {
	pos := ed.Cursor.Pos
	if pos.Line < 0 || pos.Line >= len(ed.Buffer.Lines) {
		return
	}
	length := utf8.RuneCountInString(ed.Buffer.Lines[pos.Line])
	if length > 0 && pos.Rune >= length {
		pos = ed.Buffer.PrevCluster(editor.Pos{Line: pos.Line, Rune: length})
	}
	if pos != ed.Cursor.Pos {
		ed.SetCursorPreserveDesired(pos)
	}
	if v.Mode.Visual() {
		ed.Selection.Cursor = ed.Cursor.Pos
	}
}

func (v *Vim) enterVisual(ed *editor.Editor, mode Mode) {
	kind := editor.SelectionChar
	switch mode {
	case ModeVisualLine:
		kind = editor.SelectionLine
	case ModeVisualBlock:
		kind = editor.SelectionBlock
	}
	v.Mode = mode
	v.visual = visualState{anchor: ed.Cursor.Pos, cursor: ed.Cursor.Pos, kind: kind}
	ed.StartSelection(kind)
	v.Count = 0
	v.Operator = OpNone
	v.pending = pendingState{}
}

func (v *Vim) leaveVisual(ed *editor.Editor) {
	v.Mode = ModeNormal
	ed.ClearSelection()
	v.Count = 0
	v.Operator = OpNone
	v.pending = pendingState{}
}

func (v *Vim) saveVisual(ed *editor.Editor) {
	if ed.Selection.Kind == editor.SelectionNone {
		return
	}
	v.lastVisual = visualState{
		anchor: ed.Buffer.NormalizePos(ed.Selection.Anchor),
		cursor: ed.Buffer.NormalizePos(ed.Selection.Cursor),
		kind:   ed.Selection.Kind,
	}
	first, last := ed.Selection.LineRange()
	v.lastVisualRange = [2]int{first, last}
	v.hasVisualRange = true
}

func (v *Vim) reselectVisual(ed *editor.Editor) {
	if v.lastVisual.kind == editor.SelectionNone {
		v.ringBell()
		return
	}
	switch v.lastVisual.kind {
	case editor.SelectionLine:
		v.Mode = ModeVisualLine
	case editor.SelectionBlock:
		v.Mode = ModeVisualBlock
	default:
		v.Mode = ModeVisualChar
	}
	anchor := ed.Buffer.NormalizePos(v.lastVisual.anchor)
	cursor := ed.Buffer.NormalizePos(v.lastVisual.cursor)
	ed.Selection = editor.Selection{
		Anchor: anchor,
		Cursor: cursor,
		Kind:   v.lastVisual.kind,
	}
	ed.SetCursor(cursor)
	ed.Selection.Cursor = cursor
}

func (v *Vim) undo(ed *editor.Editor) {
	count := v.takeCount()
	ed.EndGroup()
	for i := 0; i < count; i++ {
		if _, ok := ed.Undo(); !ok {
			v.ringBell()
			break
		}
	}
	v.clearPendingState()
}

func (v *Vim) redo(ed *editor.Editor) {
	count := v.takeCount()
	ed.EndGroup()
	for i := 0; i < count; i++ {
		if _, ok := ed.Redo(); !ok {
			v.ringBell()
			break
		}
	}
	v.clearPendingState()
}

func (v *Vim) clearPendingState() {
	v.Count = 0
	v.opCount = 0
	v.Operator = OpNone
	v.pending = pendingState{}
}

func operatorFor(r rune) Operator {
	switch r {
	case 'd':
		return OpDelete
	case 'c':
		return OpChange
	case 'y':
		return OpYank
	case '>':
		return OpShiftRight
	case '<':
		return OpShiftLeft
	default:
		return OpNone
	}
}

func isEscape(key input.Key, mods input.Mods) bool {
	if key == input.KeyEsc {
		return true
	}
	return key == input.KeyLeftBracket && mods&input.ModCtrl != 0
}

func ctrlRune(r rune) rune {
	// Control motions use private sentinels so they cannot collide with
	// printable command runes.
	switch r {
	case 'd':
		return '\x04'
	case 'u':
		return '\x15'
	case 'f':
		return '\x06'
	case 'b':
		return '\x02'
	default:
		return 0
	}
}

func registerByte(r rune) byte {
	switch r {
	case '"':
		return 0
	case '+', '*', '_':
		return byte(r)
	}
	if r >= 'a' && r <= 'z' {
		return byte(r)
	}
	if r >= 'A' && r <= 'Z' {
		return byte(r)
	}
	if r >= '0' && r <= '9' {
		return byte(r)
	}
	return 0
}

func runeForKey(key input.Key, mods input.Mods) (rune, bool) {
	shift := mods&input.ModShift != 0
	switch key {
	case input.KeyA, input.KeyB, input.KeyC, input.KeyD, input.KeyE,
		input.KeyF, input.KeyG, input.KeyH, input.KeyI, input.KeyJ,
		input.KeyK, input.KeyL, input.KeyM, input.KeyN, input.KeyO,
		input.KeyP, input.KeyQ, input.KeyR, input.KeyS, input.KeyT,
		input.KeyU, input.KeyV, input.KeyW, input.KeyX, input.KeyY,
		input.KeyZ:
		r := rune('a' + int(key-input.KeyA))
		if shift {
			r = unicode.ToUpper(r)
		}
		return r, true
	case input.Key0, input.Key1, input.Key2, input.Key3, input.Key4,
		input.Key5, input.Key6, input.Key7, input.Key8, input.Key9:
		digit := rune('0' + int(key-input.Key0))
		if shift {
			return rune(")!@#$%^&*("[int(key-input.Key0)]), true
		}
		return digit, true
	case input.KeySpace:
		return ' ', true
	case input.KeyMinus:
		if shift {
			return '_', true
		}
		return '-', true
	case input.KeyEqual:
		if shift {
			return '+', true
		}
		return '=', true
	case input.KeyLeftBracket:
		if shift {
			return '{', true
		}
		return '[', true
	case input.KeyRightBracket:
		if shift {
			return '}', true
		}
		return ']', true
	case input.KeyBackslash:
		if shift {
			return '|', true
		}
		return '\\', true
	case input.KeySemicolon:
		if shift {
			return ':', true
		}
		return ';', true
	case input.KeyApostrophe:
		if shift {
			return '"', true
		}
		return '\'', true
	case input.KeyComma:
		if shift {
			return '<', true
		}
		return ',', true
	case input.KeyPeriod:
		if shift {
			return '>', true
		}
		return '.', true
	case input.KeySlash:
		if shift {
			return '?', true
		}
		return '/', true
	case input.KeyBacktick:
		if shift {
			return '~', true
		}
		return '`', true
	default:
		return 0, false
	}
}
