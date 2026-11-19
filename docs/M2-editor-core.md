# M2 — Editor core

**Status:** Draft
**Depends on:** M0, M1
**Blocks:** M3, M4
**Roadmap:** [ROADMAP.md](ROADMAP.md)
**Estimate:** 2–3 weeks

## Outcome

A reliable, P8SCII-aware text buffer with cursor, selection, editing, undo,
viewport, search, and safe `.lua` file IO. It knows nothing about Vim, carts, or
LSP. Every later feature edits through this layer.

**Demo:** Open a 64KB `.lua` file, edit it with the primitive API and a debug
keyboard map for a full session, undo and redo every change, search with `/`,
and save without corrupting line endings or the trailing newline.

## Scope

### In scope

- Line-based text buffer; no rope.
- Rune, cluster, cell, and UTF-16 position conversions.
- P8SCII-aware cursor, selection, and visual-column motion.
- Insert, delete, split/join lines, indentation, and tab handling.
- Linear undo/redo with grouped changes and save points.
- Viewport scrolling, horizontal scrolling, and scrolloff.
- Literal and regex search with match ranges.
- Open/save `.lua` files with atomic writes, modified state, and errors.
- Unit, property, fuzz, and performance tests.

### Out of scope

- Vim modes, counts, operators, registers, and ex commands (M3).
- Cart parsing, code tabs, and byte-preserving cart saves (M4).
- LSP document sync and language features (M5).
- P8SCII input modes and glyph picker (M6).
- Settings and keymap files (M7); constants are used in M2.
- Multi-cursor, folding, and syntax highlighting.

## Deliverables

1. `internal/editor` buffer, position, cursor, selection, edit, undo, viewport,
   and search types.
2. `internal/fsutil` atomic file writer with mode preservation and fsync.
3. `.lua` open/save with BOM and line-ending preservation.
4. A debug screen that exercises the buffer without Vim.
5. Tests for editing edge cases, wide glyphs, VS16 sequences, undo, search,
   save round-trips, and 64KB performance.

## Detailed requirements

### 1. Buffer model

```go
type Buffer struct {
    Path            string
    Lines           []string // no line terminators
    Newline         string   // "\n" or "\r\n"
    TrailingNewline bool
    HadBOM          bool
    Version         int
    SavedVersion    int
    Mode            os.FileMode
}

func Open(path string) (*Buffer, error)
func New(path string) *Buffer
func (b *Buffer) Text() string
func (b *Buffer) Dirty() bool
func (b *Buffer) MarkSaved()
```

- Files must be valid UTF-8. Invalid UTF-8 returns a typed error; do not replace
  bytes lossily.
- Detect and strip a leading UTF-8 BOM; re-add it on save.
- Split only on `\n`. A `\r` immediately before `\n` is consumed as part of a
  CRLF newline and restored on save. A lone `\r` is a character (P8SCII byte 13
  in canonical form), not a newline.
- Preserve a missing trailing newline exactly.
- Preserve the original file mode. Do not change ownership.
- `Version` increases on every accepted edit; `SavedVersion` records the last
  successful save. `Dirty()` is `Version != SavedVersion`.
- No rope. Carts and includes are small; line slices plus a rebuilt line index
  are fast enough. Rebuild indexes after each edit.

### 2. Positions and widths

```go
type Pos struct {
    Line int // 0-based
    Rune int // rune offset within the line, always on a cluster boundary
}

type PosInfo struct {
    Pos      Pos
    Byte     int // byte offset in Buffer.Text()
    Cell     int // display column in 4px cells
    UTF16    int // UTF-16 code units from line start
}
```

Requirements:

- Cursor positions are rune offsets, normalized to P8SCII cluster boundaries.
- `SetPos` clamps to the line end and snaps to the nearest cluster boundary.
- `CellColumn` uses `p8scii.CellWidth` for every cluster; wide glyphs advance
  2 cells.
- `UTF16Column` counts UTF-16 code units, matching LSP. It is computed from the
  line text, not from cell width.
- `ByteOffset`/`PosFromByte` round-trip for every cluster boundary.
- Tabs are literal `\t` characters. Display width is the next multiple of
  `TabWidth` (default 2); a tab at cell 0 occupies 2 cells, at cell 1 it
  occupies 1 cell. Editing Tab inserts `TabWidth` spaces in M2; literal tabs in
  existing files are preserved.
- Unknown runes are one cluster of width 4.

### 3. Cursor and selection

```go
type Cursor struct {
    Pos         Pos
    DesiredCell int // preserved across vertical motion
}

type Selection struct {
    Anchor Pos
    Cursor Pos
}

func (s Selection) Normalized() (start, end Pos)
func (s Selection) Empty() bool
```

- Horizontal motion steps whole clusters. `MoveLeft` at line start joins to the
  previous line's end; `MoveRight` at line end joins to the next line's start.
- Vertical motion preserves `DesiredCell`, clamps to the target line's cell
  width, and lifts over wide glyphs (never places the cursor inside a glyph).
- `MoveWordForward/Backward`, `MoveBigWordForward/Backward`, `MoveLineStart`,
  `MoveFirstNonBlank`, `MoveLineEnd`, `MoveBufferStart`, `MoveBufferEnd`,
  `MovePageUp/Down`, and `MoveToCell` are primitives; M3 maps keys and counts.
- Selection normalization is stable and never mutates the buffer.
- A selection can be characterwise or linewise in M2; blockwise selection is
  added in M3.

### 4. Edits

```go
type Edit struct {
    Start Pos
    End   Pos
    Text  string
}

type Change struct {
    Start    Pos
    OldText  string
    NewText  string
    Before   Pos // cursor before the edit
    After    Pos // cursor after the edit
}

func (b *Buffer) Apply(e Edit) (Change, error)
```

Required operations, all built on `Apply`:

- Insert text at a position, splitting on `\n` and updating `Lines`.
- Delete a range, joining lines when the range crosses a newline.
- Split a line at the cursor (Enter).
- Join the current line with the next (J), preserving one space unless the
  current line ends in whitespace or the next starts with `)`/`]`/`}`.
- Insert/remove indentation for a line range.
- Replace one cluster.
- Delete or yank a line range.

Rules:

- Every edit is rejected if `Start`/`End` are not cluster boundaries.
- An edit never produces invalid UTF-8.
- `Change` contains enough information for undo without re-reading the file.
- After a successful edit, increment `Version` and normalize the cursor.
- Edits that produce no textual change return an empty `Change` and do not
  increment `Version`.

### 5. Undo and redo

```go
type UndoStack struct {
    Undo []Change
    Redo []Change
}
```

- Linear undo/redo; no undo tree in V1.
- `u`/`Ctrl+R` are M3 bindings; M2 provides `Undo()`, `Redo()`, `CanUndo()`,
  `CanRedo()`.
- A new edit clears the redo stack.
- Grouping:
  - `BeginGroup()` / `EndGroup()` coalesce consecutive insertions into one undo
    entry.
  - Insert sessions end on Esc, mode change, cursor motion, or 500ms of idle
    time. M3 calls these boundaries.
  - Each operator (`dw`, `dd`, `p`, `:s`) is one entry.
- Undo restores the cursor to `Before`; redo restores `After`.
- Save points: undo back to `SavedVersion` makes `Dirty()` false without a new
  save. Keep the saved text hash, not just a version number, so this survives
  redo.
- Cap history at 1000 changes; discard oldest entries first.

### 6. Viewport

```go
type Viewport struct {
    TopLine  int
    LeftCell int
    Width    int // cells; 64 in the app
    Height   int // rows; 39–40 in the app
    ScrollOff int
}
```

- `EnsureVisible(cursor, cellCol)` adjusts `TopLine` and `LeftCell` with
  scrolloff (default 2).
- `LeftCell` scrolls by whole cells. A wide glyph partially off-screen is
  clipped; the cursor still occupies both cells.
- Horizontal scroll must not move the cursor's cell; vertical scroll preserves
  `DesiredCell`.
- `Viewport.VisibleLines(buffer)` returns the global line numbers and display
  rows for the current window. M4 uses the same API with a tab offset.

### 7. Search

```go
type Search struct {
    Pattern  string
    Regex    bool
    SmartCase bool
    Forward  bool
}

type Match struct {
    Start Pos
    End   Pos
}

func (s *Search) Find(text string, from Pos) (Match, bool)
```

- Literal search by default; regex mode uses Go's `regexp` package.
- Smartcase: all-lowercase patterns are case-insensitive; any uppercase rune
  makes the search case-sensitive.
- Matches never start or end inside a cluster. If a regex match lands inside a
  VS16 sequence, expand it to the whole cluster.
- Search wraps at the buffer ends and reports whether it wrapped.
- M2 finds and returns matches; M3 adds `/`, `?`, `n`, `N`, `*`, `#`, and
  highlighting.
- Invalid regex returns a typed error; it must not panic or clear the previous
  pattern.

### 8. File IO

- Open:
  - reject directories, missing files, and files larger than 2MB with a typed
    error,
  - read bytes, validate UTF-8, strip BOM, detect newline, split lines,
  - initialize `Version = 1`, `SavedVersion = 1`.
- Save:
  - join lines with `Newline`,
  - append a trailing newline only if `TrailingNewline`,
  - prepend BOM if `HadBOM`,
  - write with `internal/fsutil.AtomicWrite`,
  - update `SavedVersion`, `Version`, and the saved-text hash.
- Atomic write:
  - create a temp file in the same directory with mode `0600`,
  - write all bytes, `Sync`, `Chmod` to the original mode, `Close`,
  - `Rename` over the target,
  - open and `Sync` the directory,
  - remove the temp file on any failure.
- Saving a read-only file returns a typed permission error and leaves the
  original untouched.
- Saving a symlink writes through to the resolved target and preserves the
  symlink. Refuse if the resolved target escapes the workspace root without an
  explicit confirmation (M4 adds the dialog; M2 returns an error).
- No automatic backups in M2; M4 adds conflict backups.

### 9. Debug screen

- A minimal buffer view renders the current viewport with the cursor and
  selection, backed by a temporary key map: arrows move, Enter splits,
  Backspace deletes, `Ctrl+S` saves, `Ctrl+Z`/`Ctrl+Y` undo/redo.
- This screen is deleted when M3 lands; it is not a shipped editor mode.

## Architecture and files

```text
internal/editor/
  buffer.go        Buffer, lines, file IO helpers
  position.go      Pos, byte/rune/cell/UTF-16 conversions
  cursor.go        cursor motion primitives
  selection.go     selection normalization
  edit.go          Edit, Apply, line operations
  undo.go          undo/redo stack and grouping
  viewport.go      scrolling and visibility
  search.go        literal/regex search
  editor.go        Editor: buffer + cursor + selection + viewport + undo
internal/fsutil/
  atomic.go        atomic write and directory fsync
internal/app/
  debug_buffer.go  temporary debug screen
```

## Interfaces and state

```go
type Editor struct {
    Buffer    *Buffer
    Cursor    Cursor
    Selection Selection
    Viewport  Viewport
    Undo      UndoStack
    Search    Search
}

func (e *Editor) Move(...)
func (e *Editor) Insert(text string)
func (e *Editor) DeleteSelection()
func (e *Editor) Undo()
func (e *Editor) Redo()
```

`Editor` is the only mutable editing state. M3 adds a Vim state machine above
it; M4 adds a cart adapter below it; M5 maps LSP positions through `Position`.

## Error handling

- Open/save errors are typed (`ErrNotUTF8`, `ErrTooLarge`, `ErrReadOnly`,
  `ErrExternalChange`) and carry the path.
- Editing errors (invalid position, invalid range) leave the buffer unchanged.
- Search errors preserve the previous pattern.
- Any panic in an edit operation is a test failure; add a fuzz target for
  sequences of edits, undo, and redo.
- Never partially write a file.

## Testing

### Unit and property tests

- Position round-trips: byte ↔ rune ↔ cell ↔ UTF-16 for ASCII, P8SCII glyphs,
  VS16 sequences, tabs, and empty lines.
- Cursor motion across narrow/wide glyphs and line boundaries.
- Edit, undo, redo, and redo invalidation.
- Save-point dirty tracking after undo/redo cycles.
- CRLF, lone CR, BOM, missing trailing newline, and empty file round-trips.
- Join/split line edge cases and indentation.
- Search literal/regex, smartcase, wrapping, and cluster expansion.

### Fuzz tests

- Random edit sequences followed by undo-all: the buffer equals the initial
  text.
- Random save/load: bytes are identical for valid UTF-8 inputs.

### Performance

- Open and split a 64KB, 8,000-line file in under 100ms.
- Apply a single-character edit in under 2ms on the target machine.
- `go test -bench` records the baseline; M7 enforces the full UI budget.

## Exit criteria

- [ ] A `.lua` file can be edited for a full session without corruption or
      panics.
- [ ] Cursor movement across wide glyphs and VS16 sequences is correct.
- [ ] Undo/redo and save round-trips are covered by tests.
- [ ] Undo back to the save point clears the dirty flag.
- [ ] Typing and scrolling stay smooth on a 64KB file.
- [ ] Atomic save leaves the original intact on injected failure.
- [ ] `mise run check` passes.

## Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Cell/rune/UTF-16 confusion | One `Position` type with explicit conversion tests; no ad-hoc column arithmetic. |
| Wide glyph cursor bugs | Cluster-snapping API; test every transition around wide glyphs and EOL. |
| Undo corrupts text | Property test random edits; store old/new text, not deltas. |
| Line-ending loss | Explicit `Newline`/`TrailingNewline` state; golden round-trip tests. |
| Search inside VS16 | Expand matches to cluster boundaries; test all VS16 glyphs. |
| Performance regressions | Line slices, benchmarks, and a 64KB fixture in CI. |
