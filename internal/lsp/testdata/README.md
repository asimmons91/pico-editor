# LSP integration fixtures

The carts and Lua files in this directory are copied unchanged from
`github.com/asimmons91/pico-lsp` `v1.6.0`:

- `navigation/cart.p8`, `navigation/lib/player.lua`, `navigation/lib/util.lua`
  from `testdata/navigation/` (definition and include coverage).
- `smoke.p8` from `testdata/smoke.p8` (P8SCII glyphs, tabs, and UTF-16 mapping).
- `objects.p8` from `testdata/annotated/objects.p8` (LuaCATS semantic tokens).

pico-lsp is MIT-licensed and lives in its own repository. These copies exist so
the editor's integration tests do not depend on the Go module cache path. Update
them only when pinning a newer pico-lsp release and re-run the LSP tests.
