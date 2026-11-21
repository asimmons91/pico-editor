# pico-editor architecture

**Status:** Normative for V1. M0 freezes this document; later milestones may
extend it but must not contradict it without updating
[ROADMAP.md](ROADMAP.md).

pico-editor is one native Go binary: an Ebitengine window, a custom-drawn
PICO-8 interface, a line-based editor with a curated Vim core, and pico-lsp
running in-process. There is no WebView, subprocess language server, runtime
font rasterizer, or plugin host in V1.

Scope and locked product decisions live in [ROADMAP.md](ROADMAP.md). The Vim
command set is frozen in [VIM.md](VIM.md). Milestone specs under `docs/` add
implementation detail.

## 1. Package boundaries

Planned layout (packages appear as their milestone lands):

```text
cmd/pico-editor/     process entry point: flags, logging, version, window bootstrap
internal/app/        application state, layout composition, Update loop, commands, UI chrome
                     (tree, tabs, status, palette, notifications, dialogs)
internal/font/       embedded 1-bit glyph atlas and metrics
internal/render/     PICO-8 palette, software Surface, text and cursor drawing
internal/input/      key, text, focus, mouse, repeat, and clipboard adapters
internal/editor/     line buffer, cursor, selection, viewport, undo, search
internal/vim/        modal state machine, registers, ex commands, dot repeat
internal/cart/       pkg/cart adapter, code tabs, byte-preserving save reconstruction
internal/workspace/  folder scan, file tree model, include association
internal/lsp/        in-process pico-lsp client and language feature state
internal/config/     TOML settings, keymaps, validation, precedence
internal/fsutil/     atomic writes and directory fsync helpers
internal/version/    ldflags-injected build metadata
```

Dependency direction:

```text
cmd/pico-editor
  -> internal/app
       -> internal/{editor,vim,input,cart,workspace,lsp,config,render,font,fsutil}
internal/vim      -> internal/{input,editor}
internal/input    -> pico-lsp pkg/p8scii
internal/editor   -> pico-lsp pkg/p8scii, internal/fsutil
internal/render   -> internal/font, pico-lsp pkg/p8scii
internal/cart     -> internal/{editor,fsutil}, pico-lsp pkg/cart
internal/workspace-> pico-lsp pkg/cart
internal/lsp      -> internal/editor, pico-lsp pkg/lsp
internal/config   -> github.com/BurntSushi/toml, internal/input, standard library
internal/font     -> pico-lsp pkg/p8scii
internal/fsutil   -> standard library only
internal/version  -> standard library only
```

Rules:

- Imports form a DAG. Core packages (`editor`, `vim`, `cart`, `lsp`) never
  import `app`; only `app` assembles them. V1 keeps all chrome in `internal/app`
  rather than adding an `internal/ui` package.
- `app` is the only place that wires dependencies. A small `Config` struct is
  passed from `main`, so tests can replace the logger, clock, clipboard, and
  LSP client.
- No package-level mutable state. `Game` and its collaborators own state.
- pico-lsp is consumed only through its public `pkg/*` packages
  (`pkg/p8scii`, `pkg/cart`, `pkg/lsp`, `pkg/glyphs`). Importing
  `github.com/asimmons91/pico-lsp/internal/...` is forbidden. Go enforces the
  boundary; review enforces the intent.
- Do not copy Rune code. Rune is GPL-3.0 reference material, not a dependency.
- Cart parsing, P8SCII mapping, and language analysis belong to pico-lsp. The
  editor must not grow a second parser or glyph table.

## 2. Single-writer rule

All application and document state is mutated on the Ebitengine `Update`
goroutine, one event at a time:

- Keyboard, text, focus, mouse, clipboard results, file IO results, LSP
  responses, timers, and notifications arrive as messages.
- `Update` drains the message queue and applies state changes. Background
  goroutines never touch editor, Vim, cart, UI, or config state directly.
- `Draw` reads a stable snapshot and never mutates application state.
- Long work (opening a cart, saving, LSP requests) is dispatched without
  blocking `Update`; the UI shows pending state and applies the result message
  later.
- Tests drive `Update` synchronously and assert state; no sleeps are needed for
  core behavior.

This removes data races from the UI layer by construction. Only infrastructure
owned entirely by a background goroutine (for example the LSP client's pending
request table) uses its own synchronization.

## 3. Coordinate systems

Five coordinate systems exist and must not be mixed implicitly:

| System | Unit | Owner | Used for |
| --- | --- | --- | --- |
| File byte offset | byte | `fsutil`, `cart` source maps | Reading and splicing original bytes; diagnostics source maps |
| Line + rune offset | decoded rune index in one line | `editor` | Cursor, selection, undo, insert/delete |
| P8SCII glyph cluster | one printable P8SCII glyph (1+ runes) | editor tokenizer, backed by `pico-lsp/pkg/p8scii` | Cursor snapping, cluster deletion, glyph width |
| Display cell | 4 px or 8 px column on the canvas | `render`, `ui` | Layout grid, block selection, viewport, cursor drawing |
| LSP position | line + UTF-16 code unit | `lsp` | Protocol messages only |

Invariants:

- Buffers store canonical Unicode text. Never use `len(string)` as a column
  count; use decoded rune offsets or UTF-16 conversion explicitly.
- P8SCII printable glyphs 16–127 are narrow (4 px, one cell); 128–255 are wide
  (8 px, two cells), with the mappings supplied by `pkg/p8scii`. Do not use
  Unicode width, `runewidth`, or grapheme libraries.
- A cursor or selection always sits on a cluster boundary. Motions, deletion,
  and search expand to cluster boundaries and never split a VS16 sequence.
- Display cells are the only units used for the 64×42 grid, tab stops, block
  selection, and horizontal scrolling. A wide glyph is wholly inside or wholly
  outside a block rectangle.
- UTF-16 exists only at the LSP boundary. Convert line rune offsets to UTF-16
  code units when sending, and back when receiving; never persist UTF-16
  offsets.
- Byte offsets exist only at the file/cart boundary. The UI and editor never
  store them as cursor positions.

## 4. Document model

- `editor.Buffer` is line-based: a slice of canonical Unicode strings with no
  trailing newline on any line, plus a modified flag and undo history. PICO-8
  carts are small; no rope is needed.
- A standalone `.lua` buffer holds the entire file.
- A `.p8` buffer holds only the `__lua__` code text, including `-->8` tab
  markers. The original file bytes live beside it in the cart model. Saving
  asks `pkg/cart` to replace the code section and preserves the header and
  every other section byte-for-byte.
- The editor never parses or rewrites `__gfx__`, `__sfx__`, `__music__`,
  `__map__`, `__gff__`, `__label__`, or the header for its own purposes.
- Newlines in buffers are `\n`. The cart adapter owns how code text is spliced
  into the original file; other sections are copied verbatim.
- The modified flag is derived from the undo save point, not from a separate
  heuristic. Undo after a save back to the save point clears it.
- Saves are atomic: write a temporary file in the same directory, fsync it,
  rename it, then fsync the directory. M2 checks mtime, size, content hash, and
  mode before saving; M4 adds the conflict dialog, backups, and explicit
  outside-root confirmation.

## 5. Cart tab model

- One code buffer per open cart. A code tab is a line range in that buffer.
- The first tab starts after the `__lua__` header line. A line that starts with
  `-->8` at column 0 begins the next tab; the marker line belongs to the tab it
  introduces and is preserved on save.
- Saving writes the tab ranges back in order, preserving the markers and all
  non-code sections. Editing a tab must never reorder or drop markers.
- Per-tab view state (cursor, selection anchor, viewport, horizontal scroll) is
  stored with the tab index and reconciled when markers are inserted or
  removed. Cursor/scroll state for hidden tabs survives tab switches.
- File tabs and cart code tabs are different concepts: file tabs select buffers;
  code tabs select line ranges within one cart buffer. `:bn`/`:bp` move file
  buffers, `:tabn`/`:tabp` move cart code tabs.

## 6. Render pipeline

```text
app state -> render.Surface (256x256, palette indices)
          -> one 256x256 RGBA ebiten.Image (WritePixels when dirty)
          -> Game.Draw -> 256x256 logical offscreen
          -> Game.DrawFinalScreen -> integer-scaled, centered, letterboxed
```

- `render.Surface` is a software 256×256 buffer of PICO-8 palette indices. All
  text, chrome, cursor, selections, and popups are composed into it. Drawing
  outside the surface is a no-op.
- The 16-color PICO-8 palette is the only color source. Text uses the checked-in
  1-bit glyph atlas; there is no runtime TTF rasterization, anti-aliasing, or
  linear filtering.
- The surface is uploaded to a single `ebiten.Image` with `WritePixels` only
  when dirty. The image size never changes in V1.
- `Game.Layout` always returns `256, 256`. `Game.Draw` renders the logical
  canvas. `Game.DrawFinalScreen` reads the final screen's bounds (physical
  pixels), computes `scale = max(1, min(width/256, height/256))`, centers the
  canvas, draws with `FilterNearest`, and fills the letterbox with palette index
  0. This is deliberate: Ebitengine's default final-screen filter is a box
  filter, which would soften pixel art.
- Window sizing is in device-independent pixels: default scale 4
  (`1024×1024`), scale 0 chooses the largest integer scale fitting 80% of the
  current monitor clamped to 2–8. Manual and debug scale changes may select
  1×; automatic sizing never goes below 2×. The minimum window is 256×256 so
  the canvas is never cropped. HiDPI is handled by Ebitengine; the final-screen
  pass works in physical pixels and coordinates are never multiplied by the
  device scale factor manually.
- `DrawFinalScreen` replaces Ebitengine's default final-screen transform.
  Ebitengine APIs that use that default transform (for example
  `ebiten.CursorPositionF` and IME candidate placement) must be mapped through
  the same canvas rectangle before use. M1 owns the mouse/IME spike; do not
  assume the default letterbox mapping matches the drawn canvas.
- Golden tests assert palette-index surfaces headlessly; they do not compare
  GPU screenshots. M1 adds an optional Ebitengine headless smoke test.

## 7. Input flow

```text
Ebitengine keys + AppendInputChars
  -> internal/input (modifiers, repeat, focus, text, mouse, clipboard)
  -> normal/visual: internal/vim consumes key events only
  -> insert: P8SCII input layer consumes Shift+letter and Ctrl+J/K/P,
             then insert text goes to internal/vim and internal/editor
  -> editor operations
```

- Ebitengine provides physical key state and, separately, committed text.
  Both are emitted; the consumer decides which to use. OS text input is never
  synthesized from key codes, so layouts and dead keys work.
- Normal-mode and visual-mode keys always go to Vim. Shift+letter commands
  (`J`, `K`, `P`, `X`, `D`, `C`, `Y`, …) must never be consumed by P8SCII
  insertion.
- Insert mode is the only place where the P8SCII layer may turn Shift+letter
  into a glyph. There is no P8 input toggle. PICO-8's `Ctrl+P` puny mode is the
  documented uppercase escape path; it changes input encoding only.
- Key repeat is implemented in `internal/input` (400 ms initial, 50 ms
  interval); focus loss clears pressed-key and repeat state.
- Clipboard access is behind an adapter, initialized lazily. Ebitengine has no
  clipboard API; M1 wires `golang.design/x/clipboard`. Failure degrades to
  internal registers and a notification, never a crash.
- `internal/input` knows nothing about Vim, the cart model, or widgets.

## 8. LSP transport

- pico-lsp runs inside the editor process. `pkg/lsp` exposes a server entry
  point (planned: `Serve(ctx, stream)`) and the editor dials it over a
  `net.Pipe` pair (or an equivalent in-memory duplex stream).
- JSON-RPC framing uses `go.lsp.dev/jsonrpc2` streams with
  `go.lsp.dev/protocol` types. The server runs on its own goroutine. The client
  has one writer (serialized), a pending-request map keyed by ID, and
  context-based cancellation.
- pico-lsp never writes to stdout. Any logging goes to stderr or the configured
  log file.
- Initialize sends client capabilities, workspace folders, and configuration.
  Document sync is full-text: carts and `.lua` files are small enough that
  incremental sync is unnecessary, and full sync keeps splices simple.
- The UI never blocks on a request. `internal/lsp` posts messages into the app
  mailbox; `Update` drains them, drops stale responses by document version, and
  keeps diagnostics and semantic tokens per buffer/version.
- Positions on the wire are UTF-16. Conversion goes through the coordinate
  rules above; a received position outside a grapheme/P8SCII cluster is
  expanded to a cluster boundary before use.
- Shutdown cancels the context, closes the stream, and waits with a bounded
  timeout. M5 defines feature-level retries and timeout behavior.

## 9. Configuration, keymaps, and commands

- Settings are TOML, in two layers:
  1. `$XDG_CONFIG_HOME/pico-editor/config.toml`
     (`~/.config/pico-editor/config.toml` when unset).
  2. `<workspace>/.pico-editor.toml`.
- Precedence is workspace > global > defaults. Tables merge field by field;
  arrays replace. Invalid files warn and keep the last valid configuration;
  defaults are always available. M7 implements parsing, validation, custom
  glyph definitions, and reload.
- Keymaps live in the same files. Key notation is defined in
  [VIM.md](VIM.md) and used identically by the config parser, the command
  matrix, and documentation.
- `internal/app` owns the command registry and stable command IDs and draws the
  palette and help; it does not delegate command semantics to a separate UI
  package in V1.
  `internal/vim` exposes its commands to the registry rather than hard-coding
  key bindings above it.
- Ex commands, palette commands, and keymap bindings all dispatch through the
  same command layer so that behavior and documentation cannot drift.

## 10. Errors, logging, recovery, and shutdown

- Logging uses `log/slog` with a text handler. Default path:
  `$XDG_STATE_HOME/pico-editor/pico-editor.log`, falling back to
  `~/.local/state/pico-editor/pico-editor.log`. Directories are `0700`, the log
  is `0600`. If the log cannot be opened, warn on stderr and log to stderr.
- Never log file contents, cart bytes, clipboard text, or LSP document text.
  Log paths, operation names, timings, and error values.
- Invalid flags print usage to stderr and exit 2. Fatal startup errors go to
  stderr and the log and exit 1. A panic is logged with its stack and exits 1;
  later milestones may recover inside command handlers where continuing is
  safe, never in a way that leaves a half-applied buffer change.
- User-visible failures (bad `:s` pattern, save conflict, unavailable
  clipboard, LSP error) become notifications or dialogs. They never panic and
  never silently discard a buffer.
- Save failures leave the buffer dirty and the file untouched (atomic write).
  Cart reconstruction errors abort the save before the rename.
- Crash recovery snapshots live under
  `$XDG_STATE_HOME/pico-editor/recovery/` (M7). They are never treated as more
  authoritative than the file on disk.
- Shutdown confirms unsaved buffers (M3/M7), cancels the LSP context, stops
  input, and closes the log. `Ctrl+Q` and window close both request a normal
  shutdown; in M0 there is no unsaved state.

## 11. Testing and CI gates

- Unit tests are table-driven for scale math, coordinate conversion, P8SCII
  widths, register behavior, regex translation, and parsing.
- Golden tests cover cart save round-trips (only `__lua__` changes), palette
  surfaces, font glyphs, and Vim matrix fixtures. Golden data lives in
  `testdata/` and is reviewed like code.
- Fuzz tests cover the Vim parser, `:s` translation, and malformed carts.
  Invariants include valid UTF-8, cluster-boundary cursors, and no panics.
- `go test ./...` runs on every change. Milestones that touch UI add headless
  Ebitengine tests where practical; M1 adds an `xvfb` smoke job once a window
  is actually opened.
- `mise run check` is the single gate locally, in CI, and before a milestone
  exit. It runs `fmt-check`, `lint`, `test`, and `build`.
- CI runs on `ubuntu-latest` with Go and `golangci-lint` installed by
  `jdx/mise-action` from `mise.toml` (never duplicated in the workflow).
  `CGO_ENABLED=0` and `GOFLAGS=-mod=readonly` are set. No X11/GL headers are
  assumed in M0/M1 CI.
- CI must be green before merge. `main` is protected; changes land through
  branches and PRs, and each PR gets the `code-review` pass described in
  `AGENTS.md`.

## 12. Dependency pins and pico-lsp schedule

Pins live only in `go.mod` and `mise.toml`. No pseudo-versions or branches may
be used at a milestone exit.

| Dependency | Pin | Needed |
| --- | --- | --- |
| Go | `1.27.4` | M0 |
| Ebitengine | `v2.10.6` | M0 |
| golangci-lint | `2.15.0` (mise) | M0 |
| pico-lsp module (M17–M19 plus catalog expansion) | `v1.7.0` (`pkg/p8scii`, `pkg/cart`, and `pkg/lsp` since `v1.6.0`; complete `pkg/glyphs` catalog in `v1.7.0`) | M1/M4/M5/M6 |
| `go.lsp.dev/protocol` | `v1.0.1` | M5 |
| `go.lsp.dev/jsonrpc2` | `v1.0.1` | M5 |
| `go.lsp.dev/uri` | `v1.0.1` (transitive) | M5 |
| TOML parser | `github.com/BurntSushi/toml v1.6.0` | M7 |
| `golang.design/x/clipboard` | `v0.11.0` (pure Go on Linux) | M1 |

`pkg/cart` in pico-lsp `v1.7.0` brings `go.lsp.dev/jsonrpc2`, `protocol`, and
`uri` in transitively. They appear as indirect `go.mod` entries before M5; the
editor does not import those packages directly until the LSP client lands.

Development rules:

- During development against a sibling checkout, use
  `replace github.com/asimmons91/pico-lsp => ../pico-lsp`. The replace is
  removed before each milestone exits.
- Never commit `go.work`. CI never needs a sibling checkout; it builds against
  the pinned release.
- A milestone cannot exit on a pseudo-version: only a tagged pico-lsp release
  with the required public packages counts.
- The pico-lsp public-package schedule (`pkg/p8scii`, `pkg/cart`, and
  `pkg/lsp`/`pkg/glyphs` all in `v1.6.0`; pico-lsp milestones M17–M19) is
  a cross-repository commitment. M6 uses the additive `v1.7.0` catalog
  expansion that covers every printable glyph. pico-lsp's roadmap records the
  same schedule; the editor must not import an unreleased API.

## 13. M0 implementation notes

- `cmd/pico-editor` parses flags, configures `log/slog`, resolves the window
  scale, sets resize limits, and calls `ebiten.RunGame`.
- `internal/app.Game` owns the Ebitengine loop. `Layout` returns `256, 256`;
  `DrawFinalScreen` performs the integer scale, centering, and letterbox pass
  described above. M0 paints palette index 0 only.
- Ebitengine runs game callbacks on its game goroutine, so the game methods
  recover frame panics themselves. `Update` returns the logged fatal error to
  stop `RunGame`; `Draw` and `DrawFinalScreen` record it for the next `Update`.
  `main` keeps a top-level recover for startup panics. Both paths exit 1.
- `internal/app/window.go` contains the pure scale, rectangle, and cursor
  mapping math; tests cover non-integer device-scale windows, odd dimensions,
  and letterboxing.
- `internal/version` supplies `Version`, `Commit`, and `BuildDate` for
  `-ldflags` injection. Development builds report `dev`/`unknown`.
- Linux backends: Ebitengine selects Wayland or X11 automatically. If a backend
  fails to open in M0 manual testing, record the environment, backend, and
  workaround here before the milestone exits.
- M1 adds the checked-in CC0 glyph data under `internal/font`, the deterministic
  `tools/fontgen` generator, the indexed `internal/render.Surface`, and the
  platform-neutral `internal/input.Tracker`. The font data was extracted once
  from PICO-8 0.2.7 and verified pixel-for-pixel against a PICO-8 screenshot;
  see `docs/spikes/M1-input.md`.
- M2 added `internal/editor` (line buffer, P8SCII cluster positions, cursor,
  selection, edits, undo, viewport, search) and `internal/fsutil`
  (`AtomicWrite` with owner/mode preservation and directory fsync). Its
  temporary debug screen was replaced by M3: no argument now opens an empty
  normal-mode buffer, `--debug-charset` keeps the charset screen, and `.p8`
  files were rejected until M4's cart adapter.
- M2 stores the save point as a SHA-256 of the canonical text, so undo/redo
  across the save point updates `Dirty()` without writing. It rejects files
  mixing CRLF and LF, rejects ambiguous CR-at-EOL saves, checks mtime, size,
  mode, and content hash before saving, refuses symlink targets outside the
  file's parent directory, and rejects cart paths even through symlinks; M4 adds
  conflict UI, backups, and explicit outside-root confirmation. A
  directory-fsync failure after the rename is reported as a
  `fsutil.CommitError` after the save point is updated, so retries are not
  blocked by a false external-change error.
- M2's temporary debug screen saved synchronously. M3 replaces it with the
  Vim-backed editor screen and routes saves through the message/result path
  described in §2: `Vim.Handle` returns actions, `app.Session.PrepareSave`
  snapshots the buffer on the Update goroutine, the snapshot is written on a
  background goroutine, and `CommitSave` records the save point on the Update
  goroutine. A save in flight does not block input; `:wq` waits for the result
  before closing.
- M3 adds `internal/vim`: modes, count and operator state, motions, text
  objects, registers, search with Vim-magic translation, ex commands including
  `:s`, visual block operations, dot repeat, and the jump list. The editor
  package stays free of modal or Vim-specific state. `internal/editor` gained
  blockwise selections, word-end motions, `Search.FindAll`, and the
  `PrepareSave`/`CommitSave` snapshot API.
- The M3 screen owns a small `.lua` multi-buffer session: `:e` opens or reuses a
  file, `:bn`/`:bp`/`:b`/`:ls` move through buffers, and `:tabn`/`:tabp` report
  that one code tab exists until M4's cart adapter lands. `:q` closes the
  current buffer or exits when it is the last one; window close and `Ctrl+Q`
  refuse to discard dirty buffers.
- `"+`/`"*` register reads always go through the asynchronous clipboard
  adapter; a yank queues a write action. Clipboard failures keep the internal
  register and surface a notification. The M1 charset screen remains available
  behind `--debug-charset`; no argument now opens an empty normal-mode buffer.
- The M3 screen draws buffer text with search-match and visual-selection
  backgrounds, a block/bar cursor, and a one-row status. M7 replaces the interim
  row with the compact segmented status bar, notifications, and expanded help
  topics; the command-line prompt still replaces the status row.
- Clipboard access uses `golang.design/x/clipboard` `v0.11.0` behind
  `input.Clipboard`, initialized lazily. Reads run on a background goroutine and
  are applied through a channel on the Update goroutine. On GNOME Wayland the
  package falls back to XWayland; without a data-control manager or X11 display
  it returns a typed unavailable error and the UI degrades to internal
  registers.
- M4 adds `internal/cart`, an adapter over pico-lsp `pkg/cart` `v1.6.0`. It
  keeps `Raw` immutable, exposes only the `__lua__` body as `editor.Buffer`, and
  splices saves back through `ReplaceCode`. Tabs are local line ranges derived
  from `pkg/cart.Tabs()` and are re-derived through `pkg/cart` after edits.
  `internal/cart` also owns conflict metadata, read-only checks, the workspace
  root boundary, and save snapshots; `internal/fsutil` adds numbered `.bak`
  copies for conflict overwrites.
- `internal/editor` gained an active line window for cart code tabs. Cursor
  motion, operators, join/delete-at-line-edge commands, paste, search, `G`/`gg`,
  and ex ranges are clamped to the active tab; the hidden `-->8` separator is
  never editable. Empty tabs use a virtual cursor and materialize one blank line
  on the first edit, so opening and saving an untouched cart stays byte-exact.
  M4 rejects creating a new `-->8` separator by typing; code that would read
  back as a cart section header is rejected at save time.
- `internal/workspace` scans the project root with the M4 ignore list and entry
  cap, follows file symlinks but not directory symlinks, and builds cart/include
  associations through the public `pkg/cart` resolver. Oversized carts are
  skipped before reading. `ResolveInclude` returns the nearest owning cart;
  `.lua` buffers show the owner in the status row.
- M4 layout is row 0 file tabs, row 1 cart code tabs for `.p8`, rows 2–40 for
  cart code (rows 1–40 for `.lua`), and row 41 for status. The file tree is a
  20-column sidebar; when visible it reduces the editor area, and a four-cell
  gutter holds diagnostic signs and relative line numbers. Ctrl+Tab and
  Ctrl+Shift+Tab switch code tabs; Ctrl+Shift+B toggles the tree; Ctrl+S saves.
- Save conflicts, deleted files, outside-root symlinks, read-only failures, and
  dirty-session quits are modal dialogs. Conflict overwrites create a numbered
  backup before the atomic replace. External changes are checked on save and on
  focus; clean files reload automatically, dirty files prompt. `Ctrl+Q` and
  window close now enter the Save all / Discard all / Cancel flow.
- The CLI accepts a file or directory. A file opens with its parent as the
  workspace and the tree hidden; a directory or no argument opens an empty
  buffer with the tree visible and focused.
- Ebitengine 2.10.6's experimental `exp/textinput` package is not used. V1
  accepts committed text only and does not promise preedit or candidate-window
  rendering.
- `internal/app` owns `canvasPosition`/`mapCursor`: it inverts Ebitengine's
  default letterbox transform with the last final-screen bounds and applies
  the same integer `CanvasRect` used for drawing. `internal/input` receives the
  mapper as a function so it does not duplicate window geometry.
- M5 adds `internal/lsp`, an in-process client over `net.Pipe` using
  pico-lsp `pkg/lsp` `v1.6.0`. `Service.Start` creates one server, sends
  `initialize` with UTF-16-only capabilities, workspace folders, and
  `initializationOptions.pico8`, then replays open documents. A single
  background writer serializes notifications; feature requests are asynchronous
  and results are applied by `Service.Poll` on the Update goroutine. Document
  changes are full-text and debounced by 120 ms; cart info is requested after
  saves, after a 300 ms edit debounce, and when the active buffer changes.
- `internal/lsp/positions.go` centralizes full-cart, code-buffer, and UTF-16
  conversion. Tokens and diagnostics store cluster-expanded rune ranges;
  `internal/app/screen.go` draws only the active cart tab. M7 implements the
  final semantic color table in `internal/lsp/colors.go`.
- LSP-backed UI lives in `internal/app/lsp_session.go` and
  `internal/app/lsp_ui.go`: diagnostics underlines and `:diag`, completion
  (`Ctrl+Space`, `Ctrl+N`/`Ctrl+Shift+N`, `Enter`/`Tab`, popup-first `Esc`),
  `K` hover, signature help, `gd`/`Ctrl+]` definition, cross-file jump actions,
  and `:lsp restart`. `Ctrl+P` remains reserved for M6.
- LSP failure is non-fatal: the status row keeps `LSP!`, editing/saving/Vim
  continue, routine request errors go to message history, and `:lsp restart`
  creates a fresh server on the same service.
- M5 tests use a copied subset of pico-lsp `v1.6.0` testdata under
  `internal/lsp/testdata`. Integration tests exercise the real server over
  `net.Pipe` for tokens, diagnostics, completion (including `p8_left`), hover,
  signature help, definition, cart info, and restart.
