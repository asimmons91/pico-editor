# M6 — P8SCII input UX

**Status:** Draft
**Depends on:** M1 (input), M3 (insert mode), M5 (`pico8/glyphs`, completion)
**Blocks:** M7
**Roadmap:** [ROADMAP.md](ROADMAP.md)
**Estimate:** 1–2 weeks

## Outcome

PICO-8's special-character input feels native: Shift+letter inserts glyphs in
insert mode, kana and literal modes work, the glyph picker is searchable,
`p8_left` completion inserts canonical Unicode, and paste/copy round-trip
through `.p8` files. Normal-mode Vim Shift commands remain untouched.

**Demo:** Enter every printable P8SCII glyph through Shift+letter, kana modes,
or the picker; save, reload, and verify identical canonical text. Type an
uppercase identifier using the literal path. Run `:help input` and see the mode
table.

## Scope

### In scope

- Shift+letter glyph insertion in insert mode, always authentic.
- Ctrl+J Hiragana and Ctrl+K Katakana modes.
- Ctrl+P literal/uppercase mode and the documented escape path.
- Glyph picker backed by `pico8/glyphs`.
- `p8_left` completion through pico-lsp.
- Clipboard copy/paste mapping for canonical P8SCII Unicode.
- Puny-font rendering variant.
- Input help and `docs/INPUT.md`.
- Round-trip and coverage tests for all printable glyphs.

### Out of scope

- A P8 input toggle or any mode that makes Shift+letter stop inserting glyphs
  by default. Authentic input is the default.
- IME preedit rendering and candidate-window positioning (M1 documents the
  limitation).
- Custom glyph pixel editing; custom glyph names are accepted from settings and
  forwarded to pico-lsp.
- Macros, mappings, and user-defined input tables.

## Deliverables

1. `internal/input/p8scii.go` with key-mode state and tables.
2. Shift+letter mapping for bytes 128–153, captured and verified against
   PICO-8 0.2.7.
3. Kana mode tables for bytes 154–229, captured and verified.
4. Ctrl+P literal mode with a visible status indicator and help.
5. Glyph picker UI over `pico8/glyphs`.
6. `p8_left` completion insertion path.
7. Clipboard mapping and paste normalization.
8. Puny-font atlas and rendering option.
9. `docs/INPUT.md` and `:help input`.
10. Coverage tests for every printable byte and normal-mode regression tests.

## Detailed requirements

### 1. Shift+letter glyphs

P8SCII bytes `128–153` are 26 glyphs in charset order. The expected mapping is:

| Key | Byte | Glyph | Key | Byte | Glyph |
| --- | --- | --- | --- | --- | --- |
| Shift+A | 128 | █ | Shift+N | 141 | ♪ |
| Shift+B | 129 | ▒ | Shift+O | 142 | 🅾️ |
| Shift+C | 130 | 🐱 | Shift+P | 143 | ◆ |
| Shift+D | 131 | ⬇️ | Shift+Q | 144 | … |
| Shift+E | 132 | ░ | Shift+R | 145 | ➡️ |
| Shift+F | 133 | ✽ | Shift+S | 146 | ★ |
| Shift+G | 134 | ● | Shift+T | 147 | ⧗ |
| Shift+H | 135 | ♥ | Shift+U | 148 | ⬆️ |
| Shift+I | 136 | ☉ | Shift+V | 149 | ˇ |
| Shift+J | 137 | 웃 | Shift+W | 150 | ∧ |
| Shift+K | 138 | ⌂ | Shift+X | 151 | ❎ |
| Shift+L | 139 | ⬅️ | Shift+Y | 152 | ▤ |
| Shift+M | 140 | 😐 | Shift+Z | 153 | ▥ |

Rules:

- The table is verified against PICO-8 0.2.7 screenshots during M6. If a
  screenshot disagrees, the screenshot wins and the table is corrected before
  exit.
- Insertion happens only in insert mode, only when Shift is the sole modifier,
  and only for physical `A`–`Z` keys. Ctrl/Alt/Meta combinations are not
  converted.
- The inserted text is canonical Unicode from `pkg/p8scii`, not a raw byte.
- Normal mode never sees these conversions: `J`, `K`, `P`, `X`, `D`, `C`, `Y`,
  and every other Shift command go to Vim.
- Visual and command-line modes do not convert Shift+letter.
- If the active keyboard layout does not produce a US letter for the physical
  key, fall back to the text rune unchanged and log once at debug level. Do not
  guess a glyph.
- Shift+letter is checked before `AppendInputChars` text is applied so the
  editor does not insert both a glyph and an uppercase letter.

### 2. Kana modes

- `Ctrl+J` toggles Hiragana input mode.
- `Ctrl+K` toggles Katakana input mode.
- `Ctrl+J` and `Ctrl+K` are mutually exclusive; entering one leaves the other.
- `Esc` leaves insert mode and clears the kana mode.
- The mode is insert-only and is shown in the status bar as `HIRA` or `KATA`.
- The kana table covers P8SCII bytes `154–229` exactly. The mapping from
  keystrokes to kana must be captured from PICO-8 0.2.7; do not invent a romaji
  transliteration scheme. Record the capture method and the raw screenshots in
  `testdata/pico8/input/` and summarize it in `docs/INPUT.md`.
- Multi-key sequences, small kana, and any dakuten/handakuten behavior are
  whatever PICO-8 does; the capture task is authoritative. If PICO-8 requires
  sequences, the input layer supports a pending-sequence buffer with a 1s
  timeout and a visible pending indicator.
- Kana characters insert canonical Unicode and occupy two cells.
- Kana mode does not change Shift+letter behavior: Shift still inserts the
  symbol glyphs from §1 unless literal mode is active.

### 3. Ctrl+P literal/uppercase mode

The always-authentic rule makes Shift+letter insert glyphs, so uppercase ASCII
needs an explicit escape path.

- `Ctrl+P` in insert mode toggles literal mode.
- In literal mode:
  - Shift+letter inserts the uppercase ASCII letter, not a glyph,
  - `Ctrl+J`/`Ctrl+K` kana modes are disabled while literal mode is active,
  - the status bar shows `LIT` (or the PICO-8-equivalent label if capture shows
    one),
  - `Esc` leaves insert mode and clears literal mode.
- The exact PICO-8 0.2.7 behavior and label for Ctrl+P are captured during M6.
  If PICO-8 calls the mode "puny" and also changes rendering, match the input
  behavior exactly and treat the rendering change as the puny-font variant in
  §7. Do not expose a second, editor-invented toggle.
- Uppercase identifiers must be testable end to end: enter `MY_VAR` in literal
  mode, save, reload, and verify the text and LSP analysis.
- Help text explains that PICO-8 identifiers case-fold, so uppercase is rarely
  needed.

### 4. Glyph picker

- Binding: `Ctrl+Shift+G`; also a command-palette entry `Glyph: pick`.
- Opens a centered popup (40×16) with:
  - a search field,
  - a list of names, glyph previews, byte values, and categories,
  - a preview pane for the selected entry,
  - a footer with `Enter insert`, `Esc close`.
- Data comes from `pico8/glyphs`; do not duplicate the catalog in the editor.
- Custom glyph entries from settings are included by the server. The editor
  only renders what the server returns.
- Filtering is case-insensitive substring on name, alias, byte (`0x80`, `128`),
  and category.
- Navigation: Up/Down, PageUp/PageDown, Home/End, Enter, Esc.
- Insertion:
  - insert mode: insert at the cursor and stay in insert,
  - normal mode: insert at the cursor, then enter insert after the inserted
    glyph,
  - visual mode: replace the selection with the glyph and return to normal.
- After insertion, the picker closes and the cursor advances by the glyph's
  cell width.
- The catalog is cached per LSP settings version; a settings change invalidates
  it.
- If LSP is unavailable, show a notification and do not open an empty picker.

### 5. `p8_left` completion

- pico-lsp returns `p8_left`-style triggers (`p8_` + glyph name, plus aliases)
  as ordinary completion items.
- Typing `p8_` in insert mode triggers completion after the normal 100ms
  debounce.
- Accepting an item inserts the canonical Unicode text from the completion
  `textEdit`/`insertText`; the editor does not maintain its own name table.
- The glyph picker and completion share the server catalog, so a glyph missing
  from one is missing from both; report upstream rather than patching locally.
- Test at least `p8_left`, `p8_right`, `p8_up`, `p8_down`, `p8_heart`, and a
  kana entry, plus one alias.

### 6. Clipboard

- Copy:
  - characterwise: selected text with LF newlines,
  - linewise: selected lines each terminated with `\n`,
  - blockwise: each line of the rectangle, padded to the widest line, joined
    with `\n` (trailing spaces are preserved for block fidelity).
- Paste:
  - read UTF-8 text; strip a leading BOM,
  - normalize CRLF and lone CR to LF,
  - map known canonical glyphs through `pkg/p8scii`; the text is already
    canonical, so this is a validation step, not a conversion,
  - unknown runes are preserved and a one-line warning is shown,
  - insert as characterwise text in insert mode, or via the active register in
    normal mode.
- System paste bindings: `Ctrl+Shift+V` in insert and visual modes; `"+p` and
  `"*p` in normal mode.
- System copy bindings: `Ctrl+Shift+C` in visual mode; `"+y` and `"*y` in
  visual/normal modes.
- Clipboard failures are notifications; never block the edit.

### 7. Puny-font rendering

- Generate a second atlas from the same CC-0 font data using PICO-8's puny
  metrics. The expected metrics are variable-width glyphs with 3×5 ink and 1px
  spacing; verify against PICO-8 0.2.7 and record the exact advance table.
- The puny variant applies to the characters PICO-8's puny font defines. Kana
  and wide glyphs fall back to the default atlas.
- Add a temporary debug toggle in M6; M7 exposes `editor.font = "default" |
  "puny"` in `config.toml`.
- Widths used for cursor motion, tab bar layout, and popup wrapping come from
  the active font metrics. LSP positions remain UTF-16 and are unaffected.
- Puny mode does not change input mapping; it is a rendering variant.
- Golden tests cover every puny glyph and a mixed string with fallback glyphs.

### 8. Help and documentation

- `docs/INPUT.md` contains:
  - the Shift+letter table,
  - kana mode entry/exit and table capture notes,
  - literal mode and the uppercase workflow,
  - picker and completion usage,
  - clipboard behavior,
  - known IME limitation.
- `:help input` opens an in-editor overlay with the same content in condensed
  form.
- The command palette lists `Help: input`, `Glyph: pick`, and
  `Input: toggle literal`.
- The status bar always shows the active input mode (`LIT`, `HIRA`, `KATA`) so
  the user is never trapped.

### 9. Capture tests

PICO-8 is a proprietary binary and is not a CI dependency. Capture is a
one-time, documented manual process:

- Use PICO-8 0.2.7 at 4× scale.
- Enter every key sequence for Shift+letter, kana, and literal modes.
- Save screenshots under `testdata/pico8/input/` with a manifest that maps
  screenshot, keys, and expected canonical glyph.
- Check in the derived tables with a hash and a comment naming the capture.
- CI tests assert table coverage, not PICO-8 behavior. A reviewer compares the
  tables to the screenshots before M6 exits.

## Architecture and files

```text
internal/input/
  p8scii.go        input modes, Shift/kana/literal tables
  p8scii_test.go
internal/font/
  puny.txt         puny glyph data
  atlas_gen.go     both atlases
internal/app/
  glyphpicker.go   picker UI and filtering
  input_ui.go      mode indicator and notifications
internal/lsp/
  glyphs.go        catalog request/cache
docs/INPUT.md
testdata/pico8/input/  screenshots and manifest
```

## Interfaces and state

```go
type P8Mode uint8

const (
    P8ModeDefault P8Mode = iota
    P8ModeHiragana
    P8ModeKatakana
    P8ModeLiteral
)

type P8Input struct {
    Mode    P8Mode
    Pending []rune // kana sequence buffer, if PICO-8 requires one
}

func (p *P8Input) HandleKey(key input.Key, mods input.Mods) (text string, handled bool)
func (p *P8Input) ModeLabel() string
```

`P8Input` is owned by the app, invoked from insert mode before Vim text
handling. It has no access to the buffer or LSP.

## Error handling

- Unknown glyph catalog responses show a notification; never insert a
  placeholder silently.
- Clipboard failure leaves the buffer unchanged and shows the reason.
- Kana pending sequences time out after 1s and insert the pending rune as
  typed; log at debug.
- A missing puny glyph falls back to the default atlas rather than drawing a
  box.
- If the capture table has a gap, M6 cannot exit; add the glyph or document the
  deliberate exclusion.

## Testing

### Coverage

- Every printable byte `16–255` is reachable through at least one path:
  - `128–153` through Shift+letter,
  - `154–229` through kana modes,
  - all through the picker.
- For each glyph: insert, save, reload, compare canonical text and byte mapping.
- `p8_left` completion inserts the same text as the picker.

### Regression

- Normal-mode `J`, `K`, `P`, `X`, `D`, `C`, `Y`, `gg`, `G`, and `$` are
  unaffected with P8SCII input enabled.
- Literal mode: `MY_VAR` round-trips and is analyzed by LSP.
- `Ctrl+P` inside an open completion popup does not navigate completion; it
  toggles literal mode. Completion uses `Ctrl+N`/`Ctrl+Shift+N`.
- Clipboard copy/paste across a cart, a `.lua`, and an external editor.
- Puny/default font switching updates cursor widths and rerenders.

### Manual

- Type a full glyph sheet by hand using Shift+letter and kana modes.
- Use the picker with only the keyboard.
- Save, close, reopen, and diff the cart.

## Exit criteria

- [ ] Every printable P8SCII glyph can be entered and round-trips through
      save/load.
- [ ] Normal-mode Vim Shift commands are unaffected.
- [ ] Uppercase identifier input has a documented, tested escape path.
- [ ] Ctrl+J/K/P modes match captured PICO-8 0.2.7 behavior.
- [ ] Glyph picker and `p8_left` completion work end to end.
- [ ] Clipboard paste maps canonical Unicode and warns on unknown runes.
- [ ] Puny-font rendering is pixel-verified and does not change input mapping.
- [ ] Input mode help is reachable from `:help input` and the palette.
- [ ] `mise run check` passes.

## Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Shift+letter table is wrong | Screenshot capture and manifest; reviewer sign-off; tests use the captured table. |
| Kana behavior is guessed | Capture PICO-8 0.2.7 sequences; support a pending buffer if needed; no invented romaji. |
| Ctrl+P conflicts with completion | Completion uses `Ctrl+N`/`Ctrl+Shift+N`; `Ctrl+P` is input mode only. |
| Picker and completion diverge | Both consume `pico8/glyphs`; no editor-local catalog. |
| Puny metrics break cursor motion | One metrics source for rendering and cells; golden tests and mixed-string tests. |
| Clipboard mangles block text | Define linewise/charwise/blockwise formats; round-trip tests. |
| IME users cannot compose | M1 documents committed-text-only support; kana modes do not depend on IME. |
