package app

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/asimmons91/pico-editor/internal/render"
)

func TestCharsetGolden(t *testing.T) {
	for _, scale := range []int{1, 2, 3, 4} {
		t.Run(fmt.Sprintf("%dx", scale), func(t *testing.T) {
			charset := NewCharset()
			surface := render.NewSurface(CanvasWidth, CanvasHeight)
			charset.Draw(surface)
			img := scaleSurface(surface, scale)

			var encoded bytes.Buffer
			encoder := png.Encoder{CompressionLevel: png.BestCompression}
			if err := encoder.Encode(&encoded, img); err != nil {
				t.Fatalf("encode golden: %v", err)
			}

			path := filepath.Join("testdata", fmt.Sprintf("charset_%dx.png", scale))
			if os.Getenv("UPDATE_GOLDEN") != "" {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("create golden directory: %v", err)
				}
				if err := os.WriteFile(path, encoded.Bytes(), 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				return
			}

			wantData, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden (run UPDATE_GOLDEN=1 go test ./internal/app): %v", err)
			}
			want, err := png.Decode(bytes.NewReader(wantData))
			if err != nil {
				t.Fatalf("decode golden: %v", err)
			}
			if !bytes.Equal(encoded.Bytes(), wantData) {
				t.Errorf("%s is stale; run UPDATE_GOLDEN=1 go test ./internal/app", path)
			}
			if img.Bounds() != want.Bounds() {
				t.Errorf("golden bounds = %v, want %v", want.Bounds(), img.Bounds())
			}
		})
	}
}

func scaleSurface(surface *render.Surface, scale int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, surface.Width*scale, surface.Height*scale))
	for y := 0; y < surface.Height; y++ {
		for x := 0; x < surface.Width; x++ {
			c := render.Palette[surface.At(x, y)&0x0f]
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					img.SetRGBA(x*scale+dx, y*scale+dy, c)
				}
			}
		}
	}
	return img
}
