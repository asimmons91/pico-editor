# pico-editor

A native PICO-8 code editor that feels like the built-in PICO-8 editor, with
semantic highlighting, pico-lsp language intelligence, a simple file tree, Vim
editing, and authentic P8SCII input. One Go binary, no WebView.

> PICO-8 is a trademark of Lexaloffle Games. This project is not affiliated
> with, sponsored by, or endorsed by Lexaloffle Games.

**Status:** M5 implemented. Automated checks pass; interactive X11/Wayland
spot checks from M1 are tracked in
[docs/spikes/M1-input.md](docs/spikes/M1-input.md), the M3 manual checklist is
in [docs/spikes/M3-vim.md](docs/spikes/M3-vim.md), the M4 cart/tree checklist is
in [docs/spikes/M4-cart.md](docs/spikes/M4-cart.md), and the M5 LSP checklist is
in [docs/spikes/M5-lsp.md](docs/spikes/M5-lsp.md). pico-lsp now runs in-process:
editing a cart or include gets semantic highlighting, diagnostics, completion,
hover, signature help, and go-to-definition, with token/character/compressed
sizes in the status row. See [docs/ROADMAP.md](docs/ROADMAP.md) for M6 (P8SCII
input UX).

With no argument the current directory opens as a workspace with the file tree
focused. Pass a `.p8` cart, `.lua` include, or directory to start there:

```sh
mise exec -- go run ./cmd/pico-editor path/to/game.p8
mise exec -- go run ./cmd/pico-editor path/to/project
```

The M1 charset demo is still available with `--debug-charset`: `1`–`4` change
the window scale, `W` toggles the width overlay, `P` cycles the background
palette, and the arrow keys move the selection.

In the editor, `:help` lists the core keys. Use `i`/`a` to insert, `Esc` to
return to normal mode, `:w` to save, `:q` to close (or `:q!` to discard), and
`:wq`/`:x` to write and close. `Ctrl+S` saves the current buffer. `Ctrl+Tab` and
`Ctrl+Shift+Tab` move between cart code tabs; `Ctrl+Shift+B` toggles the file
tree. `Ctrl+Q` and the window close button offer Save all / Discard all /
Cancel when buffers are dirty. Carts save only the `__lua__` body; the header
and every other section are preserved byte-for-byte. `.p8.png` remains
unsupported. `K` shows hover, `gd`/`Ctrl+]` jumps to a definition, `Ctrl+Space`
opens completion in insert mode, `:diag` lists diagnostics, and `:lsp restart`
restarts the in-process language server.

## Requirements

- Linux (V1 target; the editor layer is kept portable).
- [mise](https://mise.jdx.dev/) for the pinned Go 1.27.4 toolchain and
  `golangci-lint`.
- A working OpenGL environment. Ebitengine selects Wayland or X11
  automatically. Clipboard access is pure Go; on GNOME Wayland it uses
  XWayland, and it degrades to internal registers when unavailable.

## Build and run

```sh
mise install
mise run build     # bin/pico-editor
mise run run       # opens the current directory workspace
mise exec -- go run ./cmd/pico-editor path/to/file.lua
```

`Ctrl+Q` or closing the window quits.

Available tasks:

| Task | Purpose |
| --- | --- |
| `mise run build` | Build `bin/pico-editor` |
| `mise run run` | Run from source |
| `mise run test` | `go test ./...` |
| `mise run lint` | `golangci-lint run` |
| `mise run fmt` | `gofmt -w .` and `go mod tidy` |
| `mise run check` | Format check, lint, test, and build (the gate) |

## Version metadata

Development builds report `pico-editor dev (unknown, unknown)`. Release builds
inject the version, commit, and build date into
`github.com/asimmons91/pico-editor/internal/version` with `-ldflags`.

## Documentation

- [Roadmap and V1 scope](docs/ROADMAP.md)
- [Architecture and invariants](docs/ARCHITECTURE.md)
- [Frozen Vim command matrix](docs/VIM.md)
- [Notices and attributions](NOTICE)
- [M1 input and rendering spike results](docs/spikes/M1-input.md)
- [M3 Vim manual checklist](docs/spikes/M3-vim.md)
- [M4 cart and file tree checklist](docs/spikes/M4-cart.md)
- [M5 in-process LSP checklist](docs/spikes/M5-lsp.md)
- [License](LICENSE)

## Contributing

- `main` is protected. Create a branch, push it, and open a PR; never push
  directly to `main`.
- Use Conventional Commits: `<type>(<optional scope>): <description>`.
- CI runs `mise run check` on pushes to `main` and on every pull request, and
  it must be green before merge.
- Each PR gets a focused code review before merging. Review fixes go on the
  same branch and PR.
- Golden tests are required for cart round-trips, P8SCII glyph coverage and
  widths, and Vim behavior.

## License

MIT. See [LICENSE](LICENSE) and [NOTICE](NOTICE) for the PICO-8 font and
trademark attribution.
