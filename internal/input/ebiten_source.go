package input

import (
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// EbitenSource adapts Ebitengine's keyboard, text, focus, and mouse APIs into
// the normalized Source interface.
type EbitenSource struct {
	tracker      *Tracker
	chars        []rune
	canvasWidth  int
	canvasHeight int
}

// NewEbitenSource returns an Ebitengine-backed input source. canvasWidth and
// canvasHeight are the logical game-screen dimensions used by Layout.
func NewEbitenSource(canvasWidth, canvasHeight int) *EbitenSource {
	return &EbitenSource{
		tracker:      NewTracker(),
		canvasWidth:  canvasWidth,
		canvasHeight: canvasHeight,
	}
}

// Poll drains one frame of physical input.
func (s *EbitenSource) Poll() []Event {
	snap := Snapshot{
		Pressed: make(map[Key]bool),
		Mods:    currentMods(),
		Focused: ebiten.IsFocused(),
	}

	for _, key := range inpututil.AppendJustPressedKeys(nil) {
		if mapped, ok := keyFromEbiten(key); ok {
			snap.JustPressed = append(snap.JustPressed, mapped)
		}
	}

	for _, entry := range keyNames {
		key := entry.key
		if ebitenKey, ok := ebitenKeyFor(key); ok && ebiten.IsKeyPressed(ebitenKey) {
			snap.Pressed[key] = true
		}
	}

	s.chars = ebiten.AppendInputChars(s.chars[:0])
	if len(s.chars) > 0 {
		snap.Runes = s.chars
	}

	cursorX, cursorY := ebiten.CursorPosition()
	windowWidth, windowHeight := ebiten.WindowSize()
	deviceScale := 1.0
	if monitor := ebiten.Monitor(); monitor != nil {
		deviceScale = monitor.DeviceScaleFactor()
	}
	x, y := CanvasPosition(
		cursorX, cursorY,
		windowWidth, windowHeight,
		deviceScale,
		s.canvasWidth, s.canvasHeight,
	)
	snap.Mouse = MouseSnapshot{X: x, Y: y, Buttons: currentMouseButtons()}
	return s.tracker.Poll(time.Now(), snap)
}

func currentMods() Mods {
	var mods Mods
	if ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight) {
		mods |= ModShift
	}
	if ebiten.IsKeyPressed(ebiten.KeyControlLeft) || ebiten.IsKeyPressed(ebiten.KeyControlRight) {
		mods |= ModCtrl
	}
	if ebiten.IsKeyPressed(ebiten.KeyAltLeft) || ebiten.IsKeyPressed(ebiten.KeyAltRight) {
		mods |= ModAlt
	}
	if ebiten.IsKeyPressed(ebiten.KeyMetaLeft) || ebiten.IsKeyPressed(ebiten.KeyMetaRight) {
		mods |= ModMeta
	}
	return mods
}

func currentMouseButtons() MouseButtons {
	var buttons MouseButtons
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		buttons |= MouseLeft
	}
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) {
		buttons |= MouseRight
	}
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle) {
		buttons |= MouseMiddle
	}
	if ebiten.IsMouseButtonPressed(ebiten.MouseButton3) {
		buttons |= MouseButton4
	}
	if ebiten.IsMouseButtonPressed(ebiten.MouseButton4) {
		buttons |= MouseButton5
	}
	return buttons
}

func keyFromEbiten(key ebiten.Key) (Key, bool) {
	switch key {
	case ebiten.KeyA:
		return KeyA, true
	case ebiten.KeyB:
		return KeyB, true
	case ebiten.KeyC:
		return KeyC, true
	case ebiten.KeyD:
		return KeyD, true
	case ebiten.KeyE:
		return KeyE, true
	case ebiten.KeyF:
		return KeyF, true
	case ebiten.KeyG:
		return KeyG, true
	case ebiten.KeyH:
		return KeyH, true
	case ebiten.KeyI:
		return KeyI, true
	case ebiten.KeyJ:
		return KeyJ, true
	case ebiten.KeyK:
		return KeyK, true
	case ebiten.KeyL:
		return KeyL, true
	case ebiten.KeyM:
		return KeyM, true
	case ebiten.KeyN:
		return KeyN, true
	case ebiten.KeyO:
		return KeyO, true
	case ebiten.KeyP:
		return KeyP, true
	case ebiten.KeyQ:
		return KeyQ, true
	case ebiten.KeyR:
		return KeyR, true
	case ebiten.KeyS:
		return KeyS, true
	case ebiten.KeyT:
		return KeyT, true
	case ebiten.KeyU:
		return KeyU, true
	case ebiten.KeyV:
		return KeyV, true
	case ebiten.KeyW:
		return KeyW, true
	case ebiten.KeyX:
		return KeyX, true
	case ebiten.KeyY:
		return KeyY, true
	case ebiten.KeyZ:
		return KeyZ, true
	case ebiten.Key0:
		return Key0, true
	case ebiten.Key1:
		return Key1, true
	case ebiten.Key2:
		return Key2, true
	case ebiten.Key3:
		return Key3, true
	case ebiten.Key4:
		return Key4, true
	case ebiten.Key5:
		return Key5, true
	case ebiten.Key6:
		return Key6, true
	case ebiten.Key7:
		return Key7, true
	case ebiten.Key8:
		return Key8, true
	case ebiten.Key9:
		return Key9, true
	case ebiten.KeyF1:
		return KeyF1, true
	case ebiten.KeyF2:
		return KeyF2, true
	case ebiten.KeyF3:
		return KeyF3, true
	case ebiten.KeyF4:
		return KeyF4, true
	case ebiten.KeyF5:
		return KeyF5, true
	case ebiten.KeyF6:
		return KeyF6, true
	case ebiten.KeyF7:
		return KeyF7, true
	case ebiten.KeyF8:
		return KeyF8, true
	case ebiten.KeyF9:
		return KeyF9, true
	case ebiten.KeyF10:
		return KeyF10, true
	case ebiten.KeyF11:
		return KeyF11, true
	case ebiten.KeyF12:
		return KeyF12, true
	case ebiten.KeyEscape:
		return KeyEsc, true
	case ebiten.KeyEnter:
		return KeyEnter, true
	case ebiten.KeySpace:
		return KeySpace, true
	case ebiten.KeyTab:
		return KeyTab, true
	case ebiten.KeyBackspace:
		return KeyBackspace, true
	case ebiten.KeyDelete:
		return KeyDelete, true
	case ebiten.KeyInsert:
		return KeyInsert, true
	case ebiten.KeyHome:
		return KeyHome, true
	case ebiten.KeyEnd:
		return KeyEnd, true
	case ebiten.KeyPageUp:
		return KeyPageUp, true
	case ebiten.KeyPageDown:
		return KeyPageDown, true
	case ebiten.KeyUp:
		return KeyUp, true
	case ebiten.KeyDown:
		return KeyDown, true
	case ebiten.KeyLeft:
		return KeyLeft, true
	case ebiten.KeyRight:
		return KeyRight, true
	case ebiten.KeyShiftLeft, ebiten.KeyShiftRight:
		return KeyShift, true
	case ebiten.KeyControlLeft, ebiten.KeyControlRight:
		return KeyControl, true
	case ebiten.KeyAltLeft, ebiten.KeyAltRight:
		return KeyAlt, true
	case ebiten.KeyMetaLeft, ebiten.KeyMetaRight:
		return KeyMeta, true
	case ebiten.KeyMinus:
		return KeyMinus, true
	case ebiten.KeyEqual:
		return KeyEqual, true
	case ebiten.KeyLeftBracket:
		return KeyLeftBracket, true
	case ebiten.KeyRightBracket:
		return KeyRightBracket, true
	case ebiten.KeyBackslash:
		return KeyBackslash, true
	case ebiten.KeySemicolon:
		return KeySemicolon, true
	case ebiten.KeyApostrophe:
		return KeyApostrophe, true
	case ebiten.KeyComma:
		return KeyComma, true
	case ebiten.KeyPeriod:
		return KeyPeriod, true
	case ebiten.KeySlash:
		return KeySlash, true
	case ebiten.KeyBackquote:
		return KeyBacktick, true
	default:
		return KeyUnknown, false
	}
}

func ebitenKeyFor(key Key) (ebiten.Key, bool) {
	switch key {
	case KeyA:
		return ebiten.KeyA, true
	case KeyB:
		return ebiten.KeyB, true
	case KeyC:
		return ebiten.KeyC, true
	case KeyD:
		return ebiten.KeyD, true
	case KeyE:
		return ebiten.KeyE, true
	case KeyF:
		return ebiten.KeyF, true
	case KeyG:
		return ebiten.KeyG, true
	case KeyH:
		return ebiten.KeyH, true
	case KeyI:
		return ebiten.KeyI, true
	case KeyJ:
		return ebiten.KeyJ, true
	case KeyK:
		return ebiten.KeyK, true
	case KeyL:
		return ebiten.KeyL, true
	case KeyM:
		return ebiten.KeyM, true
	case KeyN:
		return ebiten.KeyN, true
	case KeyO:
		return ebiten.KeyO, true
	case KeyP:
		return ebiten.KeyP, true
	case KeyQ:
		return ebiten.KeyQ, true
	case KeyR:
		return ebiten.KeyR, true
	case KeyS:
		return ebiten.KeyS, true
	case KeyT:
		return ebiten.KeyT, true
	case KeyU:
		return ebiten.KeyU, true
	case KeyV:
		return ebiten.KeyV, true
	case KeyW:
		return ebiten.KeyW, true
	case KeyX:
		return ebiten.KeyX, true
	case KeyY:
		return ebiten.KeyY, true
	case KeyZ:
		return ebiten.KeyZ, true
	case Key0:
		return ebiten.Key0, true
	case Key1:
		return ebiten.Key1, true
	case Key2:
		return ebiten.Key2, true
	case Key3:
		return ebiten.Key3, true
	case Key4:
		return ebiten.Key4, true
	case Key5:
		return ebiten.Key5, true
	case Key6:
		return ebiten.Key6, true
	case Key7:
		return ebiten.Key7, true
	case Key8:
		return ebiten.Key8, true
	case Key9:
		return ebiten.Key9, true
	case KeyF1:
		return ebiten.KeyF1, true
	case KeyF2:
		return ebiten.KeyF2, true
	case KeyF3:
		return ebiten.KeyF3, true
	case KeyF4:
		return ebiten.KeyF4, true
	case KeyF5:
		return ebiten.KeyF5, true
	case KeyF6:
		return ebiten.KeyF6, true
	case KeyF7:
		return ebiten.KeyF7, true
	case KeyF8:
		return ebiten.KeyF8, true
	case KeyF9:
		return ebiten.KeyF9, true
	case KeyF10:
		return ebiten.KeyF10, true
	case KeyF11:
		return ebiten.KeyF11, true
	case KeyF12:
		return ebiten.KeyF12, true
	case KeyEsc:
		return ebiten.KeyEscape, true
	case KeyEnter:
		return ebiten.KeyEnter, true
	case KeySpace:
		return ebiten.KeySpace, true
	case KeyTab:
		return ebiten.KeyTab, true
	case KeyBackspace:
		return ebiten.KeyBackspace, true
	case KeyDelete:
		return ebiten.KeyDelete, true
	case KeyInsert:
		return ebiten.KeyInsert, true
	case KeyHome:
		return ebiten.KeyHome, true
	case KeyEnd:
		return ebiten.KeyEnd, true
	case KeyPageUp:
		return ebiten.KeyPageUp, true
	case KeyPageDown:
		return ebiten.KeyPageDown, true
	case KeyUp:
		return ebiten.KeyUp, true
	case KeyDown:
		return ebiten.KeyDown, true
	case KeyLeft:
		return ebiten.KeyLeft, true
	case KeyRight:
		return ebiten.KeyRight, true
	case KeyShift:
		return ebiten.KeyShiftLeft, true
	case KeyControl:
		return ebiten.KeyControlLeft, true
	case KeyAlt:
		return ebiten.KeyAltLeft, true
	case KeyMeta:
		return ebiten.KeyMetaLeft, true
	case KeyMinus:
		return ebiten.KeyMinus, true
	case KeyEqual:
		return ebiten.KeyEqual, true
	case KeyLeftBracket:
		return ebiten.KeyLeftBracket, true
	case KeyRightBracket:
		return ebiten.KeyRightBracket, true
	case KeyBackslash:
		return ebiten.KeyBackslash, true
	case KeySemicolon:
		return ebiten.KeySemicolon, true
	case KeyApostrophe:
		return ebiten.KeyApostrophe, true
	case KeyComma:
		return ebiten.KeyComma, true
	case KeyPeriod:
		return ebiten.KeyPeriod, true
	case KeySlash:
		return ebiten.KeySlash, true
	case KeyBacktick:
		return ebiten.KeyBackquote, true
	default:
		return 0, false
	}
}
