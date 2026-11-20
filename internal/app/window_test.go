package app

import (
	"image"
	"testing"
)

func TestGridGeometry(t *testing.T) {
	t.Parallel()

	if got := GridColumns * CellWidth; got != CanvasWidth {
		t.Errorf("grid width = %d, want %d", got, CanvasWidth)
	}
	if got := GridRows * CellHeight; got > CanvasHeight {
		t.Errorf("grid height = %d, want <= %d", got, CanvasHeight)
	}
}

func TestScaleFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		width, height int
		want          int
	}{
		{name: "default window", width: 1024, height: 1024, want: 4},
		{name: "full hd", width: 1920, height: 1080, want: 4},
		{name: "exact canvas", width: 256, height: 256, want: 1},
		{name: "window below canvas", width: 100, height: 100, want: 1},
		{name: "just below two", width: 511, height: 511, want: 1},
		{name: "two", width: 512, height: 512, want: 2},
		{name: "wide letterbox", width: 1920, height: 768, want: 3},
		{name: "tall letterbox", width: 768, height: 1920, want: 3},
		{name: "ultrawide", width: 3440, height: 1440, want: 5},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := ScaleFor(test.width, test.height); got != test.want {
				t.Errorf("ScaleFor(%d, %d) = %d, want %d", test.width, test.height, got, test.want)
			}
		})
	}
}

func TestAutoScale(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		width, height int
		want          int
	}{
		{name: "full hd", width: 1920, height: 1080, want: 3},
		{name: "4k", width: 3840, height: 2160, want: 6},
		{name: "720p", width: 1280, height: 720, want: 2},
		{name: "low resolution clamps up", width: 800, height: 600, want: 2},
		{name: "8k clamps down", width: 7680, height: 4320, want: 8},
		// A 1920x1080 physical monitor at a 1.25 device scale factor reports
		// 1536x864 device-independent pixels.
		{name: "fractional device scale", width: 1536, height: 864, want: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := AutoScale(test.width, test.height); got != test.want {
				t.Errorf("AutoScale(%d, %d) = %d, want %d", test.width, test.height, got, test.want)
			}
		})
	}
}

func TestResolveScale(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		requested     int
		width, height int
		want          int
	}{
		{name: "explicit scale", requested: 4, width: 1920, height: 1080, want: 4},
		{name: "automatic", requested: 0, width: 1920, height: 1080, want: 3},
		{name: "automatic without monitor", requested: 0, width: 0, height: 0, want: DefaultScale},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := ResolveScale(test.requested, test.width, test.height); got != test.want {
				t.Errorf("ResolveScale(%d, %d, %d) = %d, want %d", test.requested, test.width, test.height, got, test.want)
			}
		})
	}
}

func TestCanvasRect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		width, height int
		scale         int
		want          image.Rectangle
	}{
		{
			name:   "exact fit",
			width:  1024,
			height: 1024,
			scale:  4,
			want:   image.Rect(0, 0, 1024, 1024),
		},
		{
			name:   "letterbox top and bottom",
			width:  1920,
			height: 1080,
			scale:  4,
			want:   image.Rect(448, 28, 1472, 1052),
		},
		{
			name:   "letterbox left and right",
			width:  800,
			height: 600,
			scale:  2,
			want:   image.Rect(144, 44, 656, 556),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := CanvasRect(test.width, test.height, test.scale); got != test.want {
				t.Errorf("CanvasRect(%d, %d, %d) = %v, want %v", test.width, test.height, test.scale, got, test.want)
			}
		})
	}
}

func TestScaleForDeviceFactors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		scale       int
		device      float64
		wantScale   int
		widthPixels int
	}{
		{name: "1.25 at four", scale: 4, device: 1.25, wantScale: 5, widthPixels: 1280},
		{name: "1.5 at four", scale: 4, device: 1.5, wantScale: 6, widthPixels: 1536},
		{name: "2.0 at four", scale: 4, device: 2.0, wantScale: 8, widthPixels: 2048},
		{name: "1.25 at two", scale: 2, device: 1.25, wantScale: 2, widthPixels: 640},
		{name: "1.5 at two", scale: 2, device: 1.5, wantScale: 3, widthPixels: 768},
		{name: "2.0 at two", scale: 2, device: 2.0, wantScale: 4, widthPixels: 1024},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			physical := int(float64(CanvasWidth*test.scale) * test.device)
			if physical != test.widthPixels {
				t.Fatalf("physical width = %d, want %d", physical, test.widthPixels)
			}
			if got := ScaleFor(physical, physical); got != test.wantScale {
				t.Errorf("ScaleFor(%d, %d) = %d, want %d", physical, physical, got, test.wantScale)
			}
		})
	}
}
