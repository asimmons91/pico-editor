package input

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestEbitenKeyMappingRoundTrip(t *testing.T) {
	for _, entry := range keyNames {
		ebitenKey, ok := ebitenKeyFor(entry.key)
		if !ok {
			t.Errorf("ebitenKeyFor(%s) = false, want a mapping", entry.name)
			continue
		}
		key, ok := keyFromEbiten(ebitenKey)
		if !ok {
			t.Errorf("keyFromEbiten(%v) = false for %s", ebitenKey, entry.name)
			continue
		}
		if key != entry.key {
			t.Errorf("keyFromEbiten(ebitenKeyFor(%s)) = %s, want %s", entry.name, KeyName(key), entry.name)
		}
	}
}

func TestEbitenModifierMapping(t *testing.T) {
	cases := []struct {
		in  ebiten.Key
		out Key
	}{
		{ebiten.KeyShiftLeft, KeyShift},
		{ebiten.KeyShiftRight, KeyShift},
		{ebiten.KeyControlLeft, KeyControl},
		{ebiten.KeyControlRight, KeyControl},
		{ebiten.KeyAltLeft, KeyAlt},
		{ebiten.KeyAltRight, KeyAlt},
		{ebiten.KeyMetaLeft, KeyMeta},
		{ebiten.KeyMetaRight, KeyMeta},
	}
	for _, tc := range cases {
		got, ok := keyFromEbiten(tc.in)
		if !ok || got != tc.out {
			t.Errorf("keyFromEbiten(%v) = (%s, %t), want (%s, true)", tc.in, KeyName(got), ok, KeyName(tc.out))
		}
	}
}

func TestEbitenUnknownKey(t *testing.T) {
	if _, ok := keyFromEbiten(ebiten.Key(9999)); ok {
		t.Error("keyFromEbiten(unknown) = true, want false")
	}
	if _, ok := ebitenKeyFor(KeyUnknown); ok {
		t.Error("ebitenKeyFor(KeyUnknown) = true, want false")
	}
}

func TestEbitenKeyMappingSpotChecks(t *testing.T) {
	cases := []struct {
		in  ebiten.Key
		out Key
	}{
		{ebiten.KeyA, KeyA},
		{ebiten.KeySemicolon, KeySemicolon},
		{ebiten.KeyApostrophe, KeyApostrophe},
		{ebiten.KeyComma, KeyComma},
		{ebiten.KeyPeriod, KeyPeriod},
		{ebiten.KeySlash, KeySlash},
		{ebiten.KeyBackquote, KeyBacktick},
		{ebiten.KeyLeft, KeyLeft},
		{ebiten.KeyRight, KeyRight},
		{ebiten.KeyUp, KeyUp},
		{ebiten.KeyDown, KeyDown},
		{ebiten.KeyF10, KeyF10},
	}
	for _, tc := range cases {
		got, ok := keyFromEbiten(tc.in)
		if !ok || got != tc.out {
			t.Errorf("keyFromEbiten(%v) = (%s, %t), want (%s, true)", tc.in, KeyName(got), ok, KeyName(tc.out))
		}
		reverse, ok := ebitenKeyFor(tc.out)
		if !ok || reverse != tc.in {
			t.Errorf("ebitenKeyFor(%s) = (%v, %t), want (%v, true)", KeyName(tc.out), reverse, ok, tc.in)
		}
	}
}

func TestEbitenSourceCursorMapper(t *testing.T) {
	source := NewEbitenSource(func(x, y float64) (int, int) {
		return int(x) + 1, int(y) + 2
	})
	if x, y := source.cursorPosition(10, 20); x != 11 || y != 22 {
		t.Errorf("cursorPosition() = (%d, %d), want (11, 22)", x, y)
	}

	raw := NewEbitenSource(nil)
	if x, y := raw.cursorPosition(10.9, 20.9); x != 10 || y != 20 {
		t.Errorf("raw cursorPosition() = (%d, %d), want (10, 20)", x, y)
	}
}
