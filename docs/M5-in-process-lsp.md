# M5 — In-process pico-lsp

**Status:** Implemented
**Depends on:** M2, M4; pico-lsp release with public `pkg/lsp`
**Blocks:** M6, M7
**Roadmap:** [ROADMAP.md](ROADMAP.md)
**Estimate:** 3–4 weeks (upstream `pkg/lsp` precedes this milestone)

## Resolved decisions

| Decision | Resolution |
| --- | --- |
| Upstream pin | pico-lsp `v1.6.0`; M17–M19 shipped together, so `pkg/p8scii`, `pkg/cart`, `pkg/lsp`, and `pkg/glyphs` are all in the existing pin. The previously planned `v1.8.0` tag was folded into `v1.6.0`. |
| Semantic colors | The M7 final palette table (keyword 14, string 11, number 10, comment 13, operator 6, variable 7, parameter/property 15, function 12, type 9, namespace 14, macro/decorator 9, regexp 11, modifier 13) is implemented now. M7 may refine the table without changing M5 behavior. |
| LSP failure UX | A persistent error-colored status message plus `:lsp restart`. No modal dialog; editing, saving, and Vim remain usable. M7 migrates this to notifications and the command palette. |
| Status bar | M5 appends compact `E/W`, `T/C/Z`, and `LSP` segments to the existing one-row status bar. M7 replaces the layout with the segmented bar. |
| Integration fixtures | A small subset of pico-lsp `v1.6.0` testdata is copied under `internal/lsp/testdata` with provenance. Tests do not depend on the module cache path. |
| Completion key ownership | `Ctrl+Space`, `Ctrl+N`, `Ctrl+Shift+N`, `Enter`, `Tab`, and the popup-first `Esc` are handled by the app before Vim. `Ctrl+P` is never intercepted. |

## Outcome

pico-lsp runs inside the editor process over an in-memory JSON-RPC connection.
Real carts and includes get semantic highlighting, diagnostics, completion,
hover, signature help, go-to-definition, and cart size information. There is no
subprocess, no stdout protocol, and no blocking of the UI thread.

**Demo:** Open a cart with an error. Keywords, strings, numbers, comments, and
symbols are colored from semantic tokens; the error is underlined and counted
in the status bar. Trigger completion and accept an item; hover a function;
open signature help; jump to a definition in an included `.lua`; show token,
character, and compressed sizes in the status bar.

## Scope

### In scope

- Tagged pico-lsp `pkg/lsp` facade and `pkg/glyphs` (upstream prerequisite).
- In-process JSON-RPC over `net.Pipe`.
- Initialize, capabilities, workspace folders, configuration, and document sync.
- Diagnostics, semantic tokens, completion, hover, signature help, and
  definition.
- `pico8/cartInfo` and `pico8/glyphs` requests.
- Mapping between full-cart, code-tab, and included-file coordinates.
- Popup UI, highlighting, status, and stale-response handling.
- Integration tests against pico-lsp testdata.

### Out of scope

- References, rename, formatting, code actions, inlay hints, folding, and code
  lens (post-V1 unless they fall out for free).
- `.p8.png` decoding and virtual documents (post-V1).
- Workspace symbols, semantic token ranges, and pull diagnostics.
- Multi-root workspaces.
- Snippet expansion: advertise no snippet support and insert plain text.
- Markdown rendering: render plain text only.

## Deliverables

1. Tagged pico-lsp `pkg/lsp` and `pkg/glyphs` with a minimal in-process server
   constructor and the public glyph catalog; the editor consumes them.
2. `internal/lsp` client, transport, document sync, caches, and request
   wrappers.
3. Semantic token decoding and palette mapping.
4. Diagnostics cache, underlines, status counts, and `:diag` list.
5. Completion, hover, signature help, and definition UI.
6. Cart info status and glyph catalog request (picker UI lands in M6).
7. Integration tests over `net.Pipe`, including cancellation, timeout, crash,
   and restart.

## Detailed requirements

### 1. Upstream `pkg/lsp` (prerequisite)

The editor consumes a tagged pico-lsp release. The facade wraps `internal/lsp`
without exposing internal types:

```go
package lsp

type Options struct {
    Logger *slog.Logger
}

type Server struct{ /* unexported */ }

func New(opts Options) *Server
func (s *Server) Serve(ctx context.Context, stream jsonrpc2.Stream) jsonrpc2.Conn
```

- `Serve` uses the existing internal server and its `go.lsp.dev/jsonrpc2`
  stream. It must not write to stdout or open a socket.
- Settings are LSP-native. The editor sends `initializationOptions.pico8` and
  answers `workspace/configuration`; the default values match pico-lsp's
  current defaults. Invalid values fall back to defaults with a log entry.
- Workspace folders are part of the `initialize` params, not `Options`.
- `ServerInfo.Version` comes from pico-lsp; `Options` has no version field.
- Shutdown is caller-owned: the editor sends `shutdown`, then closes the
  returned connection and cancels the server context. There is no
  `Server.Shutdown` method.
- `pkg/glyphs` exposes `Builtins`, `Build`, `FindByName`, and `FindByByte` for
  direct catalog use. The M6 picker continues to use the `pico8/glyphs` LSP
  request; both paths read the same upstream catalog.
- Public API tests live in pico-lsp; the editor's integration tests are not a
  substitute.
- pico-lsp ships the public packages in `v1.6.0`; the editor pins that release
  and imports only public packages. No local `replace` remains.

### 2. Transport and lifecycle

```go
type Client struct {
    conn     jsonrpc2.Conn
    server   protocol.Server
    ...
}

func Start(ctx context.Context, opts Options) (*Client, error)
func (c *Client) Stop(ctx context.Context) error
```

- Create `net.Pipe()`. Wrap each end with `jsonrpc2.NewStream`; call
  `serverConn := srv.Serve(ctx, serverStream)` and keep the returned `Conn`.
- Create the editor side with
  `ctx, conn, server := protocol.NewClient(ctx, handler, jsonrpc2.NewStream(clientPipe))`.
  `protocol.NewClient` starts the connection and calls `Go`; do not call `Go`
  again. Keep `conn` for lifecycle/close and `server` for outgoing requests
  (`Initialize`, `Completion`, and so on).
- `Start` sends `initialize` with:
  - `processId`,
  - `clientInfo{name: "pico-editor", version}`,
  - `rootUri` and `workspaceFolders`,
  - `capabilities` limited to what the editor implements,
  - `initializationOptions` with the public settings.
- Then send `initialized` and open already-open documents.
- `Stop` sends `shutdown`, waits up to 2s, sends `exit`, calls
  `serverConn.Close()`, cancels the server context, closes both pipes, and
  waits for `serverConn.Done()` and the client dispatcher with the timeout. If
  the timeout expires, log and proceed.
- Restart creates a new `lsp.Server`; a server instance serves one connection.
  Stop, clear caches, start, and re-open documents. The editor remains usable
  if restart fails.
- A server crash/EOF marks LSP `failed`, shows a notification, and leaves
  editing, saving, and Vim fully functional.

### 3. Client capabilities

Advertise only implemented features:

- `workspace.workspaceFolders`: supported and change notifications.
- `workspace.configuration`: true; answer `workspace/configuration` with the
  current settings section.
- `textDocument.synchronization`: full text changes (the client sends full
  text), open/close/save.
- `textDocument.publishDiagnostics`: related information and tags.
- `textDocument.semanticTokens`: full only; requests and refresh support.
- `textDocument.completion`: completion item `documentation` in plain text,
  `snippetSupport: false`, `insertReplaceSupport: true`.
- `textDocument.hover`: plain text content only.
- `textDocument.signatureHelp`.
- `textDocument.definition`: link support false; plain locations.
- `general.positionEncodings`: **UTF-16 only**. Do not advertise UTF-8; the
  editor has one position mapping.
- Do not advertise references, rename, formatting, code actions, inlay hints,
  folding, code lens, workspace symbols, or pull diagnostics.

Full-text sync is deliberate: carts are bounded at 64KB, and it eliminates
incremental range bugs. If profiling shows a problem, incremental sync becomes
a post-V1 change with its own tests.

### 4. Document synchronization

- `.p8`: send `cart.Document.FullText()` — the reconstructed header plus current
  code plus suffix. If the splice returns an error, keep the last valid text
  and show a notification; do not send partial cart bytes. URI is the cart's
  `file://` URI; language ID is `pico8`.
- `.lua`: send the full buffer; language ID is `lua`.
- `didOpen` on buffer open; `didChange` after a 120ms edit debounce with the
  full text and a monotonically increasing version; `didSave` immediately with
  no text (the server already has it); `didClose` on buffer close.
- Coalesce rapid edits: keep only the latest text per buffer. Never send a
  version older than the last accepted one.
- On save, send `didSave` then request cart info.
- Included `.lua` files are opened/synced like any other document. The cart
  document is the root; includes are separate URIs.
- On external reload, send `didClose` + `didOpen` with the new text and version.
- If the server returns an error for an unknown document, log and re-open it
  once before marking LSP failed.

### 5. Coordinate mapping

There are four coordinate spaces:

1. Full-cart text (LSP UTF-16 line/character).
2. Code body (editor rune positions).
3. Active tab display rows (code body minus tab start).
4. Included `.lua` file (its own buffer).

Rules:

- `fullLine = cart.BaseLine + codeLine`.
- `displayLine = codeLine - activeTab.LocalStart`; the `-->8` line is hidden.
- `editor.Pos` ↔ LSP position uses M2's `UTF16` helpers, not string length.
- Semantic tokens and diagnostics that fall outside the active tab are cached
  but not drawn; switching tabs re-renders from the cache.
- Diagnostics in the header or non-code sections are logged and dropped; they
  are not possible for a valid cart.
- Definition targets map back through the cart when the target is another tab;
  otherwise open/switch the `.lua` buffer and place the cursor.
- All mapping helpers are centralized in `internal/lsp/positions.go` with
  round-trip tests.

### 6. Semantic tokens

- Read the token legend from `initialize`; do not hard-code token type indexes.
- Request `textDocument/semanticTokens/full` after:
  - `didOpen`,
  - the debounce after `didChange`,
  - `workspace/semanticTokens/refresh` from the server.
- Decode the five-integer delta format into `Token{Line, StartChar, Length,
  Type, Modifiers}`.
- Convert UTF-16 columns/lengths to editor rune ranges; a token that splits a
  P8SCII cluster is expanded to the cluster boundary and logged.
- Render tokens in this priority order: comment, string, number, keyword,
  type/class, function, property, variable, operator, parameter, namespace.
  Later tokens do not overwrite an earlier higher-priority token on the same
  cell.
- Use the M7 palette mapping; M5 may start with a provisional table from
  `internal/render`. The final table is frozen in M7.
- Cache tokens by document version; drop responses whose version is stale.
- Tokens for the whole cart are cached; only the active tab is drawn.

### 7. Diagnostics

- Handle `textDocument/publishDiagnostics`: store by URI with the version, drop
  older versions, and clear when an empty list arrives.
- Render:
  - error: 1px underline in palette 8,
  - warning: 1px underline in palette 9,
  - information/hint: status counts only.
- Status bar shows `E<n> W<n>` for the active document.
- `:diag` opens a list popup of the active document's diagnostics with
  severity, line, and message. `Enter` jumps to the location; `Esc` closes.
- Do not publish diagnostics to the server; this is a client-only feature.
- Diagnostics for included files appear when that buffer is active and are
  counted in its status.

### 8. Completion

- Trigger:
  - manual `Ctrl+Space`,
  - automatic after an identifier character, `.`, `:`, or `(`, debounced
    100ms,
  - only in insert mode and only when LSP is ready.
- Request `textDocument/completion` with the current position and a small
  context (`triggerKind`, trigger character).
- Render at most 10 items in a popup anchored below the cursor, clipped to the
  canvas. Show label and detail; documentation in a side/below panel when it
  fits.
- Navigation: `Ctrl+N` next, `Ctrl+Shift+N` previous, `Enter`/`Tab` accept,
  `Esc` close. **Do not bind `Ctrl+P`**; insert-mode `Ctrl+P` belongs to P8SCII
  puny mode.
- Accepting:
  - use `textEdit` when present (both insert and replace ranges),
  - otherwise insert `insertText` or `label` at the cursor,
  - apply `additionalTextEdits` in descending position order,
  - never apply overlapping edits; if they overlap, insert the main item only
    and notify.
- No snippet parsing: `$1`, `${1:name}`, and placeholders are inserted
  literally only if the server sends them; the editor advertises no snippet
  support so this should not happen.
- `p8_left` completion items are ordinary completion items; the editor does not
  special-case them beyond inserting canonical Unicode text.
- Re-request when the cursor moves or the prefix changes while the popup is
  open. Cancel the previous request.

### 9. Hover

- `K` in normal mode requests hover at the cursor.
- Also support `Ctrl+Shift+K`? No; keep one binding.
- Render plain text, wrapped to 40 columns, max 12 rows, with a border.
- Markdown fences and emphasis markers are stripped; links show their text.
- `K`, `Esc`, or any motion closes the popup.
- If the response is empty, show nothing and bell.

### 10. Signature help

- Request automatically in insert mode when the cursor is inside `(` or after a
  `,`; also on `Ctrl+Space` when completion has no items.
- Render up to 3 signatures, max 4 rows, with the active signature and active
  parameter highlighted (inverse video).
- Update as the cursor moves; cancel stale requests.
- `Esc` closes it before closing completion.
- No parameter cycling in V1; the server decides the active parameter.

### 11. Go-to-definition

- Bindings: `gd` and `Ctrl+]` in normal mode. M5 adds them to `docs/VIM.md`.
- Request `textDocument/definition` at the cursor.
- Results:
  - same buffer: move the cursor and push the jump list,
  - another cart tab: switch tabs, move, push the jump list,
  - `.lua` file: open/switch the buffer, move, push the jump list,
  - URI outside the workspace: refuse with a notification.
- `Ctrl+O`/`Ctrl+I` traverse the jump list (M3).
- If no definition, bell and do not move.

### 12. Cart information

- Request `pico8/cartInfo` after save, after a 300ms edit debounce, and when the
  active buffer changes.
- Cache by cart URI and document version.
- Status bar shows:
  - `T<n>/8192` tokens,
  - `C<n>/65535` chars,
  - compressed bytes with `ok`/`warning`/`error` color,
  - `n/a` for standalone `.lua`.
- Values come from the server; do not recompute sizes in the editor.
- On error `-32001 cart-not-found`, show `n/a` and log at debug.
- M7 refines the compact status layout; M5 only needs the values.

### 13. Glyph catalog request

- Expose `RequestGlyphs(ctx)` that calls `pico8/glyphs` and returns the catalog.
- M5 tests the request and caches the result per settings version.
- M6 builds the picker UI on top; no picker in M5.

### 14. LSP status and errors

- Status values: `off`, `starting`, `ready`, `failed`.
- On `failed`, show a persistent notification and expose
  `LSP: Restart` in the command palette and `:lsp restart`.
- Log protocol errors at `warn`, server logs at `info`, and request durations at
  `debug`.
- Timeouts: 2s for completion/hover/signature, 5s for definition/cart info/
  glyphs. Cancel the context on close and ignore late responses.
- Never block `Update` on a request. Results are messages applied on the UI
  thread.
- Never write LSP logs to stdout.

## Architecture and files

```text
internal/lsp/
  client.go        start/stop, request plumbing, status
  handler.go       protocol.Client implementation for server notifications
  documents.go     open buffers -> didOpen/didChange/didSave/didClose
  positions.go     cart/tab/file coordinate mapping
  tokens.go        semantic token decode, cache, ranges
  diagnostics.go   cache, rendering ranges, :diag list
  completion.go    request, popup state, insertion
  hover.go         request, wrapping, popup
  signature.go     request and popup
  definition.go    request, jump handling
  cartinfo.go      request and status cache
  glyphs.go        catalog request
internal/app/
  lsp_ui.go        popups, status, notifications
  lsp_commands.go  palette/ex commands
```

## Interfaces and state

```go
type Settings struct {
    TypeChecking string
    InlayHints   bool
    MaxProblems  int
}

type Options struct {
    WorkspaceRoot string
    Settings      Settings
    Log           *slog.Logger
    Version       string
}

type Service struct {
    Client   *Client
    Status   Status
    Diags    map[uri.URI][]Diagnostic
    Tokens   map[uri.URI]TokenSet
    CartInfo map[uri.URI]CartInfo
}

func New(opts Options) *Service
func (s *Service) Start(ctx context.Context) error
func (s *Service) Stop(ctx context.Context) error
func (s *Service) DidChange(buf BufferRef)
func (s *Service) Complete(ctx context.Context, pos editor.Pos) ([]CompletionItem, error)
```

All caches are keyed by URI and document version. Stale entries are replaced,
not merged.

## Error handling

- A failed initialize leaves the editor usable with LSP `failed`.
- A request error returns to the caller; UI commands show a one-line
  notification at info level, not a modal.
- A malformed server notification is logged and dropped; it never panics the
  UI.
- A panic in a handler is recovered by pico-lsp; the client also guards its
  own message loop.
- Closing a document while a request is in flight cancels the request and
  ignores the result.
- If `net.Pipe` closes unexpectedly, stop the client goroutine and mark failed.

## Testing

### Integration over `net.Pipe`

- Start the real pico-lsp server from the pinned release.
- Fixtures from pico-lsp `testdata`:
  - a cart with keywords, strings, numbers, comments, operators, and symbols,
  - a cart with a type error, an undefined global, and an unused local,
  - a cart with an included `.lua`,
  - a cart with multiple tabs.
- Assertions:
  - semantic token types cover each expected category and positions map to the
    right tab and cell,
  - diagnostics arrive with the right severity and version,
  - completion returns items for a prefix and `p8_left`,
  - hover and signature return non-empty content,
  - definition jumps to the expected URI/tab/line,
  - cart info matches the fixture's expected counts.

### Concurrency and failure

- `-race` with concurrent edits and requests.
- Stale token/diagnostic responses are ignored.
- Completion cancellation when the cursor moves.
- Server panic recovery and restart.
- Pipe close while a request is pending.
- 2s/5s timeout paths using a fake slow server.

### Manual

- Edit a cart continuously for five minutes; no stale highlighting or missed
  diagnostics.
- Open ten included files; definition and hover remain responsive.
- Kill the server goroutine during a request; the editor shows `failed` and
  keeps editing.

## Exit criteria

- [x] pico-lsp tags `pkg/lsp` and `pkg/glyphs`; editor pins the release.
- [x] Semantic highlighting covers keywords, strings, numbers, comments,
      operators, and symbol kinds on real carts.
- [x] Diagnostics and completion update after edits without deadlocks or stale
      state.
- [x] Hover, signature help, and definition work in `.p8` code and included
      `.lua` files.
- [x] Cart token/character/compressed status matches the server.
- [x] LSP failure never blocks editing, saving, or quitting.
- [x] `Ctrl+P` remains reserved for P8SCII input; completion uses
      `Ctrl+N`/`Ctrl+Shift+N`.
- [x] `go test -race ./...` and `mise run check` pass.

## Implementation notes

- `internal/lsp` owns transport, document sync, caches, timeouts, and request
  wrappers. A single background writer serializes document notifications; all
  feature requests run asynchronously and post events that `Update` applies.
- Full text is rebuilt through `pkg/cart.ReplaceCode` for `.p8` documents. If
  the splice fails, the last valid text remains on the server and the status
  row reports the problem. Saves flush a pending debounced change before
  `didSave`, and semantic tokens are re-requested after the edit debounce.
- Semantic tokens and diagnostics store cluster-expanded rune ranges. The
  active cart tab draws only its own range; other tabs stay cached.
- `internal/app/lsp_session.go` owns document sync, popup state, jump handling,
  and restart; `internal/app/lsp_ui.go` owns completion, hover, signature,
  diagnostics-list drawing, and status segments.
- The copied fixtures live in `internal/lsp/testdata` with a provenance README.
  The results and remaining manual checklist are in
  [spikes/M5-lsp.md](spikes/M5-lsp.md).

## Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| pico-lsp public API destabilizes | Thin facade, public API tests upstream, pinned tag, no internal imports. |
| Coordinate mapping is wrong | One positions module with round-trip tests across cart tabs and includes. |
| Full-text sync is slow | 64KB cap, 120ms debounce, benchmarks; incremental sync is a post-V1 option. |
| Stale responses overwrite new state | Version-keyed caches and cancellation; drop older versions. |
| UI blocks on a request | All requests async with contexts; results applied in `Update`. |
| LSP crash kills the editor | Separate goroutine, status `failed`, restart command, editor remains usable. |
| `Ctrl+P` collision | Reserve it for P8SCII; completion uses `Ctrl+N`/`Ctrl+Shift+N`. |
| Semantic tokens split clusters | Expand to cluster boundaries and log; test all VS16 glyphs. |
