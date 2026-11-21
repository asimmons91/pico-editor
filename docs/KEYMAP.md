# pico-editor keymap

**Status:** V1 defaults, implemented in M7.
**Related:** [VIM.md](VIM.md), [CONFIG.md](CONFIG.md), [USAGE.md](USAGE.md)

Keymaps live in `config.toml` under `[keys.normal]`, `[keys.insert]`,
`[keys.visual]`, and `[keys.command]`. Entries map the canonical key notation
from [VIM.md](VIM.md#key-notation) to a stable command ID:

```toml
[keys.normal]
"ctrl+s" = "file.save"
"ctrl+shift+b" = ""      # unbind
f2 = "tree.refresh"
```

Only global/editor commands are remappable. Vim grammar keys (`d`, `c`, `y`,
motions, text objects, ex commands) and the reserved input/LSP bindings stay
fixed. A user binding replaces the same key's default in that mode, and an empty
string removes it. Duplicate or overridden bindings produce a warning that lists
both commands; the user's binding wins.

## Defaults

| Key | Modes | Command |
| --- | --- | --- |
| `ctrl+s` | normal, insert, visual | `file.save` |
| `ctrl+shift+b` | normal | `tree.toggle` |
| `ctrl+shift+p` | normal, insert | `palette.show` |
| `ctrl+tab` | normal | `cart.tab.next` |
| `ctrl+shift+tab` | normal | `cart.tab.prev` |
| `ctrl+shift+g` | normal, insert, visual | `glyph.pick` |
| `ctrl+q` | normal, insert | `app.quit` |
| `f1` | all | `help.open` |
| `ctrl+shift+m` | normal | `messages.show` |

`Ctrl+Tab` still switches code tabs while inserting so a counted insert can
cross tabs; normal-mode switching is configurable. `Ctrl+Shift+V` and
`Ctrl+Shift+C` remain fixed clipboard bindings, and `Ctrl+P` remains the P8SCII
puny-mode key in insert.

## Command IDs

Every palette entry uses one of these stable IDs. Documentation links point at
this table.

| ID | Title | Category |
| --- | --- | --- |
| `file.save` | File: save | File |
| `file.close` | File: close | File |
| `file.reload` | File: reload | File |
| `buffer.next` | Buffer: next | Buffer |
| `buffer.prev` | Buffer: previous | Buffer |
| `buffer.list` | Buffer: list | Buffer |
| `cart.tab.next` | Cart: next code tab | Cart |
| `cart.tab.prev` | Cart: previous code tab | Cart |
| `tree.toggle` | Tree: toggle file tree | Tree |
| `tree.refresh` | Tree: refresh workspace | Tree |
| `palette.show` | Palette: show commands | Palette |
| `help.open` | Help: keys | Help |
| `help.input` | Help: input | Help |
| `diag.list` | LSP: diagnostics list | LSP |
| `messages.show` | Messages: show history | Messages |
| `lsp.restart` | LSP: restart pico-lsp | LSP |
| `glyph.pick` | Glyph: pick | Glyph |
| `input.puny` | Input: toggle puny | Input |
| `config.reload` | Config: reload settings | Config |
| `app.quit` | App: quit | App |

`Ctrl+Shift+P` opens the palette. F1 opens help; `:help input`, `:help lsp`,
`:help config`, and `:help troubleshooting` open the other topics.

## Conflict and validation rules

- Keys use `[ctrl+][alt+][shift+][meta+]<key>` with lowercase modifiers and key
  names (`ctrl+shift+p`, `f1`, `pagedown`).
- Bindings without ctrl/alt/meta are rejected unless they are `f1`..`f12`,
  because Vim owns the plain-key grammar.
- These reserved bindings cannot be reassigned: `ctrl+b`, `ctrl+f`, `ctrl+p`
  (insert), `ctrl+n` and `ctrl+shift+n`, `ctrl+space`, `ctrl+v`, `ctrl+r`,
  `ctrl+o`, `ctrl+i`, `ctrl+rightbracket` (`Ctrl+]`), `ctrl+d`, `ctrl+u`,
  `ctrl+w`, `ctrl+j`, `ctrl+k`, `ctrl+leftbracket` (`Ctrl+[`, Esc),
  `ctrl+shift+v`/`ctrl+shift+c` (clipboard), and insert-mode
  `ctrl+tab`/`ctrl+shift+tab` (counted-insert tab switching). The same motions
  and jumps are reserved in visual mode.
- Invalid notation warns with the accepted key names and keeps the previous
  binding.
- Unknown command IDs warn at load time and at invocation; a documented default
  binding is restored, or the key is left unbound.
- `:map` is out of scope for V1.
