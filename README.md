# pico-editor

A native PICO-8 code editor that feels like the built-in PICO-8 editor, with
semantic highlighting, pico-lsp language intelligence, a simple file tree, Vim
editing, and authentic P8SCII input. One Go binary, no WebView.

> PICO-8 is a trademark of Lexaloffle Games. This project is not affiliated
> with, sponsored by, or endorsed by Lexaloffle Games.

**Status:** M0 foundation. The repository builds, tests, and opens an empty
256×256 PICO-8-black canvas at 4× (1024×1024). Editor, cart, Vim, and LSP
features land in later milestones; see [docs/ROADMAP.md](docs/ROADMAP.md).

## Requirements

- Linux (V1 target; the editor layer is kept portable).
- [mise](https://mise.jdx.dev/) for the pinned Go 1.27.4 toolchain and
  `golangci-lint`.
- A working OpenGL environment. Ebitengine selects Wayland or X11
  automatically.

## Build and run

```sh
mise install
mise run build     # bin/pico-editor
mise run run       # opens the empty canvas
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
