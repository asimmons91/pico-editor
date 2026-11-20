# pico-editor configuration

**Status:** V1 schema, implemented in M7.
**Related:** [KEYMAP.md](KEYMAP.md), [USAGE.md](USAGE.md)

Settings are TOML with two layers:

1. Global: `$XDG_CONFIG_HOME/pico-editor/config.toml`
   (`~/.config/pico-editor/config.toml` when `XDG_CONFIG_HOME` is unset).
2. Workspace: `<root>/.pico-editor.toml`.

The workspace layer wins. Tables merge field by field; arrays replace the lower
layer. Missing files are fine. A missing `version` is treated as `1`; any other
version is rejected for forward compatibility.

`config.reload` in the palette, `:reload`, and refresh-on-focus reload both
layers. Values that cannot apply live (the window scale) notify that a restart
is required. Malformed global TOML opens a startup dialog with **Use defaults**
and **Quit**; a malformed workspace file warns and keeps the global layer.

```toml
version = 1

[editor]
scale = 4                 # 0 = automatic (2..8); 1..8 fixed
tab_width = 2
scrolloff = 2
blink_ms = 500

[files]
show_hidden = false
exclude = [".git", "node_modules", "vendor", "bin", "dist"]
max_entries = 20000

[lsp]
enabled = true
auto_complete = true
debounce_ms = 120

[notifications]
timeout_ms = 4000
history = 100

[recovery]
enabled = true
interval_ms = 2000
```

## `[editor]`

| Key | Type | Default | Range | Meaning |
| --- | --- | --- | --- | --- |
| `scale` | integer | `4` | `0`..`8` | `0` auto-sizes to 80% of the monitor clamped to 2×–8×; `1`..`8` is fixed. |
| `tab_width` | integer | `2` | `1`..`16` | Tab insertion and display width in cells. |
| `scrolloff` | integer | `2` | `0`..`32` | Lines kept visible around the cursor. |
| `blink_ms` | integer | `500` | `0`..`60000` | Cursor blink half-period; `0` keeps the cursor solid. |

## `[files]`

| Key | Type | Default | Range | Meaning |
| --- | --- | --- | --- | --- |
| `show_hidden` | boolean | `false` | — | Include dotfiles and dot-directories in the tree. |
| `exclude` | array of strings | see above | — | Base names skipped in the tree. The workspace array replaces the global array. |
| `max_entries` | integer | `20000` | `1`..`1000000` | Scan cap; exceeding it truncates the tree with a warning. |

## `[lsp]`

| Key | Type | Default | Range | Meaning |
| --- | --- | --- | --- | --- |
| `enabled` | boolean | `true` | — | Start the in-process pico-lsp service. When false all LSP UI is hidden. |
| `auto_complete` | boolean | `true` | — | Open completion automatically while insert mode types an identifier. `Ctrl+Space` still requests it manually. |
| `debounce_ms` | integer | `120` | `0`..`5000` | Delay before sending document changes to pico-lsp. `0` uses the server default. |

### Custom glyphs

`[[lsp.glyphs]]` adds project-specific entries to the P8SCII picker and
`p8_` completion. pico-lsp validates and merges them; exactly one of `byte`,
`glyph`, or `text` is required.

```toml
[[lsp.glyphs]]
name = "logo"
byte = 128
context = "both"          # both | string
aliases = ["mark"]
description = "project logo"
```

| Key | Type | Required | Meaning |
| --- | --- | --- | --- |
| `name` | string | yes | Stable glyph name; completion uses `p8_<name>`. |
| `byte` | integer | one of | P8SCII byte 0–255. |
| `glyph` | string | one of | A literal glyph name or canonical text understood by pico-lsp. |
| `text` | string | one of | Insertion text for the glyph. |
| `context` | string | no | `both` (default) or `string`. |
| `aliases` | array of strings | no | Extra completion triggers. |
| `description` | string | no | Picker/help text. |

The workspace array replaces the global array.

## `[notifications]`

| Key | Type | Default | Range | Meaning |
| --- | --- | --- | --- | --- |
| `timeout_ms` | integer | `4000` | `0`..`600000` | Auto-dismiss time for info/success/warning toasts. Errors persist until dismissed with Esc/Enter in normal mode. |
| `history` | integer | `100` | `0`..`10000` | Entries kept for `:messages`. |

## `[recovery]`

| Key | Type | Default | Range | Meaning |
| --- | --- | --- | --- | --- |
| `enabled` | boolean | `true` | — | Journal dirty buffers for crash recovery. |
| `interval_ms` | integer | `2000` | `100`..`600000` | Idle time before writing a dirty buffer to the journal. |

Recovery files live in `$XDG_STATE_HOME/pico-editor/recovery/`. A cart journal
stores `FullText()` so the header and non-code sections are preserved. Recovery
never writes the original file; it opens a dirty buffer and waits for an
explicit save.

## `[keys.*]`

See [KEYMAP.md](KEYMAP.md) for every command ID, the defaults, validation, and
reserved bindings.

```toml
[keys.normal]
"ctrl+s" = "file.save"
"ctrl+shift+b" = ""       # unbind
f2 = "tree.refresh"
```

## Warnings and fallbacks

- Unknown tables and keys warn once and are ignored.
- Invalid values reset to the documented default and warn.
- Invalid key notation or reserved keys warn and keep the previous binding.
- `version` values other than `1` are a configuration error.
- Notifications and logs never include file contents or clipboard text.
