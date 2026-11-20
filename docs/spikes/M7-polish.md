# M7 polish and hardening — verification notes

**Status:** Automated M7 checks pass. Interactive X11/Wayland checks are part of
the M8 release pass.
**Spec:** [M7-polish.md](../M7-polish.md)

## Automated evidence

| Area | Coverage |
| --- | --- |
| Semantic colors | `TestM7ColorTable`, `TestTokenPriorityAndModifiers`, `TestSelectionAndSearchBackgroundsArePainted`, screenshot goldens |
| Status bar | 20/40/64-cell truncation tests, required-segment tests, notification replacement |
| Commands | registry ID/doc test, required command list, palette filtering, key display |
| Settings | defaults, precedence, array replacement, unknown keys, invalid values, versions, custom glyphs |
| Keymaps | defaults, rebind, unbind, reserved/grammar rejection, layer merge, palette display |
| Notifications | dedupe, persistent errors, zero timeout, history cap, routine LSP no-toast |
| Refresh-on-focus | clean reload, dirty conflict, config stamp reload, forced cart info |
| Recovery | atomic save/load/delete, oversize, bad directory, prune, plain/cart recover, invalid cart skip, startup list actions |
| Logging | private mode, level filtering, rotation, three backups, `--log-file -` |
| Performance | palette, draw, token apply, buffer insert, 64KB open/edit acceptance |
| Screenshots | five 2× PNGs under `docs/images/` generated from surfaces |

## Reference-machine numbers

Measured on the development machine (AMD Ryzen 7 7840U), informational only:

```text
64KB cart open        1.4 ms
64KB cart edit        1.4 ms
session draw        30–50 µs/op
palette filter        5 µs/op
token apply         30–50 µs/op
buffer insert         3–6 µs/op
```

## Manual checklist

- [ ] Rebind `ctrl+s`, confirm the old binding is gone, restart, and confirm
      the new binding persists.
- [ ] Kill the process with `SIGKILL` while a cart is dirty; restart; choose
      Recover; save; verify the original cart sections and suffix are intact.
- [ ] Change a cart in an external editor while the app is focused elsewhere;
      refocus a clean buffer (auto-reload) and a dirty buffer (conflict dialog).
- [ ] Recover/Discard/Later with multiple journal entries, including one whose
      original file was deleted.
- [ ] Run on X11 and Wayland; verify integer scaling, clipboard, dialogs, and
      recovery prompts.
- [ ] Read `README.md`, `docs/USAGE.md`, `docs/CONFIG.md`, and
      `docs/KEYMAP.md` as a new user and complete the quick start without help.
- [ ] Force a log rotation with `--log-level debug` and confirm three backups.
