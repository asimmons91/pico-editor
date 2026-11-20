# M4 cart and file tree manual checklist

Status: automated coverage passes. The interactive checks below still need a
run on a real PICO-8 cart collection and desktop backend.

Automated commands:

```sh
mise run check
```

Automated coverage in this repo:

- `internal/cart`: open/save byte round-trips for all-sections, BOM, CRLF,
  empty body, leading/consecutive markers, lone CR, no trailing newline, and
  unterminated headers; edit-only-body goldens; conflict/deleted/read-only
  handling; numbered backups; tab resynchronization; header/suffix hashes.
- `internal/workspace`: nested scans, sorting, hidden/ignored entries, file and
  directory symlinks, include ownership, nearest-cart selection, refresh.
- `internal/editor`: active line-window clamping, boundary joins/deletes, empty
  tab materialization, undo/redo.
- `internal/vim`: existing matrix plus tab-bounded `G`, `gg`, `J`, search, and
  ex ranges.
- `internal/app`: cart opening, tab switching and view restoration, empty-tab
  editing, conflict overwrite with backup, read-only error, quit Save all,
  outside-root symlink confirmation, tree startup/filtering, external focus
  reloads, and tab clicks.

Interactive checklist:

| Check | Steps | Expected |
| --- | --- | --- |
| Open real carts | Open three carts from different PICO-8 versions from the tree | Code-only view, correct `0`, `1`, … tabs, no `__gfx__`/`__sfx__` text |
| Tab state | Move and scroll in tab 0, switch to tab 1 and back | Cursor and scroll restore per tab |
| Save isolation | Edit a line, `:w`, run `git diff` | Only the `__lua__` body differs |
| Empty tab | Open a cart with a leading `-->8`, type in the empty tab | Text appears before the marker; marker is not edited |
| Empty tab commands | Try `x`, `r`, `J`, `dd`, `>>`, `~` in an empty tab | Separator and other tabs are untouched |
| Include owner | Open an included `.lua` from the tree | Status shows `[owning-cart.p8]`; save edits the include |
| Conflict | Edit a cart, change it in PICO-8, then save in pico-editor | Conflict dialog; Overwrite creates `.bak`; Reload/Cancel behave |
| Deleted file | Delete the open cart externally and save | Recreate/Cancel dialog; Cancel leaves the buffer dirty |
| Read-only | `chmod -w` the cart, edit, save | Error dialog names the path and OS error; file unchanged |
| Outside root | Open a workspace symlink pointing outside the root and save | Save anyway/Cancel dialog; the symlink remains a symlink |
| Focus refresh | Change a clean open file externally, refocus the editor | Clean buffer reloads; dirty buffer prompts |
| Tree | `Ctrl+Shift+B`, `j`/`k`, `Enter`, `h`/`l`, `/`, `r`, `q`, `Esc` | Tree navigates and opens files; dirty/open markers are visible |
| Quit | Dirty two buffers, `Ctrl+Q` or close the window | Save all / Discard all / Cancel; Save all writes both |
| Tabs | `Ctrl+Tab`, `Ctrl+Shift+Tab`, click file and cart tabs | Active tab changes and stays visible |
| Startup | Run with no argument, a directory, and a file | Directory/no-arg opens the tree; a file opens the editor |

Known V1 limits:

- `.p8.png` remains unsupported.
- Search and substitutions stay inside the active code tab.
- Full file watching is deferred to M7; M4 refreshes on focus and on demand.
- There is no Save as, rename, delete, or new-file command in M4.
