# M0 — Foundation and decisions

**Status:** Implemented; exit pending upstream pico-lsp schedule and manual resize verification
**Depends on:** none
**Blocks:** M1
**Roadmap:** [ROADMAP.md](ROADMAP.md)
**Estimate:** 2–3 days

## Outcome

The repository builds, tests, and runs an empty PICO-8-sized window through one
command. The decisions that would otherwise leak into every later milestone —
canvas geometry, settings format, license, dependency pinning, and the Vim
matrix — are written down and enforced by CI.

**Demo:** `mise run check` passes locally and in CI. `mise run run` opens a
1024×1024 window (256×256 logical canvas at 4×) filled with PICO-8 black. The
window resizes with integer scaling and never blurs.

## Scope

### In scope

- Go module and toolchain wiring.
- `cmd/pico-editor` entry point, flags, logging, and version metadata.
- Minimal Ebitengine game loop and empty window.
- `docs/ARCHITECTURE.md` with package boundaries and cross-cutting invariants.
- `docs/VIM.md` with the frozen V1 command matrix.
- `mise.toml` tasks for `build`, `run`, `test`, `lint`, `fmt`, and `check`.
- `golangci-lint` configuration.
- GitHub Actions CI using mise.
- MIT license, font/trademark notices, and README disclaimer.
- Dependency pin policy, including the pico-lsp release schedule.
- Repository hygiene: `.gitignore`, README skeleton, issue/PR expectations.

### Out of scope

- Font data, rendering, input handling, and clipboard/IME work (M1).
- Text buffers, editing, search, and file IO (M2).
- Vim behavior (M3).
- Cart parsing, file tree, and workspace handling (M4).
- pico-lsp imports and LSP behavior (M5). M0 only records the pin policy.
- Settings parsing and keymap implementation (M7). M0 only freezes the format.
- Packaging, release automation, and install docs (M8).

## Deliverables

1. `cmd/pico-editor/main.go` and a minimal `internal/app` that opens the empty
   window.
2. `docs/ARCHITECTURE.md` describing the decisions below.
3. `docs/VIM.md` containing the complete matrix from
   [M3](M3-vim-core.md#2-command-matrix); M0 freezes it and M3 implements it.
4. `mise.toml` with the six required tasks and pinned tools.
5. Ebitengine pinned to `v2.10.6` and a verified `go.sum`.
6. `internal/version` with `Version`, `Commit`, and `BuildDate` injection.
7. `.golangci.yml` and a clean lint run.
8. `.github/workflows/ci.yml` running `mise run check` on pushes and PRs.
9. `LICENSE` (MIT), `NOTICE` (PICO-8 font attribution and trademark disclaimer),
   and a README with the same disclaimer.
10. `.gitignore` for build outputs, editor state, and local recovery files.
11. A written pico-lsp dependency plan:
    - `pkg/p8scii` in pico-lsp `v1.6.0`, required before M1.
    - `pkg/cart` in pico-lsp `v1.7.0`, required before M4.
    - `pkg/lsp` and `pkg/glyphs` in pico-lsp `v1.8.0`, required before M5.
    - Local `replace` during development; released version pinned before each
      milestone exits.

## Detailed requirements

### 1. Module and toolchain

- Module path: `github.com/asimmons91/pico-editor`.
- `go.mod` declares `go 1.27.4`; `mise.toml` pins Go to `1.27.4`. These must
  stay identical.
- No `go.work` is committed. Development against pico-lsp uses a `replace`
  directive that is removed before release. CI never needs a sibling checkout
  once a milestone pins a release.
- Dependencies are pinned to exact versions. Do not take Ebitengine
  `v2.11.0-alpha` or any pre-release.
- `go mod tidy` is run before every milestone exit; CI uses `-mod=readonly`.

### 2. Entry point and window

- `main.go` parses flags, configures logging, and calls `ebiten.RunGame`.
- Flags in M0:
  - `--version` prints `pico-editor <version> (<commit>, <date>)` and exits.
  - `--log-level debug|info|warn|error` defaults to `info`.
  - `--log-file PATH` overrides the default log path.
- The game loop has no UI yet: clear the screen to palette index 0 and present.
- Logical canvas is **256×256** with a 4×6 glyph cell. The layout grid is
  **64 columns × 42 rows**. This is a locked decision for every later
  milestone.
- Default window scale is 4× (1024×1024 device-independent pixels). A scale of
  `0` means automatic: choose the largest integer scale that fits 80% of the
  current monitor, clamped to 2–8.
- Window resizing is enabled. Every frame, compute
  `scale = max(1, min(width/256, height/256))`, center the 256×256 canvas, and
  use nearest-neighbor sampling. Letterbox with palette index 0. Never use
  linear filtering.
- `Ctrl+Q` quits. Closing the window also quits. M0 has no unsaved state, so no
  confirmation is required.

### 3. `docs/ARCHITECTURE.md`

The document is normative for later milestones. It must contain:

1. Package graph and one-line responsibility for each package.
2. The single-writer UI rule: all state mutation happens in `Update`; background
   results arrive as messages and are applied there.
3. Coordinate systems:
   - file byte offsets,
   - rune offsets within a line,
   - P8SCII glyph clusters,
   - display cells,
   - LSP UTF-16 positions.
4. Document model: `.p8` buffers hold only the `__lua__` code text; the original
   cart bytes live beside the buffer for byte-preserving saves. `.lua` buffers
   hold the whole file.
5. Cart tab model: one code buffer per cart, tabs are line ranges, and view
   state is stored per tab.
6. Render pipeline: software `Surface` (256×256 palette indices) uploaded to one
   Ebitengine image, scaled with nearest-neighbor. This keeps golden tests
   headless and pixel-exact.
7. LSP transport: pico-lsp runs in-process over `net.Pipe` using
   `go.lsp.dev/jsonrpc2` streams with `go.lsp.dev/protocol` types; the UI never
   blocks on a request.
8. Input flow: Ebitengine keys plus `AppendInputChars`, with Vim and P8SCII
   input layered above the raw event stream.
9. Config, keymap, and command registry ownership.
10. Error, logging, recovery, and shutdown policy.
11. Testing strategy and CI gates.
12. Dependency pins and the pico-lsp public-package schedule.

### 4. `docs/VIM.md`

- Copy the command matrix from M3 §2 without omissions.
- State the out-of-scope list explicitly: macros, marks, plugins, autocmds,
  splits/windows, `:set`, `:g`, `:v`, `:normal`, `:terminal`, and scripting.
- Define key notation used by the matrix and by `config.toml`: `ctrl+`,
  `alt+`, `shift+`, `meta+`, then a key name such as `a`, `enter`, `esc`,
  `space`, `tab`, `left`, `f1`.
- M0 does not implement the matrix; it only freezes it.

### 5. mise tasks

`mise.toml` defines exactly these public tasks:

| Task | Behavior |
| --- | --- |
| `build` | `go build -trimpath -o bin/pico-editor ./cmd/pico-editor` |
| `run` | `go run ./cmd/pico-editor` |
| `test` | `go test ./...` |
| `lint` | `golangci-lint run` |
| `fmt` | `gofmt -w .` and `go mod tidy` |
| `check` | `fmt-check`, `lint`, `test`, and `build` |

- `fmt-check` fails when `gofmt -l .` prints anything or when `go mod tidy`
  would change `go.mod`/`go.sum`. It must not rewrite files in CI.
- `check` is the single gate used locally, in CI, and before every milestone
  exit.
- Pin `golangci-lint` in `mise.toml`; do not rely on a globally installed
  version.

### 6. CI

- GitHub Actions workflow triggers on `pull_request` and pushes to `main`.
- Use `jdx/mise-action` (or the maintained equivalent) so the toolchain comes
  from `mise.toml`; do not duplicate the Go version in the workflow.
- Jobs run on `ubuntu-latest` and execute `mise run check`.
- Set `GOFLAGS=-mod=readonly` and enable Go module caching.
- Ebitengine `v2.10.6` builds with `CGO_ENABLED=0`; do not add X11/GL headers to
  the M0 CI image. M1 adds a separate `xvfb` smoke job when a window is actually
  opened.
- CI must be green before a PR is merged. After opening a PR, run the
  `code-review` skill and fix its findings on the same PR, per `AGENTS.md`.

### 7. Logging, errors, and version metadata

- Use `log/slog` with a text handler.
- Default log path: `$XDG_STATE_HOME/pico-editor/pico-editor.log`
  (`~/.local/state/pico-editor/...` when unset). Create directories with `0700`
  and the log with `0600`.
- Never log file contents. Log paths, operation names, and durations.
- Fatal startup errors go to stderr and the log, then exit non-zero.
- Ebitengine runs frame callbacks on its game goroutine, outside `main`'s
  recover. Recover panics inside `Update`, `Draw`, `DrawFinalScreen`, and
  `Layout`, log the stack, and return the fatal error from `Update` so
  `RunGame` exits non-zero. Keep a top-level recover for startup panics. Later
  milestones may recover in command handlers where continuing is safe.
- `internal/version` values are injected with
  `-ldflags "-X ...Version=... -X ...Commit=... -X ...BuildDate=..."`.
  Development builds report `dev` and `unknown`.

### 8. License and notices

- `LICENSE` is the standard MIT text with the project's copyright holder.
- `NOTICE` records:
  - PICO-8's font is CC-0; the derived 1-bit atlas will be attributed there.
  - PICO-8 is a trademark of Lexaloffle Games. This project is not affiliated
    with or endorsed by Lexaloffle.
  - No Rune code is copied; Rune is reference material only.
- README repeats the trademark disclaimer in its first screen.

### 9. Dependency plan and pin policy

- Until the pico-lsp public packages exist, the editor must not import
  `github.com/asimmons91/pico-lsp/internal/...`; Go forbids it and the repo
  rules forbid copying the code.
- During development:
  `replace github.com/asimmons91/pico-lsp => ../pico-lsp`.
- Before M1 starts: pico-lsp publishes `pkg/p8scii` in `v1.6.0` and the editor
  pins that release. The temporary `replace` is removed.
- Before M4 starts: `pkg/cart` in pico-lsp `v1.7.0` exposes byte-preserving
  save/splice; the editor pins it.
- Before M5 starts: pico-lsp `v1.8.0` publishes `pkg/lsp` and `pkg/glyphs`; the
  editor pins that release.
- A milestone cannot exit on a pseudo-version or a branch. Only a tagged
  release counts.

## Architecture and files

```text
cmd/pico-editor/main.go          flags, logging, RunGame
internal/app/app.go              Game struct, Update/Draw/Layout
internal/app/window.go           integer-scale and letterbox math
internal/version/version.go      ldflags-injected build metadata
docs/ARCHITECTURE.md             normative cross-cutting design
docs/VIM.md                      frozen V1 command matrix
mise.toml                        tool pins and tasks
.golangci.yml                    lint configuration
.github/workflows/ci.yml         mise run check
LICENSE
NOTICE
README.md
.gitignore
```

## Interfaces and state

```go
// internal/version
var (
    Version   = "dev"
    Commit    = "unknown"
    BuildDate = "unknown"
)

// internal/app
type Game struct{}

func (g *Game) Update() error
func (g *Game) Draw(screen *ebiten.Image)
func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) // 256, 256
```

`Game` owns no globals. Dependencies are passed through a small `Config` struct
so M1 can add input and rendering without rewiring `main`.

## Error handling

- Invalid flags print usage to stderr and exit 2.
- Missing log directory or unwritable log file prints a warning to stderr and
  continues with stderr logging.
- A panic during startup is logged with a stack trace and exits 1.
- A panic during a frame is logged and exits 1 in M0; later milestones recover
  in command handlers where continuing is safe. Ebitengine runs frame callbacks
  on its game goroutine, so M0 recovers inside `Update`, `Draw`,
  `DrawFinalScreen`, and `Layout` as well as keeping a top-level recover for
  startup panics.

## Testing

### Automated

- `go build ./...` and `go test ./...` pass with `CGO_ENABLED=0`.
- Window scale math is table-tested for common monitor sizes, including
  non-integer device scale factors.
- `--version` output is tested through `internal/version`.
- `mise run check` is the same command CI runs.

### Manual

- `mise run run` opens the window at 4×.
- Resize the window through several sizes; the canvas stays centered and sharp.
- Run under Wayland and X11 sessions; both must open. If one backend fails,
  record the limitation and the workaround in `docs/ARCHITECTURE.md`.

## Exit criteria

- [x] `mise install` installs Go 1.27.4 and the lint tool.
- [x] `mise run check` passes locally and in CI.
- [x] `mise run run` opens the empty 256×256 canvas at 4×.
- [ ] Resizing never blurs or crops the canvas.
- [x] `docs/ARCHITECTURE.md` and `docs/VIM.md` exist and are linked from the
      roadmap.
- [x] License, notices, and README disclaimer are present.
- [ ] The pico-lsp public-package schedule is recorded and accepted by the
      pico-lsp repository.
- [x] No pico-lsp `internal/` package is imported.

## Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Ebitengine API drift | Pin `v2.10.6`; isolate window and drawing behind `internal/app/window.go`. |
| Canvas decision is revisited late | Freeze 256×256 and 64×42 in `ARCHITECTURE.md`; later specs assume it. |
| CI toolchain drift | `mise.toml` is the only version source; CI uses mise, not `setup-go` defaults. |
| pico-lsp public packages slip | Treat the upstream release as a milestone dependency with a written owner and date; do not start M1 on an unreleased API. |
| License ambiguity | MIT is adopted now; `NOTICE` records the CC-0 font and the PICO-8 trademark. |
