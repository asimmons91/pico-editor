# pico-editor V1 Roadmap

Status: M3 — Vim curated core implemented; M4 (cart model and file tree) next.
Interactive M1 spot checks remain tracked in `docs/spikes/M1-input.md`, and the
M3 manual checklist lives in `docs/spikes/M3-vim.md`.
Date: 2026-11-20
Repo: `github.com/asimmons91/pico-editor`
Go: `1.27.4`, managed by mise

## Design documents

- [ARCHITECTURE.md](ARCHITECTURE.md) — normative package boundaries,
  coordinate systems, render pipeline, LSP transport, and dependency pins.
- [VIM.md](VIM.md) — frozen V1 Vim command matrix. M3 must not add commands
  without updating the roadmap.

## Vision

A native PICO-8 code editor that feels like the built-in PICO-8 editor, but with
semantic highlighting, pico-lsp language intelligence, a simple file tree, Vim
editing, and authentic P8SCII input. One Go binary, no WebView.

## Locked decisions

| Area | Decision |
| --- | --- |
| Renderer | Go + Ebitengine 2.10+ (pinned `v2.10.6` during V1) |
| Editor UI | Custom-drawn PICO-8 UI using a 1-bit glyph atlas |
| Canvas | 256×256 logical, 64×42 cells, integer-scaled 1×–8×; automatic sizing clamps to 2×–8× |
| Vim | Curated core: normal/insert/visual char/line/block, core motions/operators/text objects, registers, search, undo, `:s`, `:w`/`:q`/`:e` |
| LSP | pico-lsp in-process through thin public `pkg/*` facades |
| Cart view | Show only `__lua__`, split into `-->8` code tabs, preserve all other cart sections |
| P8SCII input | Always authentic: Shift+letter inserts P8SCII glyphs in insert mode |
| Font source | Checked-in CC-0 1-bit atlas, verified against PICO-8 screenshots |
| Settings | TOML: global config plus workspace override, keymaps in the same file |
| License | MIT |
| Release | Linux `amd64`/`arm64` AppImage plus tar.gz |
| V1 platform | Linux first; keep the editor layer portable |
| File scope | `.p8` carts and included `.lua` files |

## V1 scope

### In scope

- Open a project folder and browse `.p8` and `.lua` files in a simple file tree.
- Open a `.p8` cart as a PICO-8-style code view with code tabs.
- Save carts without modifying `__gfx__`, `__sfx__`, `__music__`, `__map__`,
  `__gff__`, `__label__`, or the cart header.
- Pixel-perfect PICO-8 font rendering with the 16-color PICO-8 palette.
- Semantic highlighting from pico-lsp semantic tokens.
- Diagnostics, completion, hover, signature help, and go-to-definition.
- Cart token/character/compressed-size status from pico-lsp.
- Curated Vim mode: normal, insert, visual char/line/block, motions, operators,
  text objects, registers, search, undo/redo, `:s`, and basic ex commands.
- Authentic P8SCII input: Shift+letter glyphs, Ctrl+J/K kana modes, Ctrl+P puny
  mode, glyph picker, and `p8_left` completion.
- Settings file, keymap configuration, command palette, and a minimal error UI.
- Linux AppImage and tar.gz releases with install/run documentation.

### Out of scope for V1

- Sprite, map, sound, and music editors.
- Editing or exporting `.p8.png` carts.
- Full Vim: macros, marks, plugins, autocmds, splits/windows, advanced ex.
- References, rename, formatting, code actions, inlay hints, folding, and code
  lens (unless they fall out of the LSP work for free).
- Multi-cursor, remote workspaces, AI, terminal, and plugin system.
- macOS and Windows packaging. Keep code portable, but only ship and test Linux.
- Additional themes beyond the PICO-8 palette.

## Architecture

```text
cmd/pico-editor/          Ebitengine entry point and window lifecycle
internal/app/             Application state, layout, event loop, commands
internal/ui/              PICO-8 chrome, file tree, tabs, status, popups
internal/font/            Embedded 1-bit glyph data and metrics
internal/render/          Palette, software surface, text and cursor drawing
internal/input/           Keyboard, text, clipboard, and P8SCII input modes
internal/editor/          Buffer, cursor, selection, viewport, undo, search
internal/vim/             Modal editing state machine
internal/cart/            pkg/cart adapter, code tabs, save reconstruction
internal/workspace/       Folder scan, file tree model, include association
internal/lsp/             In-process pico-lsp client and feature UI
internal/config/          TOML settings and keymaps
internal/fsutil/          Atomic writes and directory fsync
```

Upstream pico-lsp changes:

```text
pkg/lsp/                  Public server API, for example Serve(ctx, stream)
pkg/cart/                 Public parse/splice/save API
pkg/p8scii/               Public charset, widths, encode/decode API
pkg/glyphs/               Public glyph catalog for the picker
```

The editor stores canonical Unicode text so `.p8` files and pico-lsp positions
stay compatible. pico-lsp owns the P8SCII mapping and width table; the editor's
`internal/input` owns key modes and input tables. A P8SCII tokenizer maps text
to P8SCII glyphs for display width, cursor motion, and input. LSP positions use
UTF-16 offsets.

## Milestones

Each milestone has a detailed spec with scope, concrete requirements, test
plans, and exit criteria. This roadmap remains the source of truth for V1
scope; the specs add implementation detail and record resolved decisions.

| Milestone | Detailed spec |
| --- | --- |
| M0 — Foundation and decisions | [M0-foundation.md](M0-foundation.md) |
| M1 — Font, rendering, and input foundation | [M1-font-and-input.md](M1-font-and-input.md) |
| M2 — Editor core | [M2-editor-core.md](M2-editor-core.md) |
| M3 — Vim curated core | [M3-vim-core.md](M3-vim-core.md) |
| M4 — Cart model and file tree | [M4-cart-and-file-tree.md](M4-cart-and-file-tree.md) |
| M5 — In-process pico-lsp | [M5-in-process-lsp.md](M5-in-process-lsp.md) |
| M6 — P8SCII input UX | [M6-p8scii-input.md](M6-p8scii-input.md) |
| M7 — Polish and hardening | [M7-polish.md](M7-polish.md) |
| M8 — Linux release | [M8-linux-release.md](M8-linux-release.md) |

### M0 — Foundation and decisions

Detailed spec: [M0-foundation.md](M0-foundation.md)

Goal: make the repo buildable and lock the decisions that affect the rest of
the roadmap.

Deliverables:

- `docs/ARCHITECTURE.md` with the package boundaries and LSP transport design.
- `docs/VIM.md` with the exact V1 Vim command matrix.
- `mise.toml` tasks for `build`, `run`, `test`, `lint`, `fmt`, and `check`.
- Ebitengine `v2.10.6` and an empty 256×256 logical window at 4×.
- CI using mise, `go test`, and `golangci-lint`.
- MIT license, CC-0 font notice, and PICO-8 trademark disclaimer.
- Written decisions for canvas, settings, release formats, and the pico-lsp
  public-package schedule (`pkg/p8scii` before M1, `pkg/cart` before M4,
  `pkg/lsp` before M5).

Exit criteria:

- `mise run check` passes locally and in CI.
- `mise run run` opens an empty editor window.
- V1 Vim command matrix and LSP post-V1 list are written down.
- The architecture and dependency decisions are recorded, and no pico-lsp
  `internal/` package is imported.

### M1 — Font, rendering, and input foundation

Detailed spec: [M1-font-and-input.md](M1-font-and-input.md)

Goal: prove the PICO-8 look and the input layer before building editor
features.

Deliverables:

- pico-lsp `pkg/p8scii` public mapping and width table.
- Checked-in CC-0 glyph data and a build-time atlas generator.
- P8SCII width table: narrow 4px glyphs and wide 8px glyphs.
- 256×256 software surface with the 16-color palette.
- Full-charset test screen with all 256 P8SCII glyphs.
- Integer window scaling, HiDPI handling, and cursor drawing.
- Keyboard abstraction for modifiers, Shift+letter glyph keys, and text input.
- Clipboard and IME spikes on Ebitengine 2.10, using
  `golang.design/x/clipboard` for clipboard access.

Exit criteria:

- Every printable P8SCII glyph renders pixel-perfect at 2x, 3x, and 4x.
- Wide glyphs occupy two narrow cells and cursor movement matches.
- Shift, Ctrl, Alt, and text input behave correctly on Linux.
- Window resizing does not blur or misalign the glyph grid.
- pico-lsp exposes and tags `pkg/p8scii`, and the editor pins it.

### M2 — Editor core

Detailed spec: [M2-editor-core.md](M2-editor-core.md)

Goal: a reliable text buffer and viewport independent of Vim and LSP.

Deliverables:

- Line-based text buffer; no rope needed for PICO-8-sized files.
- P8SCII-aware cursor, selection, and visual-column motion.
- Insert, delete, split/join lines, tabs, and indentation.
- Undo/redo, viewport scrolling, horizontal scrolling, and search.
- Open/save plain `.lua` files with modified state and error handling.
- Unit tests for editing edge cases and multi-rune P8SCII glyphs.

Exit criteria:

- A `.lua` file can be edited for a full session without corruption or panics.
- Cursor movement across wide glyphs and VS16 sequences is correct.
- Undo/redo and save round-trips are covered by tests.
- Typing and scrolling stay smooth on a cart-sized file.

### M3 — Vim curated core

Detailed spec: [M3-vim-core.md](M3-vim-core.md)

Goal: daily-driver modal editing for Lua and PICO-8 carts.

Deliverables:

- Modal state machine with normal, insert, visual char/line/block, and
  command-line modes.
- Core motions: `h/j/k/l`, `w/W/b/B/e/E`, `0/^/$`, `gg/G`, `f/F/t/T/;/,`, `%`,
  `{/}`, page motions, and counts.
- Core operators: `d/c/y/p`, `x/X`, `s/S`, `r`, `J`, `~`, `>`/`<`, `u`/`Ctrl+R`,
  and `.` repeat.
- Text objects: `iw/aw`, `iW/aW`, `i(/a(`, `i{/a{`, `i[/a[`, `i"/a"`, `i'/a'`,
  and paragraphs.
- Registers, visual modes including block, search with `n/N`, and undo/redo.
- Ex commands: `:w`, `:q`, `:q!`, `:wq`, `:x`, `:e`, `:bn`, `:bp`, `:b`, `:ls`,
  `:tabn`, `:tabp`, `:help`, `:noh`, and `:s`.
- Visual block `I`/`A`, block operations, and `:s` substitution.
- Mode indicator and a command matrix test suite.

Exit criteria:

- The V1 command matrix passes against fixture-based behavior tests.
- Normal-mode Shift+letter commands work and are not consumed by P8SCII input.
- Visual block editing and `:s` pass their fixtures.
- Editing a real cart tab feels natural for a Vim user.
- Macros, marks, plugins, splits, and `:set` remain explicitly out of scope.

### M4 — Cart model and file tree

Detailed spec: [M4-cart-and-file-tree.md](M4-cart-and-file-tree.md)

Goal: open real PICO-8 carts without exposing or damaging non-code sections.

Deliverables:

- pico-lsp `pkg/cart` public parser plus byte-preserving `ReplaceCode`
  save/splice API.
- Code-only view with `-->8` tab bar and inactive-tab folding.
- Save reconstruction that preserves all non-code sections byte-for-byte.
- Atomic writes and safe handling of read-only or externally changed files.
- Simple file tree for project folders; open `.p8` and `.lua` files.
- File tabs and a basic workspace model.
- Include-to-cart association through the `pkg/cart` resolver.

Exit criteria:

- Golden tests on real carts show only the `__lua__` body changing on save.
- Code tabs switch cleanly and preserve cursor/scroll state per tab.
- The file tree opens carts and includes without losing unsaved changes.
- Save failures surface a useful error and do not corrupt the cart.
- pico-lsp tags `pkg/cart`, and the editor pins the release.

### M5 — In-process pico-lsp

Detailed spec: [M5-in-process-lsp.md](M5-in-process-lsp.md)

Goal: use pico-lsp as the editor's language engine without a subprocess.

Deliverables:

- Public pico-lsp packages `pkg/lsp` and `pkg/glyphs`; `pkg/cart` and
  `pkg/p8scii` were delivered in M4 and M1.
- In-process JSON-RPC transport over `net.Pipe` or equivalent.
- Initialize, configuration, workspace folders, and didOpen/didChange/didSave.
- Diagnostics, semantic tokens, completion, hover, signature help, and
  go-to-definition.
- Cart info status request and P8SCII glyph picker request.
- Tests against pico-lsp testdata and an upstream pico-lsp release.

Exit criteria:

- Semantic highlighting covers keywords, strings, numbers, comments, operators,
  and symbol kinds on real carts.
- Diagnostics and completion update after edits without deadlocks or stale state.
- Hover/signature/definition work in `.p8` code and included `.lua` files.
- The editor pins a released pico-lsp version with the public API.

### M6 — P8SCII input UX

Detailed spec: [M6-p8scii-input.md](M6-p8scii-input.md)

Goal: make PICO-8's special-character input feel native.

Deliverables:

- Shift+letter glyph insertion in insert mode, always authentic to PICO-8.
- Captured PICO-8 0.2.7 Shift+letter, kana, and literal-mode tables.
- Ctrl+J Hiragana and Ctrl+K Katakana input modes.
- Ctrl+P literal/puny mode and the documented uppercase escape path.
- Glyph picker UI backed by pico-lsp.
- `p8_left`-style completion through pico-lsp.
- Clipboard and paste mapping for canonical P8SCII Unicode.
- Puny-font rendering variant and `docs/INPUT.md`.

Exit criteria:

- Every printable P8SCII glyph can be entered and round-trips through save/load.
- Normal-mode Vim Shift commands are unaffected.
- Uppercase identifier input has a documented, tested escape path.
- Ctrl+J/K/P modes match the captured PICO-8 behavior.
- Input mode help is accessible from the command palette or `:help`.

### M7 — Polish and hardening

Detailed spec: [M7-polish.md](M7-polish.md)

Goal: turn the feature set into a coherent V1.

Deliverables:

- Semantic token color mapping to the 16-color PICO-8 palette.
- Status bar: mode, position, diagnostics, tokens, characters, compressed size.
- Command palette, TOML settings file, keymap configuration, and
  notifications.
- Refresh-on-focus for external changes; full file watching is post-V1.
- Error dialogs, crash recovery, and logging to a file.
- Documentation, screenshots, and performance pass.

Exit criteria:

- All V1 features are discoverable and documented.
- No P0/P1 bugs remain in editing, saving, or LSP flows.
- Screenshot/golden tests cover the font, palette, tabs, and status bar.
- A 64KB cart opens and edits without visible latency.

### M8 — Linux release

Detailed spec: [M8-linux-release.md](M8-linux-release.md)

Goal: publish a usable V1.

Deliverables:

- `goreleaser` configuration for Linux `amd64`/`arm64` tar.gz archives.
- AppImage packaging for both architectures.
- App icon, version metadata, checksums, and install/run docs.
- Clean-machine smoke tests on Ubuntu and Fedora, X11 and Wayland.
- `v1.0.0` tag and release notes.
- MIT license and PICO-8 trademark disclaimer in every artifact.

Exit criteria:

- A new user can install and open a cart using only the release docs.
- AppImage and tar.gz both start without a Go toolchain or Node/Rust runtime.
- CI builds and tests the release commit.
- Checksums, license, and notices are published with the release.

## Sequencing

```text
M0 -> M1 -> M2 -> M3 -> M6 -> M7 -> M8
                \-> M4 -> M5 -/
```

Upstream pico-lsp gates:

```text
pkg/p8scii  before M1
pkg/cart    before M4
pkg/lsp     before M5
```

M3 (Vim) and M4/M5 can overlap if there are separate workstreams. M6 needs the
input pipeline from M1/M3 and the glyph picker/completion from M5. The
pico-lsp extractions are separate prerequisite tasks tracked upstream: M1 starts
after `pkg/p8scii` is tagged, M4's editor adapter after `pkg/cart`, and M5 after
`pkg/lsp`.

## Planning estimates

Estimates are for one focused developer and are ranges, not commitments.

| Milestone | Estimate |
| --- | --- |
| M0 — Foundation | 2–3 days |
| M1 — Font and input | 1–2 weeks |
| M2 — Editor core | 2–3 weeks |
| M3 — Vim curated core | 6–8 weeks |
| M4 — Cart and file tree | 2–3 weeks (+ upstream `pkg/cart`) |
| M5 — In-process LSP | 3–4 weeks (+ upstream `pkg/lsp`) |
| M6 — P8SCII input | 1–2 weeks |
| M7 — Polish | 2–3 weeks |
| M8 — Release | 1–2 weeks |

Sequential total: roughly 19–28 focused weeks for the editor, plus about one
week for the pico-lsp public-package extraction. With M3/M4/M5 overlap, plan
for roughly 5–7 months full-time. Part-time work will take longer. Vim scope is
the largest source of variance, so re-estimate after the M1 and early M3 spikes.

## Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Vim scope expands without bound | Lock `docs/VIM.md`, timebox M3, defer macros/marks/plugins explicitly. |
| P8SCII Shift+letter conflicts with uppercase input | Keep normal-mode Vim untouched; implement and document PICO-8's puny/literal path; test uppercase identifiers. |
| pico-lsp public API refactor destabilizes the server | Add public package tests upstream, keep the LSP boundary narrow, and pin a released version. |
| LSP UI work is larger than expected | Ship diagnostics + semantic tokens first, then completion/hover/definition; defer references/rename/format. |
| Cart save corrupts non-code sections | Golden round-trip tests, byte-preserving suffix writes, atomic saves, and backups on external modification. |
| Font widths or glyph mapping are wrong | Compare all 256 glyphs against PICO-8 screenshots; test wide glyphs and VS16 sequences. |
| Ebitengine input/IME/clipboard gaps | Spike in M1; Ebitengine 2.10 has no clipboard or IME APIs, so use `golang.design/x/clipboard` and committed text only. |
| Linux graphics/packaging issues | Test on a clean distribution early; document OpenGL/Wayland/X11 requirements. |
| Feature creep from “simple” file tree or LSP extras | Enforce the V1 out-of-scope list; move extras to post-V1 issues. |

## V1 release criteria

- Carts open as PICO-8 code tabs and save without touching other sections.
- Font and palette are pixel-accurate at supported integer scales.
- Semantic highlighting, diagnostics, completion, hover, signature help, and
  go-to-definition work on real carts and includes.
- Curated Vim core is a usable daily driver.
- P8SCII input, glyph picker, and `p8_left` completion work end to end.
- Linux AppImage and tar.gz install and run on a clean machine.
- Tests, CI, docs, and a license are in place.

## Resolved decisions

These were resolved while writing the milestone specs; the linked spec is
normative.

| Decision | Resolution | Spec |
| --- | --- | --- |
| Canvas | 256×256 logical, 64×42 cells, integer 1×–8×; auto 2×–8× | M0/M1 |
| Settings | TOML global config plus workspace override; keymaps in the same file | M0/M7 |
| License | MIT | M0 |
| Linux release formats | AppImage plus tar.gz, `amd64` and `arm64` | M8 |
| Vim profile | Curated core plus visual block and `:s` | M3 |
| Font source | Checked-in CC-0 atlas verified against PICO-8 screenshots | M1 |
| pico-lsp public API | Thin `pkg/*` facades; `pkg/p8scii` before M1, `pkg/cart` before M4, `pkg/lsp` before M5 | M0/M4/M5 |
| `.lua` include attachment | Workspace scan plus `pkg/cart` resolver | M4 |
| File watching | Refresh-on-focus in M7; full watcher post-V1 | M7 |
| `.lua` mixed line endings | Reject with `ErrMixedNewlines`; LF and CRLF files round-trip byte-for-byte | M2 |
| External changes | M2 checks mtime, size, and content hash on save; M4 adds the conflict dialog and backups | M2/M4 |
| M2 debug screen | Replaced by the M3 Vim screen; `--debug-charset` keeps the charset demo | M2/M3 |
| M3 startup | No argument opens an empty unnamed buffer in NORMAL mode | M3 |
| M3 buffer/tab scope | Minimal `.lua` multi-buffer session for `:e`/`:bn`/`:bp`/`:b`/`:ls`; `:tabn`/`:tabp` report one code tab until M4 | M3 |
| M3 save pipeline | Vim returns actions; the app snapshots and writes off the Update goroutine, then commits the save point | M3 |
| M3 clipboard registers | `"+`/`"*` reads are asynchronous; yanks queue writes; internal registers remain the fallback | M3 |
| M3 Backspace/Space | Normal-mode motions use Vim h/l semantics with line wrap | M3 |
| M3 help | Minimal scrollable overlay for `keys`, `input`, `lsp`, and `config`; M7 expands topics | M3 |
| Tab insertion | Spaces to the next tab stop, matching VIM.md | M2 |
| Run-cart command | Post-V1 | — |
| References, rename, formatting, code actions | Post-V1 | — |
| `.p8.png` read-only support | Post-V1 | — |

## Remaining research tasks

These are scheduled in the specs and do not block M0:

- Capture PICO-8 0.2.7 Shift+letter, kana, and Ctrl+P behavior (M6).
- Capture puny-font metrics against PICO-8 (M6).
- Interactive X11/Wayland clipboard, IME, and non-US layout spot checks
  (M1; automated results and the remaining checklist are in
  `docs/spikes/M1-input.md`).
- Confirm the `arm64` AppImage build and smoke test (M8).
