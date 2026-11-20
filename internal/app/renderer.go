package app

import (
	"log/slog"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/asimmons91/pico-editor/internal/render"
)

// Renderer owns the indexed software surface and its single Ebitengine image.
// The image is created lazily on the game goroutine so unit tests stay
// headless.
type Renderer struct {
	Surface render.Surface
	Image   *ebiten.Image

	pixels []byte
	dirty  bool
}

// NewRenderer returns a cleared renderer of the given size.
func NewRenderer(width, height int, logger *slog.Logger) *Renderer {
	surface := render.NewSurface(width, height)
	surface.Logger = logger
	return &Renderer{
		Surface: *surface,
		pixels:  make([]byte, width*height*4),
		dirty:   true,
	}
}

// MarkDirty requests an upload on the next frame.
func (r *Renderer) MarkDirty() {
	r.dirty = true
}

// Upload expands the palette indices to RGBA and writes them to the image.
func (r *Renderer) Upload() {
	if !r.dirty {
		return
	}
	if r.Image == nil {
		r.Image = ebiten.NewImage(r.Surface.Width, r.Surface.Height)
	}
	for i, index := range r.Surface.Pix {
		c := render.Palette[index&0x0f]
		r.pixels[i*4+0] = c.R
		r.pixels[i*4+1] = c.G
		r.pixels[i*4+2] = c.B
		r.pixels[i*4+3] = c.A
	}
	r.Image.WritePixels(r.pixels)
	r.dirty = false
}

// Present draws the uploaded image at 1:1 into the 256x256 logical screen.
func (r *Renderer) Present(target *ebiten.Image) {
	if r.Image == nil {
		return
	}
	target.DrawImage(r.Image, nil)
}
