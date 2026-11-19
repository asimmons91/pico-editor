# M7 — Polish and hardening

**Status:** Draft
**Depends on:** M1–M6
**Blocks:** M8
**Roadmap:** [ROADMAP.md](ROADMAP.md)
**Estimate:** 2–3 weeks

## Outcome

The feature set becomes a coherent V1: semantic colors, a compact status bar, a
command palette, TOML settings and keymaps, notifications, refresh-on-focus,
error dialogs, crash recovery, file logging, documentation, and a performance
pass. A new user can discover every V1 feature without reading the source.

**Demo:** A fresh configuration opens a cart, shows colors, sizes, and
diagnostics, lists every command in the palette, rebinds a global key, recovers
an unsaved buffer after a forced kill, and opens a changed file without losing
work. A 64KB cart stays responsive.

## Scope

### In scope

- Semantic token color mapping to the 16-color palette.
- Status bar: mode, input mode, file/tab, position, diagnostics, sizes, LSP.
- Command palette, settings file, keymap configuration, and notifications.
- Refresh-on-focus for external changes and config reload.
- Error dialogs, crash recovery, and rotating file logs.
- Documentation, screenshots, and performance benchmarks.
- Final V1 bug pass.

### Out of scope

- Full file watching with `fsnotify` (post-V1).
- Run-cart in an installed PICO-8 binary (post-V1).
- Themes beyond the PICO-8 palette.
- Multi-cursor, macros, marks, splits, plugins, AI, terminal, and remote work.
- Save-as, rename, delete, and new-file tree operations (post-V1).
- macOS and Windows packaging.

## Deliverables

1. Final semantic color table and render tests.
2. Status bar layout with truncation priorities.
3. Command registry and palette UI.
4. TOML settings and keymap loading, validation, precedence, and reload.
5. Notification system with history.
6. Refresh-on-focus behavior for buffers, tree, and config.
7. Error dialogs, crash recovery journal, and rotating logs.
8. `README.md`, `docs/USAGE.md`, `docs/CONFIG.md`, `docs/KEYMAP.md`,
   `docs/TROUBLESHOOTING.md`, and screenshots.
9. Performance benchmarks and a 64KB cart acceptance test.
10. A documented P0/P1 bug triage with zero open blockers.

## Detailed requirements

### 1. Semantic colors

Use palette indices only. The final table:

| Token type | Index | Color | Notes |
| --- | --- | --- | --- |
| `keyword` | 14 | `#FF77A8` | control flow, declarations |
| `string` | 11 | `#00E436` | includes escapes |
| `number` | 10 | `#FFEC27` | numbers and numeric literals |
| `comment` | 13 | `#83769C` | readable on black |
| `operator` | 6 | `#C2C3C7` | punctuation |
| `variable` | 7 | `#FFF1E8` | default identifier color |
| `parameter` | 15 | `#FFCCAA` | function parameters |
| `property` | 15 | `#FFCCAA` | fields and members |
| `function` | 12 | `#29ADFF` | calls and declarations |
| `type` / `class` / `enum` | 9 | `#FFA300` | type-like symbols |
| `namespace` | 14 | `#FF77A8` | modules and namespaces |
| `macro` | 9 | `#FFA300` | macro-like symbols |
| `regexp` | 11 | `#00E436` | regex literals |
| `decorator` | 9 | `#FFA300` | annotations |
| `modifier` | 13 | `#83769C` | modifiers and attributes |
| default | 7 | `#FFF1E8` | unknown token types |

Modifiers:

- `declaration` / `definition`: no color change; declarations read the same as
  uses.
- `readonly` / `static`: index 6 instead of the base color.
- `deprecated`: index 8, plus a one-cell `!` marker in the status bar when the
  cursor is on the token.
- `defaultLibrary`: index 13.
- Unknown modifiers are ignored.

Rendering:

- Token priority is fixed: comment > string > number > keyword > type > function
  > property > variable > operator > parameter > namespace > default. Later
  tokens never overwrite higher-priority cells.
- The current line is background index 1; text keeps its token color.
- Selection is background index 2; text keeps its token color.
- Search matches use background index 3; the current match uses background
  index 9 with foreground index 0.
- Diagnostics underline errors with index 8 and warnings with index 9, one
  pixel on row `y+5` of the cell.
- Cursor: block inverts the cell(s); insert bar uses index 10.

### 2. Status bar

One row, 64 cells, at the bottom. Segments, left to right:

```text
<N|I|V|VL|VB|:><LIT|HIRA|KATA>  <file>[:<tab>]<*>  <line>:<col>  E<n> W<n>  T<n> C<n> Z<n>  [LSP]
```

- Mode: `N`, `I`, `V`, `VL`, `VB`, `:`; append `LIT`, `HIRA`, or `KATA` when
  active.
- File: base name, optional cart tab index, `*` when dirty.
- Position: 1-based line and 1-based cell column.
- Diagnostics: omit zero counts; color errors/warnings.
- Sizes: `T` tokens/8192, `C` chars/65535, `Z` compressed bytes. Color `Z` at
  the warning/error thresholds. Show `n/a` for standalone `.lua`.
- LSP: `LSP` when ready, `LSP!` when failed, hidden when off. Use index 8 for
  failure.
- Notifications temporarily replace the rightmost segments for 4s.
- Truncation priority: drop sizes, then diagnostics, then file path segments;
  never drop mode, position, or LSP failure. Truncate the middle of long paths
  with `…`.
- The command-line prompt replaces the status row while active.

### 3. Command registry and palette

```go
type Command struct {
    ID       string   // "file.save"
    Title    string   // "File: save"
    Category string   // "File"
    Run      func(ctx Context) error
    Enabled  func(State) bool
}
```

Required commands (IDs are stable; keymaps and docs use them):

`file.save`, `file.close`, `file.reload`, `buffer.next`, `buffer.prev`,
`buffer.list`, `cart.tab.next`, `cart.tab.prev`, `tree.toggle`, `tree.refresh`,
`palette.show`, `help.open`, `help.input`, `diag.list`, `messages.show`,
`lsp.restart`, `glyph.pick`, `input.literal`, `config.reload`, `app.quit`.

Palette:

- `Ctrl+Shift+P` opens a centered 48×12 popup.
- Fuzzy match on title, category, and ID; show the bound key on the right.
- Up/Down, PageUp/PageDown, Enter, Esc.
- Disabled commands are hidden unless the user types their ID.
- Every V1 feature must have a palette entry; a test enumerates the registry
  and fails on a missing documentation link.

### 4. Settings

TOML, two layers:

1. Global: `$XDG_CONFIG_HOME/pico-editor/config.toml`
   (`~/.config/pico-editor/config.toml` when unset).
2. Workspace: `<root>/.pico-editor.toml`.

Precedence: workspace > global > defaults. Tables merge field by field; arrays
replace. Unknown keys warn once; invalid values fall back to the default and
warn. `version = 1` is required for forward compatibility; a missing version is
treated as 1.

```toml
version = 1

[editor]
scale = 4                 # 0 = auto, 2..8 when fixed
font = "default"          # default | puny
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

- Unknown top-level tables and keys are ignored with a warning.
- Invalid ranges (for example `scale = 1` or `scale = 99`) fall back and warn.
- `editor.font = "puny"` switches the M6 atlas and metrics.
- `lsp.enabled = false` starts the editor without pico-lsp; all LSP UI is
  hidden and the editor remains fully usable.
- `config.reload` and `:reload` reload both layers. Refresh-on-focus reloads
  when either file's mtime changes.
- Settings changes that cannot apply live (font atlas, scale) show a
  notification asking for a restart; V1 does not hot-swap the font.
- `docs/CONFIG.md` documents every key with type, default, and range.

### 5. Keymaps

- Sections: `[keys.normal]`, `[keys.insert]`, `[keys.visual]`,
  `[keys.command]`.
- Entries map a key string to a command ID: `"ctrl+s" = "file.save"`.
- Only global/editor commands are remappable. Vim grammar keys (`d`, `c`, `y`,
  motions, text objects, ex commands) are fixed in V1; `:map` is out of scope.
- Defaults:

  | Key | Mode | Command |
  | --- | --- | --- |
  | `ctrl+s` | normal, insert, visual | `file.save` |
  | `ctrl+shift+b` | normal | `tree.toggle` |
  | `ctrl+shift+p` | normal, insert | `palette.show` |
  | `ctrl+tab` / `ctrl+shift+tab` | normal | `cart.tab.next` / `cart.tab.prev` |
  | `ctrl+shift+g` | normal, insert | `glyph.pick` |
  | `ctrl+q` | normal, insert | `app.quit` |
  | `f1` | all | `help.open` |
  | `ctrl+shift+m` | normal | `messages.show` |

- `Ctrl+P` is reserved for P8SCII literal mode in insert and must not be bound
  to a command there.
- Duplicate bindings in the same mode are a configuration error: the first
  default is removed, the user's binding wins, and a warning lists both.
- Keys in the frozen Vim matrix and reserved input/LSP bindings (`ctrl+b`,
  `ctrl+f`, `ctrl+p`, `ctrl+n`, `ctrl+space`, and the rest of `docs/VIM.md`)
  cannot be rebound to another command; validation rejects them. `:map` is out
  of scope.
- An empty string unbinds: `"ctrl+shift+b" = ""`.
- Key notation is the M0 format. Invalid keys are errors with the accepted
  grammar.
- `docs/KEYMAP.md` lists defaults, command IDs, and conflict behavior.

### 6. Notifications and messages

- Toast: top-right, max 3 stacked, width 40, auto-dismiss after
  `notifications.timeout_ms`.
- Errors persist until dismissed with `Esc` or `Enter`; info/success auto-
  dismiss.
- Show at most one identical message per 2s; increment a counter instead.
- `:messages` and the palette command open a scrollable history of the last
  `notifications.history` entries with severity and timestamp.
- Routine LSP logs never become toasts.

### 7. Refresh-on-focus

- On window focus:
  - stat every open buffer; auto-reload clean buffers; conflict-prompt dirty
    ones (M4 dialog),
  - rescan the workspace tree,
  - reload config if either file changed,
  - request a cart-info refresh for the active cart.
- Do not poll while focused; no background watcher in V1.
- A full `fsnotify` watcher is post-V1 and must not be added without a roadmap
  change.
- A manual `:e`/`:e!` remains available.

### 8. Errors, crash recovery, and logging

Error dialogs:

- Centered 48×10 modal with title, message, path, and operation.
- Buttons/actions are keyboard-driven: `Enter` primary, `Esc` cancel, arrows
  choose.
- Errors never close the buffer or quit the app.
- Panic recovery wraps command execution and LSP message handling; log the
  stack and show a notification. Fatal render/init errors log and exit 1.

Crash recovery:

- Directory: `$XDG_STATE_HOME/pico-editor/recovery/`.
- One file per dirty buffer: `<sha256(path)>.json` plus `<...>.txt`.
- Metadata: path, buffer kind, base version, original mtime/size, timestamp.
- Contents: the full file text (for carts, the string from `FullText()`
  including header and suffix, so recovery re-parses normally). If the splice
  returns an error, skip recovery for that buffer and notify; never write
  partial cart bytes.
- Write after 2s of idle dirty state, on focus loss, and on a quit attempt that
  is cancelled. Use atomic writes.
- Delete on clean save, close, or explicit discard.
- On startup, scan the directory; if entries exist, show a modal list with
  path and timestamp. **Recover** opens the buffer dirty; **Discard** deletes
  the entry; **Later** keeps it and continues.
- Cap at 100 entries and 1MB per entry; prune oldest with a warning.
- Recovery never writes the original file automatically.

Logging:

- `$XDG_STATE_HOME/pico-editor/pico-editor.log`, `0600`, directory `0700`.
- Rotate at 1MB, keep 3 files (`.1`, `.2`, `.3`).
- Levels: info lifecycle, warn recoverable, debug requests/edits, error fatal.
- Include component (`app`, `lsp`, `cart`, `fs`) and duration where useful.
- Never log file contents, clipboard contents, or configuration values that
  could contain secrets.
- `--log-level` overrides the file level; stderr logging is available with
  `--log-file -`.

### 9. Documentation and screenshots

- `README.md`: what it is, screenshot, install pointer, V1 feature list,
  disclaimer, license.
- `docs/USAGE.md`: open a folder/cart, edit, save, Vim basics, LSP features,
  P8SCII input, palette.
- `docs/CONFIG.md`: full TOML reference.
- `docs/KEYMAP.md`: defaults and command IDs.
- `docs/INPUT.md`: from M6.
- `docs/TROUBLESHOOTING.md`: graphics/audio backend, clipboard, IME, LSP
  restart, logs, recovery.
- Screenshots under `docs/images/` generated from golden surfaces: cart view,
  completion, picker, palette, error dialog. PNGs are committed and checked by
  a golden test.
- `:help` topics: `keys`, `input`, `lsp`, `config`, `troubleshooting`.

### 10. Performance

Budgets on the reference machine:

| Operation | Budget |
| --- | --- |
| Cold start to first frame | < 300ms |
| Open 64KB cart | < 200ms |
| Keystroke to frame | < 8ms |
| Save 64KB cart | < 100ms |
| Semantic token refresh after edit | < 150ms |
| Command palette open/filter | < 16ms |
| Memory after opening a 64KB cart | < 100MB |

- Add benchmarks for buffer edits, viewport rendering, token application, and
  palette filtering.
- CI records benchmark output but does not fail on noisy shared runners; the
  release checklist requires a local run on the reference machine.
- Profile CPU and allocations; remove per-frame allocations in the render path
  and avoid rebuilding the surface when nothing changed.

### 11. Final bug pass

- Triage all P0/P1 bugs in editing, saving, recovery, LSP, and input.
- V1 exits with zero open P0/P1 issues.
- P2 issues are recorded in the roadmap's post-V1 backlog, not fixed silently.
- Re-run the full manual matrix from M1–M6 on X11 and Wayland.

## Architecture and files

```text
internal/app/
  commands.go      registry and default bindings
  recovery.go      recovery journal
  focus.go         refresh-on-focus
internal/ui/
  palette.go       palette UI
  status.go        status segments and truncation
  notifications.go toast and history
  dialogs.go       modal errors and prompts
internal/config/
  settings.go      TOML loading and precedence
  keymap.go        keymap parsing and validation
internal/lsp/
  colors.go        semantic token -> palette mapping
docs/
  USAGE.md CONFIG.md KEYMAP.md TROUBLESHOOTING.md
  images/*.png
```

## Interfaces and state

```go
type Settings struct {
    Version int
    Editor  EditorSettings
    Files   FileSettings
    LSP     LSPSettings
    Notifications NotificationSettings
    Recovery RecoverySettings
    Keys    map[string]map[string]string
}

func LoadSettings(root string) (Settings, []Warning, error)
func (s Settings) CommandFor(mode, key string) (string, bool)
```

`Settings` is immutable after load. A reload builds a new value and swaps it at
the next frame boundary.

## Error handling

- A malformed TOML file is a startup error with path and line; the editor offers
  "Use defaults" or "Quit".
- A workspace config error does not prevent opening the folder; show a warning
  and use the global layer.
- Keymap conflicts warn and keep the user's binding.
- Recovery failures log and never block startup.
- A failed log rotation keeps the current file and warns.
- A failed screenshot golden test is a normal test failure, not a runtime error.

## Testing

### Automated

- Color table: every token type maps to a palette index; priority order golden.
- Status: truncation at 20, 40, 64 cells; no dropped mode/position/LSP failure.
- Command registry: every documented feature has an entry; IDs unique.
- Settings: precedence, array replacement, unknown keys, invalid values,
  version handling.
- Keymap: parse round-trip, conflicts, unbinding, reserved `Ctrl+P` in insert.
- Recovery: write, crash simulation, recover, discard, prune, atomic failure.
- Refresh-on-focus: clean reload, dirty conflict, config reload, tree rescan.
- Notifications: dedupe, timeout, history cap.
- Screenshots: golden PNGs for the five documented screens.
- Performance: benchmarks and a 64KB acceptance test.

### Manual

- Rebind `ctrl+s`, confirm the old binding is gone, and restart.
- Kill the process with unsaved changes; recover; save; verify the original.
- Change a cart in an external editor while focused elsewhere; refocus.
- Run on X11 and Wayland; verify graphics, clipboard, and dialogs.
- Read the docs as a new user and complete the quick start without help.

## Exit criteria

- [ ] All V1 features are discoverable through the palette and documented.
- [ ] No P0/P1 bugs remain in editing, saving, LSP, input, or recovery.
- [ ] Screenshot/golden tests cover font, palette, tabs, status, and dialogs.
- [ ] Settings and keymaps load, validate, and reload as specified.
- [ ] Refresh-on-focus handles clean and dirty external changes.
- [ ] Crash recovery works after a forced kill.
- [ ] Logs rotate and contain no file or clipboard contents.
- [ ] A 64KB cart opens and edits within the performance budgets.
- [ ] `mise run check` passes.

## Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Polish scope expands indefinitely | Out-of-scope list is enforced; P2 issues go to the backlog. |
| Status bar is unreadable at 64 cells | Fixed truncation priorities and 20/40/64-cell tests. |
| Config/keymap bugs lock users out | Defaults are always available; startup offers "Use defaults"; conflicts warn. |
| Recovery corrupts originals | Recovery never writes originals; atomic journal files; explicit recover. |
| Refresh-on-focus loses work | Clean-only auto-reload; dirty buffers use M4's conflict dialog. |
| Performance regresses | Budgets, benchmarks, and a 64KB acceptance test before exit. |
| Screenshots drift | Generate from golden surfaces and fail the golden test on change. |
