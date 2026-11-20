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
