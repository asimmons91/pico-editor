# M5 — in-process pico-lsp results

**Status:** Implemented. Automated integration tests pass; interactive
X11/Wayland spot checks remain to be recorded.

## Pin and upstream

pico-lsp `v1.6.0` includes all four public packages: `pkg/p8scii`, `pkg/cart`,
`pkg/lsp`, and `pkg/glyphs`. M17–M19 were released together, so the editor keeps
the existing `v1.6.0` pin and does not need a local `replace` or the previously
planned `v1.8.0` tag.

## What landed

- `internal/lsp` starts pico-lsp over a `net.Pipe`, negotiates UTF-16, sends
  workspace folders and `initializationOptions.pico8`, and syncs full cart and
  `.lua` text with a 120 ms debounce.
- Semantic tokens, diagnostics, completion, hover, signature help, definition,
  `pico8/cartInfo`, and `pico8/glyphs` are cached per URI/version. Results are
  applied on the Update goroutine; a single background writer preserves
  notification order.
- The M7 final semantic color table is already in use. Token priority is fixed
  as comment > string > number > keyword > type > function > property >
  variable > operator > parameter > namespace.
- UI: diagnostics underlines and `:diag`; completion popup (`Ctrl+Space`,
  `Ctrl+N`/`Ctrl+Shift+N`, `Enter`/`Tab`, popup-first `Esc`); `K` hover;
  signature help; `gd`/`Ctrl+]` definition; cross-file jump actions; persistent
  failure status and `:lsp restart`.
- Status row suffix: `E/W` counts, `T/C/Z` cart budgets with compressed-size
  color, and `LSP`/`LSP!`. Standalone `.lua` shows `T/C/Z n/a`.
- Restart keeps the editor usable and replays open documents. A failed start or
  crash never blocks editing, saving, Vim, or quitting.

## Automated coverage

`mise run check` (format, lint, tests, build) and `go test -race ./...` pass.

- `internal/lsp`: cluster expansion for UTF-16 token ranges, cart-base-line
  position round trips, token priority and modifiers, settings normalization,
  real-server semantic tokens, token refresh after edits, diagnostics, cart
  info, completion, hover, signature help, definition, `p8_left` glyph
  completion, save-with-pending-edit ordering, stop-during-start, pipe close,
  and restart.
- `internal/app`: semantic highlighting and diagnostic underlines drawn in the
  code area, completion acceptance (including multiple additional edits),
  automatic completion and signature triggers, hover, definition and jump-back,
  both same-cart and cross-tab definitions, diagnostics list jump, status
  segments, cart info after restart, and restart usability.
- `internal/vim`: matrix fixtures for `K`, `gd`, `Ctrl+]`, `:diag`, and
  `:lsp restart`, plus a cross-file jump action.
- Fixtures under `internal/lsp/testdata` are copied from pico-lsp `v1.6.0`;
  see the provenance README there.

## Manual checklist

Run on X11 and Wayland:

- [ ] Open a cart with an error; keywords, strings, numbers, comments, and
      symbols are colored; errors are underlined; `E/W` appears in the status
      row.
- [ ] Edit continuously for five minutes; highlighting and diagnostics track
      the edit without stale ranges.
- [ ] `Ctrl+Space` completes a prefix and `p8_left`; accepting inserts the
      canonical P8SCII glyph.
- [ ] `K` shows hover; `Esc` closes it; a motion closes it and still moves.
- [ ] Typing `(` or `,` shows signature help; the active parameter is
      highlighted.
- [ ] `gd` jumps into a second cart tab and into an included `.lua`; `Ctrl+O`
      returns.
- [ ] `:diag` lists diagnostics; `Enter` jumps to the selected one.
- [ ] Save updates `didSave` and the `T/C/Z` status counts.
- [ ] `:lsp restart` recovers after a forced server failure without losing
      unsaved edits.
- [ ] `Ctrl+P` remains reserved for P8SCII input; completion only uses
      `Ctrl+N`/`Ctrl+Shift+N`.

## Known limitations for M6/M7

- M7 replaces the compact status suffix and the persistent failure message with
  the segmented status bar and notification system.
- The glyph picker UI is M6; M5 only caches the `pico8/glyphs` catalog.
- The final semantic color table is already M7's table, but M7 still owns the
  status layout, notification history, and command palette.
