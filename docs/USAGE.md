# pico-editor usage

**Status:** V1 quick start, implemented in M7.

## Open a project or file

```sh
pico-editor                 # current directory, file tree focused
pico-editor path/to/project # workspace root
pico-editor path/to/game.p8 # cart code view
pico-editor path/to/lib.lua # include or standalone Lua file
```

A `.p8` cart opens as code tabs. Only `__lua__` is editable; the header,
`__gfx__`, `__sfx__`, `__music__`, `__map__`, `__gff__`, and `__label__` are
preserved byte-for-byte on save. A `.p8.png` cart is not supported in V1.

`Ctrl+Shift+B` toggles the tree. `Ctrl+Tab` / `Ctrl+Shift+Tab` switch cart code
tabs in normal mode. Per-tab cursor and scroll state are preserved.

## Edit and save

Vim is the editing model: `i`/`a`/`o` insert, `Esc` returns to normal, and
`:w` writes. `Ctrl+S` saves from normal, insert, and visual modes. `:q` closes
the current buffer; `:q!` discards; `:wq`/`:x` write and close. Dirty buffers
get a Save all / Discard all / Cancel prompt on quit.

The complete V1 command matrix is in [VIM.md](VIM.md).

## Language intelligence

pico-lsp runs in-process. With a cart or include open:

- Semantic highlighting uses the PICO-8 palette.
- `K` shows hover; `gd` or `Ctrl+]` jumps to a definition.
- `Ctrl+Space` opens completion; `Ctrl+N` / `Ctrl+Shift+N` select; Enter/Tab
  accepts.
- Typing `(` or `,` opens signature help.
- `:diag` lists diagnostics; `:lsp restart` restarts the server.
- The status bar shows `E`/`W`, token/character/compressed sizes, and `LSP` or
  `LSP!`.

Set `[lsp] enabled = false` in the config to run without the server.

## P8SCII input

Insert mode is always authentic PICO-8 input. Shift+letter inserts glyphs;
`Ctrl+J` is Hiragana, `Ctrl+K` Katakana, and `Ctrl+P` puny mode. `Ctrl+Shift+G`
(or `:glyph`) opens the searchable glyph picker. `Ctrl+Shift+V` pastes and
`Ctrl+Shift+C` copies a visual selection. Normal-mode Vim Shift commands are
never converted. See [INPUT.md](INPUT.md) for the full tables and the uppercase
escape path.

## Command palette

`Ctrl+Shift+P` opens the palette. Type to fuzzy-filter by title, category, or
command ID. Up/Down/PageUp/PageDown move, Enter runs, Esc closes. The active
mode's key binding is shown on the right. Every V1 feature has a palette entry;
the stable IDs and defaults are in [KEYMAP.md](KEYMAP.md).

Useful palette entries and commands:

- `File: save`, `File: close`, `File: reload`
- `Buffer: next/previous/list`
- `Tree: toggle file tree`, `Tree: refresh workspace`
- `LSP: diagnostics list`, `LSP: restart pico-lsp`
- `Glyph: pick`, `Input: toggle puny`
- `Messages: show history`, `Config: reload settings`
- `Help: keys`, `Help: input`; `:help config` and `:help troubleshooting`
- `App: quit`

## Notifications and messages

Toasts appear in the top-right and replace the rightmost status segments.
Info/success/warning toasts auto-dismiss after `notifications.timeout_ms`;
errors stay until Esc or Enter in normal mode. In normal mode, `:messages` or
`Ctrl+Shift+M` opens the history with severity and timestamps. Repeated
identical messages within two seconds increment a counter instead of stacking.

## External changes, focus, and recovery

On window focus, clean buffers reload automatically, dirty buffers get the M4
conflict dialog, the file tree rescans, and both config files reload when their
mtime changes. There is no background watcher in V1.

Dirty buffers are journaled under
`$XDG_STATE_HOME/pico-editor/recovery/` after two seconds of idle time, on focus
loss, and when a quit is cancelled. On startup a modal list offers Recover,
Discard, or Later for each entry. Recovery opens a dirty buffer and never writes
the original file; save explicitly after reviewing it.

## Scaling and layout

The canvas is 256×256 logical pixels (a 64×42 cell grid) and is drawn with
nearest-neighbor integer scaling. `editor.scale = 0` picks the largest scale
that fits 80% of the monitor, clamped to 2×–8×. Fixed values 1–8 are also
accepted. Scale changes need a restart.

The file tree is a 20-column sidebar when open and reduces the editor width
instead of covering it. The editor has a four-cell gutter with diagnostic signs
and relative line numbers; the current line number is absolute. File and cart
tabs form the bufferline, and the status row spans the full canvas width with
the mode block on the left and position, sizes, LSP, and percentage on the
right. `Ctrl+Shift+B` toggles the sidebar.

## Logs and troubleshooting

The rotating log is `$XDG_STATE_HOME/pico-editor/pico-editor.log` (1MB, three
backups). `--log-level debug` and `--log-file -` are useful for bug reports.
See [TROUBLESHOOTING.md](TROUBLESHOOTING.md) and [CONFIG.md](CONFIG.md).
