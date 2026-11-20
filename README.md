# pico-editor

A native PICO-8 code editor that feels like the built-in PICO-8 editor, with
semantic highlighting, pico-lsp language intelligence, a simple file tree, Vim
editing, and authentic P8SCII input. One Go binary, no WebView.

> PICO-8 is a trademark of Lexaloffle Games. This project is not affiliated
> with, sponsored by, or endorsed by Lexaloffle Games.

**Status:** M3 implemented. Automated checks pass; interactive X11/Wayland
spot checks from M1 are tracked in
[docs/spikes/M1-input.md](docs/spikes/M1-input.md), and the M3 manual checklist
is in [docs/spikes/M3-vim.md](docs/spikes/M3-vim.md). The editor now has the
curated Vim core from [docs/VIM.md](docs/VIM.md): normal, insert, visual
character/line/block, motions, operators, text objects, registers, search,
undo/redo, dot repeat, and ex commands including `:s`. See
[docs/ROADMAP.md](docs/ROADMAP.md) for the next milestone (M4: carts and the
file tree).

With no argument an empty Vim buffer opens. Pass a `.lua` file to edit it:

```sh
mise exec -- go run ./cmd/pico-editor path/to/file.lua
```

The M1 charset demo is still available with `--debug-charset`: `1`–`4` change
the window scale, `W` toggles the width overlay, `P` cycles the background
palette, and the arrow keys move the selection.

In the editor, `:help` lists the core keys. Use `i`/`a` to insert, `Esc` to
return to normal mode, `:w` to save, `:q` to close (or `:q!` to discard), and
`:wq`/`:x` to write and close. `Ctrl+Q` and the window close button also refuse
to discard unsaved changes. `.p8` carts are rejected until M4 can preserve
non-code sections.

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
mise run run       # opens an empty Vim buffer
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
