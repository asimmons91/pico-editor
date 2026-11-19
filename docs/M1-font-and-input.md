# M1 — Font, rendering, and input foundation

**Status:** Draft
**Depends on:** M0; pico-lsp release with public `pkg/p8scii`
**Blocks:** M2, M6
**Roadmap:** [ROADMAP.md](ROADMAP.md)
**Estimate:** 1–2 weeks

## Outcome

The editor can draw every P8SCII glyph pixel-perfectly at integer scales and
turn Linux keyboard, text, and clipboard input into a stable event stream.
Nothing is editable yet; the milestone proves the look and the input layer that
every later feature depends on.

**Demo:** A full-charset screen shows all 256 P8SCII glyphs with byte labels.
Pressing `1`–`4` changes the window scale; `W` overlays cell and pixel widths.
Typing and pasting prints the received keys and runes. A written spike report
records clipboard, IME, key-repeat, and non-US-layout behavior.

## Scope

### In scope

- Consuming the tagged pico-lsp `pkg/p8scii` package (mapping, widths,
  clusters).
- Checked-in 1-bit glyph data and a build-time atlas generator.
- P8SCII width table and cluster-aware text measurement.
- 16-color PICO-8 palette and a software 256×256 surface.
- Integer window scaling, HiDPI handling, letterboxing, and cursor drawing.
- Keyboard, text, modifier, key-repeat, and clipboard abstraction.
- IME and clipboard spikes using Ebitengine 2.10 APIs.
- Full-charset test screen and golden-image tests.

### Out of scope

- Text buffers, editing, search, and file IO (M2).
- Vim behavior (M3).
- Cart parsing and file tree (M4).
- pico-lsp client or language features (M5).
- Shift+letter glyph insertion, kana modes, puny mode, and glyph picker (M6).
  M1 only exposes the raw key/text events those features consume.
- User settings and keymaps (M7). Constants live in code until then.

## Deliverables

1. pico-lsp `pkg/p8scii` with `Decode`, `Encode`, `Lookup`, `Width`,
   `CellWidth`, `NextCluster`, and `PrevCluster`, tested across all 256 bytes.
2. `internal/font` with checked-in glyph data, metrics, lookup, and a generated
   atlas.
3. `tools/fontgen` plus `go generate` wiring; CI verifies the generated file is
   current.
4. `internal/render` with the PICO-8 palette, software surface, and text
   drawing.
5. `internal/input` with event types, key naming, modifier normalization, text
   input, key repeat, and clipboard access.
6. `internal/app` window scaling and the full-charset screen.
7. `docs/spikes/M1-input.md` with spike results and decisions.
8. Golden tests for every glyph at 1×, 2×, 3×, and 4×, wide-glyph occupancy,
   cursor placement, and resize math.

## Detailed requirements

### 1. Upstream `pkg/p8scii` (prerequisite)

The editor must not own a second P8SCII table. pico-lsp already has the
canonical mapping in `internal/p8scii`; M1 consumes a tagged public facade.

Required public API:

```go
package p8scii

// Decode converts P8SCII bytes to canonical Unicode text.
func Decode(code []byte) string

// Encode converts canonical Unicode text back to P8SCII bytes.
func Encode(text string) []byte

// Lookup returns the P8SCII byte for a canonical glyph string.
func Lookup(glyph string) (byte, bool)

// Width returns the cell count for a P8SCII byte: 0 for control bytes,
// 1 for bytes 16–127, and 2 for bytes 128–255.
func Width(b byte) int

// CellWidth returns the display width of the first cluster in text.
func CellWidth(text string) int

// NextCluster and PrevCluster step whole P8SCII glyphs, including
// base + VS16 sequences.
func NextCluster(text string, i int) int
func PrevCluster(text string, i int) int
```

Requirements:

- Widths come from the P8SCII byte, never from Unicode East Asian width or
  grapheme libraries.
- Bytes `0–15` are control codes with width 0 and no glyph.
- Bytes `16–127` occupy one 4px cell; bytes `128–255` occupy two 4px cells
  (8px).
- The VS16 sequence is part of the preceding glyph. `😐` and `😐️` must not
  produce two glyphs or two widths.
- The mapping table is the existing pico-lsp table. Do not add editor-local
  overrides.
- Tests round-trip every byte and every canonical glyph, including the VS16
  entries.
- pico-lsp publishes this package in `v1.6.0`; the editor pins the tag before
  this milestone starts.

### 2. Glyph data and atlas

- Data lives in `internal/font/glyphs.txt`, one record per line (values below
  are format placeholders, not the real glyphs):

  ```text
  # byte width row0 row1 row2 row3 row4 row5
  20 4 00 00 00 00 00 00
  41 4 00 00 00 00 00 00
  80 8 00 00 00 00 00 00
  ```

  Width is `4` or `8`; each row is two hex digits. Narrow glyphs use the low
  4 bits of each row. Wide glyphs use all 8 bits.

- Exactly 256 records are required, one for every P8SCII byte. Control bytes
  are present but fully blank.
- The glyph bitmaps are derived from PICO-8's CC-0 font, checked in once, and
  verified against PICO-8 0.2.7 screenshots. Record provenance in `NOTICE` and
  in a comment at the top of `glyphs.txt`. Do not extract from a PICO-8 binary
  at build time and do not import a community asset at runtime.
- `tools/fontgen`:
  - parses and validates the data,
  - rejects duplicate/missing bytes, bad widths, and rows with ink outside the
    declared width,
  - emits `internal/font/atlas_gen.go` with `var glyphs [256]Glyph`,
  - emits a packed `image.Paletted` atlas for inspection and optional upload,
  - is deterministic; `go generate ./internal/font` must reproduce the file
    byte-for-byte.
- `Glyph` is:

  ```go
  type Glyph struct {
      Width uint8    // 4 or 8 pixels
      Rows  [6]uint8 // top row first
  }
  ```

- Metrics are constants: cell width 4, wide width 8, cell height 6, line
  advance 6. There is no baseline offset in V1; text draws from the top-left of
  the cell.

### 3. PICO-8 palette

Use the canonical 16-color palette:

```text
0  #000000  1  #1D2B53  2  #7E2553  3  #008751
4  #AB5236  5  #5F574F  6  #C2C3C7  7  #FFF1E8
8  #FF004D  9  #FFA300  10 #FFEC27  11 #00E436
12 #29ADFF  13 #83769C  14 #FF77A8  15 #FFCCAA
```

- `internal/render.Palette` is a `[16]color.RGBA`.
- Index 0 is the default background; index 7 is the default text color.
- No colors outside this table are allowed in V1. Do not add alpha blending;
  UI emphasis uses palette swaps or inversion.

### 4. Software surface

Rendering goes to a 256×256 indexed surface, not directly to Ebitengine. This
keeps golden tests headless and guarantees pixel-exact output.

```go
type Surface struct {
    Pix    []uint8 // row-major palette indices
    Width  int     // 256
    Height int     // 256
}

func (s *Surface) Clear(index uint8)
func (s *Surface) Set(x, y int, index uint8)
func (s *Surface) FillRect(x, y, w, h int, index uint8)
func (s *Surface) InvertRect(x, y, w, h int)
func (s *Surface) DrawGlyph(x, y int, g font.Glyph, fg, bg uint8) int
func (s *Surface) DrawText(x, y int, text string, fg, bg uint8) int
```

- Bounds checks are mandatory; drawing outside the surface is a no-op, not a
  panic.
- `DrawText` consumes clusters, advances 4px for narrow and 8px for wide
  glyphs, and returns the advanced cell width.
- Unknown runes draw a 4×6 replacement box, advance 4px, and log once per rune
  at debug level. Never panic on unknown input.
- Newlines are not handled by `DrawText`; callers draw a line at a time.
- The surface is expanded to RGBA through the palette and uploaded to one
  `ebiten.Image` with `WritePixels` only when the surface is dirty. Reallocate
  the image only when the size changes (it does not in V1).

### 5. Window scaling and HiDPI

- `Layout` always returns `256, 256`.
- On startup:
  - `scale = 4` unless configured to `0` (auto) in M7; in M1 use a constant.
  - `ebiten.SetWindowSize(256*scale, 256*scale)`.
  - `ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)`.
  - `ebiten.SetWindowTitle("pico-editor")`.
- Every `DrawFinalScreen`:
  - read the final screen bounds (physical pixels),
  - compute `scale = max(1, min(width/256, height/256))`,
  - center at `((width-256*scale)/2, (height-256*scale)/2)`,
  - draw the 256×256 offscreen with `Filter: ebiten.FilterNearest` and an
    integer `GeoM.Scale`,
  - fill the remaining area with palette index 0.
- Scaling lives in the final-screen pass because `Game.Draw` only sees the
  256×256 logical screen and Ebitengine's default final-screen filter is a box
  filter. Do not scale a second time in `Draw`.
- `ebiten.DeviceScaleFactor()` is respected by Ebitengine's window sizing; do
  not multiply coordinates by it manually. Add a table test for scale math,
  including 1.25, 1.5, and 2.0 device factors.
- Resizing must never use fractional scaling, linear filtering, or a viewport
  that crops the canvas.

### 6. Cursor drawing

- `render.CursorStyle` has `Block` and `Bar`.
- Normal mode draws a block cursor: invert the cells covered by the glyph under
  the cursor. A wide glyph inverts eight pixels; a narrow glyph inverts four.
- Insert mode draws a two-pixel-wide bar at the cursor's left edge, full cell
  height.
- Blink is owned by the app: visible for 500ms, hidden for 500ms, reset to
  visible on any input. The renderer only draws the requested style and
  visibility.
- At end-of-line, draw a half-width block after the last glyph so the cursor is
  reachable.

### 7. Input abstraction

Create `internal/input` with a small, testable event stream:

```go
type Mods uint8

const (
    ModShift Mods = 1 << iota
    ModCtrl
    ModAlt
    ModMeta
)

type Key int // KeyA..KeyZ, Key0..Key9, KeyF1..KeyF12, names below

type Event struct {
    Kind  Kind // KeyDown, Text, Focus, Resize, Mouse
    Key   Key
    Mods  Mods
    Runes []rune
    Repeat bool
}

type Source interface {
    Poll() []Event
}
```

Requirements:

- Key events come from `inpututil.IsKeyJustPressed` and
  `ebiten.IsKeyPressed` for modifiers. Key names are stable US-layout names;
  `input.KeyName` and `input.ParseKey` round-trip the strings used in
  `config.toml`.
- Text events come from `ebiten.AppendInputChars`. Do not synthesize text from
  key codes when the OS provides it; non-US layouts and dead keys must work.
- A printable key can produce both a key event and a text event. The input layer
  emits both; the editor decides which to consume. This is required so
  normal-mode Vim keeps using key events while insert mode uses text.
- Shift+letter detection uses the physical key plus `ModShift`. M1 only records
  the event; M6 maps it to P8SCII glyphs.
- Key repeat: Ebitengine reports key state, not OS repeat events. Implement a
  repeat timer in `internal/input`: first repeat after 400ms, then every 50ms
  while the key is held. Emit `Repeat: true`. Do not repeat modifier keys,
  `esc`, or text runes.
- Focus loss clears all pressed-key and repeat state.
- Mouse events are defined but only used later for tab/tree clicks. Include
  position in logical canvas pixels and buttons.
- The input package has no knowledge of Vim, P8SCII modes, or UI widgets.

### 8. Clipboard

Ebitengine 2.10 has no clipboard API. Use `golang.design/x/clipboard` (pure Go
on Linux, X11 and Wayland backends) behind a small interface:

```go
type Clipboard interface {
    ReadText() (string, error)
    WriteText(string) error
}
```

- Initialize lazily on first use, not at startup.
- On headless or unsupported environments, return a typed "unavailable" error;
  the UI degrades to internal registers.
- Read text as UTF-8. Strip a leading UTF-8 BOM. Normalize CRLF to LF at the
  call site, not inside the clipboard package.
- Never write binary data.
- The M1 spike verifies both an X11 session and a Wayland session (including
  XWayland) and records the package version and limitations.

### 9. IME spike

Ebitengine 2.10 exposes committed text through `AppendInputChars` but no
preedit or candidate-window API. The spike must answer:

1. Does `AppendInputChars` deliver committed text under ibus and fcitx5?
2. Is the candidate window positioned acceptably by the compositor?
3. Does focus loss cancel an in-progress composition?
4. Is there any supported way to position the candidate window?

The V1 decision is expected to be: accept committed text only; do not promise
preedit rendering; P8SCII kana input does not depend on the IME. Record the
result in `docs/spikes/M1-input.md`. If committed text is unreliable, define a
fallback (compose in an editor popup) before M6 starts.

### 10. Full-charset screen

- 16×16 grid of 16×16-pixel tiles.
- Each tile draws the glyph at `(x+4, y+2)` and the two-digit uppercase hex byte
  at `(x+2, y+9)` using the 4×6 font. Wide glyphs use the full 8px and still fit
  inside the tile.
- Arrow keys move a selection; `1`–`4` set the window scale; `W` toggles a
  width overlay (cell count and pixel width); `P` cycles the background through
  all 16 palette colors for contrast checks.
- The screen is temporary scaffolding but is retained behind
  `--debug-charset` for manual verification in later milestones.

### 11. Spike report

`docs/spikes/M1-input.md` records:

- Ebitengine, clipboard, and OS versions.
- Clipboard behavior on X11 and Wayland.
- IME results for ibus and fcitx5.
- Key-repeat behavior and the chosen timer values.
- Non-US layout behavior for Shift+letter and punctuation.
- Any fallback or known limitation, with the milestone that must address it.

## Architecture and files

```text
internal/font/
  font.go              Glyph, metrics, lookup
  glyphs.txt           checked-in 1-bit data
  atlas_gen.go         generated by tools/fontgen
  font_test.go
tools/fontgen/main.go  parser, validator, generator
internal/render/
  palette.go           canonical 16-color palette
  surface.go           indexed 256×256 surface
  text.go              cluster-aware text drawing
  cursor.go            block/bar cursor
internal/input/
  event.go             Event, Kind, Mods
  keys.go              key names, parse, repeat timer
  clipboard.go         clipboard interface and x/clipboard adapter
internal/app/
  app.go               game loop, input poll, surface upload
  window.go            integer scale and letterbox math
  charset.go           full-charset debug screen
docs/spikes/M1-input.md
```

## Interfaces and state

```go
type Glyph struct {
    Width uint8
    Rows  [6]uint8
}

func Lookup(glyph string) (Glyph, bool)
func CellWidth(text string) int

type Renderer struct {
    Surface render.Surface
    Image   *ebiten.Image
}

func (r *Renderer) Present(screen *ebiten.Image, outsideW, outsideH int)
```

The app holds one `Renderer` and one `input.Source`. No other package writes to
the Ebitengine image.

## Error handling

- Unknown glyphs draw a replacement box and log once; they never panic.
- Clipboard failures are returned to the caller and surfaced as a notification
  in M3/M6.
- Invalid key names in `ParseKey` return an error with the accepted names.
- A failed surface upload is fatal for the frame; log and exit cleanly rather
  than drawing a stale frame forever.
- Missing or malformed font data fails the build/test, never the run.

## Testing

### Automated

- `pkg/p8scii`: all 256 bytes round-trip; widths; VS16 clusters; invalid UTF-8
  handling matches pico-lsp.
- `internal/font`: exactly 256 records; no ink outside width; generated file
  matches `go generate`; lookup for every byte.
- `internal/render`: golden PNGs of the full-charset screen at 1×, 2×, 3×, and
  4×; wide-glyph occupancy; cursor over narrow, wide, and end-of-line positions.
- `internal/app`: scale math table; letterbox offsets; no fractional scale.
- `internal/input`: key-name round-trip; modifier parsing; repeat timing;
  focus-loss reset; clipboard unavailable path.
- `go test -race ./...` passes.

### Manual

- Charset screen on X11 and Wayland at 2×, 3×, and 4×.
- Compare every glyph against PICO-8 0.2.7 at 4×; record any mismatch and fix
  the data before exit.
- Copy from a terminal and paste into a debug text field on both backends.
- Type with a non-US layout and with an IME active.
- Resize the window to 1×, 5×, and a non-integer size; the canvas stays sharp
  and centered.

## Exit criteria

- [ ] pico-lsp exposes and tags `pkg/p8scii`; the editor imports it with no
      duplicated mapping.
- [ ] Every printable P8SCII glyph renders pixel-perfect at 2×, 3×, and 4×.
- [ ] Wide glyphs occupy two cells and cursor motion matches.
- [ ] The 16-color palette matches the canonical values.
- [ ] Shift, Ctrl, Alt, Meta, text input, and key repeat behave correctly on
      Linux.
- [ ] Window resizing never blurs or misaligns the grid.
- [ ] Clipboard works on X11 and Wayland or has a documented fallback.
- [ ] IME limitations are recorded and accepted for V1.
- [ ] Golden tests run without a GPU or display.
- [ ] `mise run check` passes.

## Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Glyph data is wrong or incomplete | Check in once with provenance; screenshot-compare all 256; fail the build on bad records. |
| Width table drifts from pico-lsp | Widths live in `pkg/p8scii`; the editor never owns a second table. |
| VS16 sequences split incorrectly | Cluster helpers in `pkg/p8scii`; property tests over every VS16 glyph. |
| Clipboard is flaky on Wayland | Isolate behind an interface; degrade to internal registers; document backend behavior. |
| IME preedit cannot be positioned | Accept committed text only; do not promise preedit; P8SCII kana input is independent. |
| Key repeat differs from OS expectations | Central timer with fixed 400ms/50ms values; tune once against PICO-8 and Vim feel. |
| Ebitengine pre-release temptation | Pin `v2.10.6`; no alpha APIs in M1. |
