# M1 input and rendering spikes

**Date:** 2026-11-20
**Status:** Automated checks complete; the interactive checks below remain for
the milestone owner (see [Open manual checks](#open-manual-checks)).
**Milestone:** [M1 — Font, rendering, and input foundation](../M1-font-and-input.md)

## Environment

| Component | Version / detail |
| --- | --- |
| OS | Bluefin 44 (Fedora 44 base), kernel `7.2.11-200.fc44.x86_64` |
| Session | GNOME Wayland (`XDG_SESSION_TYPE=wayland`), XWayland `DISPLAY=:0` |
| Go | 1.27.4 (mise) |
| Ebitengine | `v2.10.6` |
| `golang.design/x/clipboard` | `v0.11.0` (pure Go on Linux; no CGO) |
| pico-lsp | `v1.6.0` (`pkg/p8scii`) |
| PICO-8 | 0.2.7 |

## Font data and verification

- Source: the CC0 PICO-8 font records in `pod/f_pico8.pod` inside the PICO-8
  0.2.7 data file. The pod contains 256 `CBMP` records: 128 narrow 3x5 glyphs
  followed by 128 wide 7x5 glyphs, one per P8SCII byte. Each record is a
  28-byte header (magic, tag, width, height, 8 bpp, two reserved words)
  followed by `width*height` 8-bit pixels with `0` = background and `1` = ink.
- The data was extracted once during development and checked in as
  `internal/font/glyphs.txt`. Nothing is extracted at build or run time.
- Control bytes `0x00`-`0x0f` are checked in blank and are never drawn as cells.
  Narrow glyphs occupy the left 4-pixel cell and wide glyphs the left 8-pixel
  cell with a one-pixel right gap, matching PICO-8's 4/8-pixel advances.
- Verification: a temporary PICO-8 0.2.7 cart printed bytes `0x10`-`0xff` in a
  16x16 grid of 8x8 tiles and saved a 1x screenshot with
  `extcmd("screen", 1, 1)` under `pico8 -x`. All 240 printable glyphs
  (3x5/7x5 ink regions) compared pixel-for-pixel against `glyphs.txt`:
  **0 mismatches**.
- Golden tests render the full charset at 1x, 2x, 3x, and 4x in
  `internal/app/testdata/`.

## Clipboard

The adapter is lazy: `clipboard.Init` runs on the first paste, not at startup.
A headless run therefore starts normally and returns a typed
`input.ErrUnavailable` only when the user tries to paste. Reads run on a
background goroutine and are delivered to the Update goroutine through a
buffered channel, so a hung X11 owner or Wayland data-control offer cannot
freeze drawing or key handling. Runtime `clipboard.ErrUnavailable` and
`ErrUnsupported` results are also mapped to `input.ErrUnavailable`.

Known limitation: the clipboard package's native Wayland read path has no
deadline, so a source that never closes its transfer pipe can leave the worker
goroutine blocked. That worker and its pipe leak until exit, and because the
package holds a global lock across the read, later paste attempts remain
pending. The editor stays responsive, but paste requires a restart in this
rare case. M3/M6 should replace or wrap the backend with cancellable reads.

Real round-trips were run with the editor's `SystemClipboard` (write a token,
read it back, restore the previous text):

| Backend | Command shape | Result |
| --- | --- | --- |
| X11 / XWayland | `env -u WAYLAND_DISPLAY DISPLAY=:0` | Pass, 28-byte round-trip |
| Wayland-only (`DISPLAY` unset) | `env -u DISPLAY WAYLAND_DISPLAY=wayland-0` | Unavailable |
| Headless | no `DISPLAY`, no `WAYLAND_DISPLAY` | Unavailable |

The Wayland-only failure is expected on this host: GNOME does not advertise
`ext_data_control_manager_v1` or `zwlr_data_control_manager_v1`, so the
clipboard package falls back to the X11 backend through XWayland. With the
normal session environment (`DISPLAY=:0` plus `WAYLAND_DISPLAY=wayland-0`) it
uses XWayland successfully.

V1 decision: keep the adapter and degrade to internal registers plus a
notification when `ErrUnavailable` is returned. A compositor or future
clipboard package that supports data-control can remove the XWayland
requirement without changing the editor seam. CRLF normalization is applied by
the paste call site; a lone CR is preserved because it is a P8SCII byte.

## IME

- Ebitengine's stable API exposes only committed text through
  `ebiten.AppendInputChars`; `InputChars` is deprecated. There is no stable
  preedit, candidate-window, or caret-placement API.
- Ebitengine 2.10.6 contains an experimental `exp/textinput` package with
  composition sessions and caret bounds. It is intentionally not used: the V1
  architecture forbids alpha/experimental APIs, and PICO-8 kana input is owned
  by the editor's P8SCII input layer, not the OS IME.
- This host runs `ibus-daemon`, but no IBus engines are configured
  (`org.gnome.desktop.input-sources sources` is empty), so a live ibus
  composition could not be exercised here. fcitx5 is not installed.
- V1 decision: accept committed text only. Do not promise preedit rendering or
  candidate-window placement. If committed text turns out to be unreliable in
  interactive testing, compose in an editor popup before M6.

## Key repeat

- Ebitengine reports physical key state, not OS repeat events. The editor owns a
  `input.Tracker` timer: first repeat after **400 ms**, then every **50 ms**
  while the key is held.
- Modifiers, `esc`, and text runes are not repeated by the timer. Text events
  are emitted only when the OS supplies committed runes.
- Focus loss clears pressed-key and repeat state and emits a `KindFocus` event.
- Table tests cover exact repeat boundaries, release, focus loss, modifiers,
  escape, and text behavior in `internal/input`.

## Non-US layouts and text input

- Key events use physical US-layout identities (`KeyQ`, `KeyY`, punctuation),
  while committed text comes from `AppendInputChars`. The editor never
  synthesizes text from key codes, so dead keys and non-US layouts use the OS
  text path.
- Shift+letter produces both a key event (`ModShift` plus the letter key) and
  the OS text rune(s). M1 records both; M6 decides when insert mode turns the
  key event into a P8SCII glyph. Normal-mode Vim will consume key events only.
- A live layout switch was not exercised in this session; see the open manual
  checks.

## Window scaling and HiDPI

- `Layout` always returns `256x256`. `DrawFinalScreen` computes
  `scale = max(1, min(width/256, height/256))` from physical final-screen
  pixels, centers the canvas, and uses `FilterNearest`.
- Scale math is table-tested at device scale factors 1.25, 1.5, and 2.0. For
  example, a 4x window on a 1.25-factor display occupies 1280 physical pixels,
  which the final-screen pass draws at integer 5x; no fractional scaling or
  linear filtering is used. Manual/debug scale selection includes 1x; automatic
  sizing clamps to 2x-8x.
- Pressing `1`-`4` in the charset screen sets the window to
  `256*scale` device-independent pixels. The final-screen pass still chooses an
  integer physical scale, so the grid stays sharp.
- Ebitengine's cursor position is read with `CursorPositionF`; the app inverts
  Ebitengine's default letterbox transform and applies its exact integer
  `CanvasRect`, using the last final-screen bounds. Table tests round-trip
  points through integer, letterboxed, wide, odd-sized, and fractional/HiDPI
  rectangles. M4 should still add a click-level integration test before
  tree/tab hit testing.
- The CI `xvfb` smoke job builds with `CGO_ENABLED=0`, opens the editor under
  Xvfb, and requires the `first-frame` log record emitted after the
  final-screen pass. It therefore fails if window/GL initialization hangs or
  the final-screen draw never runs.

## Open manual checks

These require an interactive session and are recorded here so the milestone
owner can close them without re-deriving the setup:

1. Run the editor under X11 and Wayland at 2x, 3x, and 4x; resize to 1x, 5x,
   and a non-integer window size. Confirm the grid stays sharp and centered.
2. With ibus or fcitx5 active, type committed text and confirm
   `AppendInputChars` receives it; note whether focus loss cancels composition.
3. Switch to a non-US layout and type Shift+letter and punctuation; confirm key
   events remain US-layout identities and text events follow the layout.
4. Copy from a terminal and press `Ctrl+V` on the charset screen; confirm the
   paste appears in the debug log on both X11 and Wayland.
