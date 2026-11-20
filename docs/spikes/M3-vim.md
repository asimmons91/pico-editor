# M3 Vim curated core results

**Date:** 2026-11-20
**Status:** Automated checks complete; the interactive checklist below remains
for the milestone owner.
**Milestone:** [M3 — Vim curated core](../M3-vim-core.md)

## Environment

| Component | Version / detail |
| --- | --- |
| OS | Bluefin 44 (Fedora 44 base), kernel `7.2.11-200.fc44.x86_64` |
| CPU | AMD Ryzen 7 7840U |
| Go | 1.27.4 (mise) |
| Ebitengine | `v2.10.6` |
| pico-lsp | `v1.6.0` (`pkg/p8scii`) |

## Automated results

- `internal/vim/testdata/matrix/*.vimtest` contains 221 fixture cases and
  covers every frozen row in [`../VIM.md`](../VIM.md). The coverage test fails
  when a `matrix:` ID is missing or duplicated.
- The fixture runner asserts buffer text, cursor, mode, visual selection,
  registers, viewport top/left, notifications, bells, clipboard text, and
  queued application actions. Scripted action handling lets `:w`, `:q`,
  `:e`, buffer switching, `:help`, `:ls`, and clipboard registers run headless.
- Fuzz target `FuzzHandle` ran for 10 seconds with 16 workers (about 170k
  executions) without a panic, invalid UTF-8, or a cursor off a cluster
  boundary.
- `TestRandomUndoRestoresBuffer` runs 300 deterministic random edit sessions
  and undoes back to the original text each time.
- `go test -race ./...` passes.
- `mise run check` passes (format, lint, tests, build).
- `BenchmarkKeystroke64K`: **360 ns/op** for a motion in an 8,000-line 64KB
  buffer on the reference machine, below the 2ms budget. No benchmark gate is
  enforced on shared CI.

## Resolved decisions

- No file argument opens an empty unnamed buffer; `--debug-charset` keeps the
  M1 charset demo.
- M3 provides a minimal `.lua` multi-buffer session. `:tabn`/`:tabp` report one
  code tab until M4 lands the cart adapter.
- Vim returns actions for filesystem and clipboard work. Saves snapshot on the
  Update goroutine, write in the background, and commit the save point when the
  result arrives.
- `"+`/`"*` paste reads asynchronously; a cached register is only the fallback
  when the clipboard is unavailable.
- Normal-mode Backspace/Space use Vim h/l line-wrap semantics.
- `:help` opens a minimal scrollable overlay; M7 expands the topics.

## Manual checklist

- [ ] Edit a real `.lua` file for 30 minutes using only Vim keys. Repeat on a
      cart code tab after M4 adds the cart adapter.
- [ ] Verify normal-mode Shift commands (`J`, `K`, `P`, `X`, `D`, `C`, `Y`)
      with M6 P8SCII input enabled.
- [ ] Run `:%s/old/new/g` on a 2,000-line file and undo it.
- [ ] Verify visual-block indent, block `I`/`A`, block paste, and block change.
- [ ] Verify `:q`, `:q!`, `:wq`, and the window close button on a dirty buffer.
- [ ] Verify `"+y`/`"+p` against an external editor on X11 and Wayland.
- [ ] Open `:help` and each topic, scroll, and close with Esc.
- [ ] Check the status row, command-line prompt, search highlights, and cursor
      in insert, normal, visual, line, and block modes at 2x and 4x.
- [ ] Check buffer switching with `:e`, `:bn`, `:bp`, `:b N`, and `:ls`.

## Known limitations

- Cart code tabs, the file tree, and workspace includes are M4 work. M3's demo
  uses a standalone `.lua` buffer.
- Completion, diagnostics, hover, signature help, definition jumps, and
  `:diag` are M5 work. The keys are inert or display a milestone notice.
- P8SCII Shift+letter, kana, and literal input modes, the glyph picker, and
  clipboard paste bindings are M6 work. Normal-mode Shift commands already work
  and are covered by fixtures.
- The compact segmented status bar, command palette, TOML settings, and full
  help topics are M7 work.
