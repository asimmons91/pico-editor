# AGENTS.md

## Read first

- M0 (foundation) and M1 (font, rendering, and input foundation) are implemented: `cmd/pico-editor`, `internal/app`, `internal/font`, `internal/fontgen`, `internal/render`, `internal/input`, `internal/version`, mise tasks, and CI exist. `docs/ROADMAP.md` links one detailed spec per milestone (`docs/M0-foundation.md` through `docs/M8-linux-release.md`); start with the roadmap, then read `docs/ARCHITECTURE.md` and `docs/VIM.md`. M1 results and the remaining interactive spot checks live in `docs/spikes/M1-input.md`.
- `docs/ROADMAP.md` is the source of truth for V1 scope, locked decisions, milestones, and deferred work. Read it before adding features; update it when a locked decision changes.
- Do not implement post-V1 items without explicit approval: full Vim macros/marks/plugins/splits, references/rename/formatting/code actions, `.p8.png`, run-cart, macOS/Windows packaging.

## Stack and boundaries

- Go 1.27.4, pinned in `mise.toml`; module `github.com/asimmons91/pico-editor`.
- Stack: upstream Ebitengine 2.10+ (`github.com/hajimehoshi/ebiten/v2`), a checked-in 1-bit P8SCII glyph atlas, a custom Vim core, and pico-lsp in-process. Linux-first.
- Do not vendor or fork Rune, and do not use Rune's Ebitengine fork. It is reference material only. Its GPL-3.0 code cannot be copied into this repo unless the license decision changes.
- Upstream pico-lsp is a separate repo at `/var/home/austin/projects/pico-lsp`. Read its `AGENTS.md` before changing it; it has its own branch/PR/test rules.
- The editor imports public pico-lsp packages (`pkg/p8scii` now; `pkg/cart` before M4; `pkg/lsp`/`pkg/glyphs` before M5). Go forbids importing another module's `internal/` packages. Use a local `replace` during development and pin a release before shipping.
- Do not duplicate cart parsing, P8SCII mapping, or language analysis if an upstream pico-lsp package can own it. Do not add a second Lua/PICO-8 parser for highlighting; use pico-lsp semantic tokens.

## Domain constraints

- P8SCII widths are not Unicode widths: printable glyphs 16–127 are 4px/1 cell; 128–255 are 8px/2 cells. Use an explicit P8SCII width table, not `runewidth`/grapheme width.
- `.p8` code lives in `__lua__`; code tabs start with `-->8` at line start. The editor shows only code, but saves must preserve the header and every non-code section byte-for-byte. Do not normalize or rewrite those sections.
- `.p8` files store canonical Unicode forms of P8SCII glyphs; use pico-lsp's `p8scii` mapping. Do not invent glyph mappings.
- LSP positions are UTF-16. Store canonical Unicode text and use a P8SCII tokenizer only for display width and cursor motion.
- P8SCII input is always authentic: Shift+letter inserts glyphs in insert mode. Do not add a P8 input toggle. Normal-mode Vim Shift commands must remain unaffected. PICO-8 identifiers case-fold, so uppercase input is rarely required; use PICO-8's puny/literal behavior for the escape path.
- PICO-8's font is CC-0. Render it as a 1-bit atlas; do not replace it with a generic pixel font or rely on runtime TTF anti-aliasing.
- V1 Vim is only the curated core in the roadmap. Do not grow it into full Vim without updating the roadmap.

## Commands

- `mise install` installs the pinned Go toolchain and `golangci-lint`; `mise exec -- go version` should report `go1.27.4`.
- `mise run check` is the gate: `fmt-check`, `lint`, `test`, and `build`. Also available: `mise run build`, `mise run run`, `mise run test`, `mise run lint`, and `mise run fmt`.
- Use `mise exec -- go <cmd>` for Go commands not exposed as tasks. `go build ./...`, `go test ./...`, `go vet ./...`, and `gofmt` work directly.
- CI runs `mise run check` with `CGO_ENABLED=0` and `GOFLAGS=-mod=readonly`. Do not add toolchain versions to workflows; `mise.toml` is the only source.
- pico-lsp `v1.6.0` is pinned for `pkg/p8scii`; `pkg/cart` and `pkg/lsp`/`pkg/glyphs` follow the pin schedule in `docs/ARCHITECTURE.md` §12. Never import pico-lsp `internal/` packages.

## Working agreements

- `main` is protected: never push directly to it. Create a branch, push it, and open a PR.
- CI runs `mise run check` on pushes to `main` and on every PR; it must be green before merge.
- Use Conventional Commits for every commit: `<type>(<optional scope>): <description>`. Allowed types: `feat`, `fix`, `chore`, `docs`, `refactor`, `test`, `build`, `ci`, `perf`. Use imperative, lowercase descriptions with no trailing period. Mark breaking changes with `!` and/or a `BREAKING CHANGE:` footer.
- After opening a PR, load the `code-review` skill with the `skill` tool and review that PR (pass its URL or number). Fix the findings and push them to the same branch/PR before merging; do not open a separate PR for review fixes.
- Golden-test cart round-trips, P8SCII glyph coverage/widths, and Vim behavior. Use Ebitengine 2.10's headless VM/screenshot support for UI tests when possible.
- Keep the Go version in `go.mod` and `mise.toml` in sync.
