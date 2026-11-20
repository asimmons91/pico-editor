# Troubleshooting

**Status:** V1, implemented in M7.

## The window is blurry or too small

- Use an integer scale. `editor.scale = 4` or another fixed value avoids
  fractional window scaling; `0` selects automatic sizing.
- Wayland fractional scaling can letterbox the canvas. The editor intentionally
  uses nearest-neighbor integer scaling, so a fractional compositor scale may
  look softer than a fixed integer scale.
- Scale changes apply on restart.

## The editor does not start

- Confirm a working OpenGL environment. Ebitengine selects Wayland or X11
  automatically.
- Run with `--log-file -` to see logs on stderr.
- If an audio backend warning appears, it is harmless: V1 plays no audio
  but Ebitengine may initialize one.
- If a configuration file is malformed, startup offers **Use defaults** or
  **Quit**. The same dialog appears for an unsupported `version`.
- Check `$XDG_STATE_HOME/pico-editor/pico-editor.log` for the reason. Logs
  rotate at 1MB and keep `.1`, `.2`, and `.3`.

## Clipboard does not work

- pico-lsp and the editor use `golang.design/x/clipboard`. On GNOME Wayland it
  may go through XWayland. If the clipboard is unavailable, `"+`/`"*` registers
  fall back to internal registers and a notification explains the failure.
- `Ctrl+Shift+V` pastes; `Ctrl+Shift+C` copies a visual selection.

## IME or kana input

Ebitengine 2.10 provides committed text only; the editor does not implement an
IME candidate window. `Ctrl+J` Hiragana, `Ctrl+K` Katakana, and `Ctrl+P` puny
mode are built-in P8SCII input and do not require an IME. See
[INPUT.md](INPUT.md).

## LSP does not respond

- The status shows `LSP!` and a persistent notification when the server fails.
- Run `:lsp restart` or pick **LSP: restart pico-lsp** in the palette.
- Check the log for the `component=lsp` records. Routine request errors go to
  `:messages`, not to toasts.
- Set `[lsp] enabled = false` to keep editing without the server.

## A file changed on disk

- Clean buffers reload on focus.
- Dirty buffers get a conflict dialog: Overwrite, Reload, or Cancel. Overwrite
  creates a backup in the same directory before writing.
- `:e!` discards local changes and reloads; `:e` refuses a dirty buffer.

## Recover unsaved work

1. Start the editor. If a journal exists, the recovery list shows each path and
   timestamp.
2. Press `R` (or Enter) to open the entry as a dirty buffer, `D` to discard it,
   or `L`/Esc to keep it for later.
3. Review the buffer and save explicitly. Recovery never writes the original
   file automatically.

Journals are in `$XDG_STATE_HOME/pico-editor/recovery/` (one JSON and one text
file per dirty buffer, capped at 100 entries and 1MB each). Disable with
`[recovery] enabled = false`.

## keybindings do not work

- Vim grammar keys are fixed. Use a ctrl/alt/meta binding or `f1`..`f12`.
- `ctrl+p` is reserved for puny mode in insert.
- `Ctrl+Shift+P` opens the palette; `F1` opens help.
- Check the warnings in `:messages`; invalid keys and duplicate bindings are
  reported there. See [KEYMAP.md](KEYMAP.md).

## Where are the files?

| Purpose | Path |
| --- | --- |
| Global config | `$XDG_CONFIG_HOME/pico-editor/config.toml` |
| Workspace config | `<root>/.pico-editor.toml` |
| Log | `$XDG_STATE_HOME/pico-editor/pico-editor.log` |
| Recovery | `$XDG_STATE_HOME/pico-editor/recovery/` |

`XDG_CONFIG_HOME` and `XDG_STATE_HOME` fall back to
`~/.config` and `~/.local/state`.
