# M4 — Cart model and file tree

**Status:** Draft
**Depends on:** M2; pico-lsp release with public `pkg/cart`; M3 may overlap
**Blocks:** M5
**Roadmap:** [ROADMAP.md](ROADMAP.md)
**Estimate:** 2–3 weeks (upstream `pkg/cart` precedes this milestone)

## Outcome

The editor opens real `.p8` carts as a code-only view with `-->8` tabs, edits
only the `__lua__` section, and saves without touching the header, graphics,
sound, map, or any other section. It also browses a project folder, opens
`.p8` and `.lua` files, and associates includes with their owning cart.

**Demo:** Open a cart from the file tree, switch code tabs, edit a line, save,
and show `git diff` proving only `__lua__` changed. Open an included `.lua`,
edit it, and save. Force an external change and confirm the conflict dialog and
backup behavior.

## Scope

### In scope

- Tagged pico-lsp `pkg/cart` with a byte-preserving save/splice API (upstream
  prerequisite).
- `.p8` parsing, code tabs, code-only view, and save reconstruction.
- Per-tab cursor and scroll state.
- Atomic writes, read-only handling, external-change detection, and backups.
- Workspace root, recursive file tree, file tabs, and buffer management.
- Include-to-cart association using `pkg/cart`.
- Golden round-trip tests on real carts.

### Out of scope

- `.p8.png` reading or writing (post-V1).
- Editing any non-`__lua__` section.
- Sprite/map/sound/music editors.
- Multi-root workspaces and remote files.
- File watching (M7 provides refresh-on-focus; full watching is post-V1).
- LSP cart info and diagnostics (M5).
- Save-as, rename, delete, and new-file creation in the tree (post-V1 unless
  needed for M4 tests).

## Deliverables

1. Tagged pico-lsp `pkg/cart` with `Parse`, `Document`, `Tabs`, `Code`,
   `ReplaceCode`, `Includes`, and an include resolver facade; the editor
   consumes it.
2. `internal/cart` adapter that holds the original bytes and the editable code
   separately.
3. Code-only cart view with a tab bar and per-tab view state.
4. Save pipeline with byte preservation, atomic writes, conflict detection, and
   backups.
5. `internal/workspace` folder scan, include association, and refresh.
6. File tree overlay, file tabs, and buffer lifecycle.
7. Golden tests, failure-injection tests, and a manual cart checklist.

## Detailed requirements

### 1. Upstream `pkg/cart` (prerequisite)

The editor consumes a tagged pico-lsp release. pico-lsp already parses carts in
`internal/cart`, but it has no save path. The upstream prerequisite adds both
the public facade and a new, tested splice API.

Minimum public surface:

```go
package cart

type Tab struct {
    Index      int    // zero-based, matching pico8/cartInfo and PICO-8
    Name       string // "Tab 1" when PICO-8 stores no name
    StartLine  int    // first code line, full-cart, zero-based
    EndLine    int    // exclusive
    MarkerLine int    // hidden -->8 line, or -1 for the first tab
}

type Document struct {
    // unexported original bytes and section offsets
}

func Parse(data []byte) (*Document, error)
func (d *Document) Code() []byte
func (d *Document) Tabs() []Tab
func (d *Document) Includes() []Include
func (d *Document) ReplaceCode(code []byte) []byte
func (d *Document) CodeStartLine() int
```

Requirements:

- `Parse` accepts a `.p8` with a header, an optional `__lua__` section, and any
  suffix sections. It must reject malformed section markers with a typed error
  that includes the line number.
- `Code()` returns exactly the bytes between the `__lua__` marker line and the
  next section marker (or EOF), without the marker line.
- `ReplaceCode` returns a new byte slice with only that range replaced. Every
  byte before and after the range is preserved, including spacing, line
  endings, and section order.
- `ReplaceCode` handles:
  - an empty code section,
  - a code section with no trailing newline before a following section,
  - code ending with or without a newline,
  - a cart with no following section.
- `Tabs()` splits the code body on lines that start with `-->8` and returns
  full-cart line numbers. `Index` is zero-based (normalize the existing internal
  1-based value), `StartLine` is the first content line, and `MarkerLine`
  identifies the hidden separator (-1 for the first tab).
- `Includes()` exposes include paths already resolved by the existing cart
  loader, including tab selectors and quoted paths.
- The include resolver facade must be public so the editor can map a `.lua`
  file to its nearest owning cart without reimplementing include rules.
- Upstream tests must cover byte-identical `Parse`→`ReplaceCode` round-trips for
  every cart in pico-lsp's corpus, plus edits inside the code section.
- pico-lsp tags the release (target `v1.7.0`) before M4 starts. The editor
  removes the local `replace` and pins the tag.

### 2. `internal/cart` adapter

```go
type Document struct {
    Path     string
    Raw      []byte      // original file bytes
    Parsed   *cart.Document
    Code     *editor.Buffer // code section only
    Tabs     []cart.Tab
    BaseLine int         // full-cart line of code line 0
}

func Open(path string) (*Document, error)
func (d *Document) FullText() string
func (d *Document) Save() error
```

- The editable buffer contains **only** the `__lua__` code body. Header and
  non-code sections never enter the buffer, so they cannot be edited or
  re-encoded.
- `BaseLine` is the full-cart line of code line 0. It is constant for the
  lifetime of the document because the header is not editable.
- `FullText` = `Parsed.ReplaceCode([]byte(Code.Text()))`, converted to a string.
  It is the text sent to pico-lsp and used for diagnostics mapping.
- `Save` = `ReplaceCode` on the current code text, then the atomic writer from
  M2. The original `Raw` bytes are not mutated in place.
- The code buffer's newline style is detected from the code section. `.p8`
  files are normally LF; a CRLF code section is preserved.
- A cart with no `__lua__` section opens read-only with a clear error; do not
  create a section implicitly.
- An empty `__lua__` section opens as one empty tab.

### 3. Code tabs

- Parse tabs from the code body; tab index 0 is the first tab, even when the
  body starts with `-->8`.
- Tab bar row (see layout below) shows `0`, `1`, … plus a dirty marker on the
  active tab. Tab names are shown only if pico-lsp returns a name.
- Only the active tab is drawn. Separator lines are hidden.
- Convert public full-cart tab lines to code-body lines once:
  `localStart = tab.StartLine - BaseLine`, `localEnd = tab.EndLine - BaseLine`.
- Display mapping:
  - `displayLine = codeLine - localStart`,
  - the `-->8` line itself is not drawn,
  - a tab with zero code lines shows one empty row with the cursor.
- Editing is confined to the active tab:
  - cursor motion clamps to the tab's line range,
  - `o`/`O` stay inside the tab,
  - `J` at the last line of a tab is a no-op,
  - deleting the last line of a tab is allowed; the separator remains,
  - paste never moves text across a tab boundary implicitly.
- Per-tab view state is stored in `TabState{Cursor, DesiredCell, TopLine,
  LeftCell}` and restored on switch.
- Switching: `Ctrl+Tab` next, `Ctrl+Shift+Tab` previous, click the tab bar, or
  `:tabn`/`:tabp`.
- Tab switch is not a buffer switch; the cart document stays open and LSP sees
  the same full text.

### 4. Layout with carts

The 64×42 grid is fixed:

| Rows | Content |
| --- | --- |
| 0 | File tabs (open buffers) |
| 1 | Cart code tabs (only for `.p8`) |
| 2–40 | Code area for `.p8` (39 rows) |
| 1–40 | Code area for `.lua` (40 rows) |
| 41 | Status/command line |

- The cart tab row is hidden for `.lua` buffers.
- Long file-tab and cart-tab strips scroll horizontally to keep the active tab
  visible.
- The status bar shows `file:tab` for carts and `file` for `.lua`.
- The file tree overlay covers columns 0–19 and rows 0–40 when open; it does
  not resize the code area.

### 5. Save pipeline

- Save triggers: `:w`, `:wq`, `Ctrl+S` (global command), and save-on-quit when
  the user chooses Save.
- Before writing:
  - stat the target; compare `mtime`, size, and a SHA-256 of the current bytes
    with the values recorded at open/last save,
  - if changed, show a conflict dialog: **Overwrite**, **Reload**, **Cancel**.
- Overwrite on conflict:
  - write a backup first: `<name>.p8.bak`, or `<name>.p8.bak.N` for the first
    free `N` in 1–9,
  - then atomically replace the cart.
- Reload discards the buffer and re-parses from disk after confirmation.
- Cancel leaves the buffer dirty and does not write.
- Read-only files: show an error dialog with the path and OS error; keep the
  buffer dirty.
- If the file was deleted externally, offer **Recreate** or **Cancel**.
- Save failure (disk full, permission, rename failure) leaves the original
  intact, keeps the buffer dirty, and logs the error.
- Symlinks: resolve to the target for the atomic replace, but keep the symlink
  in place. Refuse targets outside the workspace root unless the user confirms.
- Save success updates `SavedVersion`, conflict metadata, and the cart-info
  request (M5).

### 6. Workspace and file tree

```go
type Workspace struct {
    Root    string
    Entries []Entry
    Carts   map[string]*CartRef // include path -> owning cart
}

func Open(root string) (*Workspace, error)
func (w *Workspace) Refresh() error
func (w *Workspace) ResolveInclude(path string) (*CartRef, bool)
```

- CLI:
  - `pico-editor <file>` opens the file and uses its parent as the workspace,
  - `pico-editor <dir>` opens the folder with an empty buffer,
  - no argument opens the current directory.
- Scan rules:
  - include `.p8` and `.lua` files,
  - directories are listed but only matching files are shown,
  - skip hidden entries (leading `.`), `.git`, `node_modules`, `vendor`,
    `bin`, and `dist` by default; M7 makes this configurable,
  - follow file symlinks; do not recurse directory symlinks; detect loops,
  - cap at 20,000 entries; show a notification and stop scanning.
- Sort directories first, then case-insensitive name.
- File tree UI:
  - `Ctrl+Shift+B` toggles and focuses the tree; `Esc` returns to the editor,
  - `j`/`k` or arrows move, `Enter` opens, `h`/`l` collapse/expand,
  - `r` rescans, `/` filters by substring, `q` closes the tree,
  - active buffer and dirty buffers are marked,
  - opening a file already in a buffer switches to it; it does not reload.
- File tabs:
  - one tab per open buffer, active highlighted, dirty marker,
  - `:bn`/`:bp`/`:b N`/`:ls` operate on file buffers,
  - closing a dirty buffer requires `:q!` or the quit dialog.
- Quit with dirty buffers: modal **Save all**, **Discard all**, **Cancel**.
  There is no partial-save mode in V1.

### 7. Include association

- Use the public `pkg/cart` resolver; do not reimplement `#include` parsing.
- Build `Workspace.Carts` by parsing every `.p8` during scan.
- For a `.lua` file:
  - resolve the nearest cart that includes it (directory distance first, then
    lexicographic path),
  - if none, treat it as a standalone `.lua`.
- The status bar shows the owning cart name for included `.lua` files.
- M5 confirms the association with `pico8/cartInfo`; if the server disagrees,
  the server wins and logs a warning.
- Cart tab selectors in includes (for example `cart.p8:2`) are handled by
  `pkg/cart`; the editor only forwards paths.

### 8. External changes

- M4 detects changes on save and when the app regains focus.
- On focus, stat every open buffer and the workspace root.
- If a file changed and the buffer is clean, reload it automatically and keep
  cursor/scroll when the line count allows.
- If the buffer is dirty, show the conflict dialog.
- Tree refresh on focus is automatic; a full file watcher is post-V1.

### 9. Error handling

- Parse failures show the path, line, and reason; the file is not opened.
- Save failures never truncate or partially replace the cart.
- A failed backup aborts the overwrite and reports the error.
- A tree scan permission error skips that directory and logs a warning.
- Unknown file types are ignored by the tree but can still be opened by path.

## Architecture and files

```text
internal/cart/
  document.go      adapter over pkg/cart
  tabs.go          tab ranges and view state
  save.go          conflict detection, backup, atomic write
internal/workspace/
  workspace.go     root, scan, entries
  include.go       resolver facade and association
  workspace_test.go
internal/app/
  cartview.go      code-only view and tab bar
  filetree.go      overlay tree
  filetabs.go      file buffer tabs
  buffers.go       open/close/switch lifecycle
```

## Interfaces and state

```go
type TabState struct {
    Cursor      editor.Pos
    DesiredCell int
    TopLine     int
    LeftCell    int
}

type CartView struct {
    Doc      *cart.Document
    Active   int
    Tabs     []TabState
    Dirty    bool
}
```

`Document.Raw` is immutable. Only `Code` changes. This is the invariant that
guarantees byte-preserving saves.

## Testing

### Golden carts

- Curate fixtures from pico-lsp `testdata/carts` plus hand-built edge cases:
  - header only and empty code,
  - multiple tabs, including a leading `-->8`,
  - every non-code section present,
  - no trailing newline,
  - CRLF code,
  - read-only file.
- Tests:
  - open→save with no edits is byte-identical,
  - edit one line→save changes bytes only inside `__lua__`,
  - tabs match the separator lines,
  - header and suffix hashes are unchanged,
  - `BaseLine` mapping is correct for LSP-style line numbers.
- Keep a test that hashes every non-code byte range before and after save.

### Failure injection

- Atomic write failures at create, write, sync, chmod, rename, and directory
  sync.
- Read-only target.
- External change between stat and rename.
- Backup name collisions (`.bak` through `.bak.9`).

### Workspace

- Scan nested directories, hidden files, symlink loops, and oversized trees.
- Include resolution for single and multiple carts, nested includes, and
  quoted paths.
- Unsaved-buffer prompts on close and quit.

### Manual

- Open three real carts from different PICO-8 versions; verify tabs, save, and
  `git diff`.
- Edit a cart while PICO-8 itself has the file open; verify conflict handling.
- Edit an included `.lua`; verify the owning cart is reported.

## Exit criteria

- [ ] pico-lsp tags `pkg/cart` with `ReplaceCode`; editor pins the release.
- [ ] Golden tests show only the `__lua__` body changing on save.
- [ ] Code tabs switch cleanly and preserve cursor/scroll state per tab.
- [ ] The file tree opens carts and includes without losing unsaved changes.
- [ ] Save failures surface a useful error and never corrupt the cart.
- [ ] External changes and overwrite backups behave as specified.
- [ ] No editor-local cart parser or P8SCII mapping exists.
- [ ] `mise run check` passes.

## Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Save corrupts non-code sections | Code-only buffer, `ReplaceCode` splice, byte-hash goldens, atomic writes. |
| pico-lsp `pkg/cart` slips | Treat the upstream release as a hard M4 dependency; do not start the adapter on `internal/` imports. |
| Tab boundaries drift after edits | Tabs are ranges over the code buffer; clamp editing to the active range; test insert/delete at boundaries. |
| External changes overwrite user data | mtime/size/hash check, conflict dialog, numbered backups, failure injection. |
| File tree scans explode | Ignore rules, entry cap, lazy refresh, permission-tolerant scan. |
| Include association is wrong | Use `pkg/cart` resolver; server `cartInfo` wins in M5; show owner in status. |
| Symlink writes escape the workspace | Resolve targets; require confirmation outside the root. |
