package input

import "testing"

func TestCanvasPosition(t *testing.T) {
	cases := []struct {
		name                      string
		cursorX, cursorY          int
		windowWidth, windowHeight int
		deviceScale               float64
		wantX, wantY              int
	}{
		{
			name: "identity at integer scale", cursorX: 128, cursorY: 128,
			windowWidth: 1024, windowHeight: 1024, deviceScale: 1,
			wantX: 128, wantY: 128,
		},
		{
			name: "identity corners", cursorX: 255, cursorY: 255,
			windowWidth: 1024, windowHeight: 1024, deviceScale: 1,
			wantX: 255, wantY: 255,
		},
		{
			name: "letterboxed 1920x1080", cursorX: 7, cursorY: 7,
			windowWidth: 1920, windowHeight: 1080, deviceScale: 1,
			wantX: 0, wantY: 0,
		},
		{
			name: "letterboxed center", cursorX: 128, cursorY: 128,
			windowWidth: 1920, windowHeight: 1080, deviceScale: 1,
			wantX: 128, wantY: 128,
		},
		{
			name: "wide window", cursorX: 128, cursorY: 128,
			windowWidth: 1280, windowHeight: 720, deviceScale: 1,
			wantX: 128, wantY: 128,
		},
		{
			name: "hidpi window", cursorX: 128, cursorY: 128,
			windowWidth: 1280, windowHeight: 720, deviceScale: 1.5,
			wantX: 128, wantY: 128,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotX, gotY := CanvasPosition(tc.cursorX, tc.cursorY, tc.windowWidth, tc.windowHeight, tc.deviceScale, 256, 256)
			if gotX != tc.wantX || gotY != tc.wantY {
				t.Errorf("CanvasPosition() = (%d, %d), want (%d, %d)", gotX, gotY, tc.wantX, tc.wantY)
			}
		})
	}
}

func TestCanvasPositionDefaults(t *testing.T) {
	if x, y := CanvasPosition(10, 20, 0, 0, 1, 256, 256); x != 10 || y != 20 {
		t.Errorf("zero window = (%d, %d), want the input position", x, y)
	}
	if x, y := CanvasPosition(10, 20, 1024, 1024, 0, 256, 256); x != 10 || y != 20 {
		t.Errorf("zero device scale = (%d, %d), want (10, 20)", x, y)
	}
	if x, y := CanvasPosition(10, 20, 1024, 1024, 1, 0, 0); x != 10 || y != 20 {
		t.Errorf("zero canvas = (%d, %d), want the input position", x, y)
	}
}
