# M6 input capture and results

**Date:** 2026-11-20
**Status:** Automated checks complete; interactive spot checks remain for the
milestone owner.
**Milestone:** [M6 — P8SCII input UX](../M6-p8scii-input.md)

## Environment

| Component | Version / detail |
| --- | --- |
| OS | Bluefin 44 (Fedora 44 base) |
| PICO-8 | 0.2.7 |
| Capture | Xvfb container, 512×512 PICO-8 window, `xdotool` key injection |
| Editor | `github.com/asimmons91/pico-editor` at M6 |
| pico-lsp | `v1.7.0` (complete printable glyph catalog) |

## Capture

- Screenshots, typed key lists, saved code bodies, and the manifest are in
  [`testdata/pico8/input/`](../../testdata/pico8/input/).
- The manifest SHA-256 is recorded in `internal/input/p8scii.go`.
- Shift+A–Z was captured one glyph per line and matches bytes `0x80`–`0x99`.
- Hiragana and Katakana were captured by typing romaji one sequence per line and
  saving with `Ctrl+S`; the saved `.p8` bodies are the authoritative expected
  values. A broad CV/digraph/alias/c-row/invalid sweep, the doubled-consonant
  and `n` edge cases, and the ten Shift+digit symbols are also checked in.
- Ctrl+P was captured separately: the status reads `PUNY FONT MODE (CTRL-P)`,
  unshifted letters save as uppercase ASCII, and Shift+letter saves standard
  lowercase ASCII. It does not change rendering.
- Puny glyphs are the existing uppercase-ASCII slots in the checked-in atlas;
  no second atlas is needed.

## Automated results

- `internal/input` covers modes, Shift+letter, puny case swapping, kana
  sequences, invalid-sequence behavior, doubled consonants, `n` handling, and
  Shift+digit symbols.
- `internal/app/m6_test.go` covers save/reload, normal-mode regression, puny
  round-trip, kana insertion, picker filtering/insertion (normal, insert, and
  visual), catalog coverage, `:glyph`/`:input`, the command palette, completion
  (`p8_left`, `p8-left`, kana), and clipboard bindings.
- pico-lsp `v1.7.0` has a catalog test asserting exactly one entry for every
  printable byte `0x10`–`0xff`.

## Manual checklist

1. In a real terminal, type the full Shift+letter sheet and compare it with
   PICO-8 0.2.7.
2. Enter `Ctrl+J`, type `ka`, `kya`, `nnka`, and `xtsu`; repeat with `Ctrl+K`.
3. Enter `Ctrl+P`, type `my_var`, save, reload, and confirm `MY_VAR`.
4. Open the picker with only the keyboard; filter by `0x87`, `kana`, and
   `p8-left`; insert in insert/normal/visual modes.
5. Copy a linewise and blockwise selection to an external editor and paste it
   back; confirm block padding and unknown-rune warning.
6. Run `mise run check` and confirm `docs/INPUT.md` matches the status bar.

## Known limitations

- IME preedit and candidate placement remain unsupported (M1 limitation).
- PICO-8's kana pending buffer has no timeout; the editor matches that.
- Pasting a lone CR normalizes it to LF at the clipboard boundary as M6
  requires; editor-internal edits still preserve a lone CR as P8SCII data.
