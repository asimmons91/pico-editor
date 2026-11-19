# M3 — Vim curated core

**Status:** Draft
**Depends on:** M2
**Blocks:** M6
**Roadmap:** [ROADMAP.md](ROADMAP.md)
**Estimate:** 6–8 weeks

## Outcome

Daily-driver modal editing for Lua and PICO-8 code: normal, insert, visual
character/line/block, command-line, counts, operators, text objects, registers,
search, undo, dot repeat, and the curated ex commands including `:s`.

**Demo:** Run the full command matrix against fixture files. Edit a real cart
tab end to end with only Vim keys, including a visual-block indent and a
`:%s/old/new/g`, then save. Normal-mode Shift commands (`J`, `K`, `P`, `X`,
`D`, `C`, `Y`) behave as Vim commands and are never consumed by P8SCII input.

## Scope

### In scope

- Normal, insert, visual character, visual line, visual block, and command-line
  modes.
- Counts, operator-pending state, registers, dot repeat, and undo/redo.
- The exact motion, operator, text-object, visual, search, insert, and ex
  matrices below.
- `:s` substitution with ranges and flags.
- P8SCII-aware motion, selection, and display.
- Mode indicator and a matrix-driven test suite.

### Out of scope

- Macros (`q`, `@`), marks (`m`, `` ` ``, `'`), jumps beyond the simple
  back/forward stack, and sessions.
- Plugins, scripts, `:source`, `:normal`, `:g`, `:v`, `:terminal`, `:set`,
  `:map`, and abbreviations.
- Splits/windows, tabs as Vim windows, folding, and diff mode.
- `R` replace mode, `g?` rot13, `gq` formatting, and `=` reindent filters.
- Multi-cursor and blockwise `$`/`o` edge cases beyond the matrix.

## Deliverables

1. `internal/vim` state machine, command parser, and executor.
2. Register store with unnamed, numbered, named, black-hole, and clipboard
   registers.
3. Search prompt, incremental highlight, Vim-magic-to-Go regex translation, and
   `:noh`.
4. Ex command parser and executor, including `:s`.
5. Visual block selection and block operations, including `I`/`A`.
6. Jump list with `Ctrl+O`/`Ctrl+I` for definition jumps (M5 uses it).
7. Matrix fixture tests, fuzz tests, and a manual checklist.
8. `docs/VIM.md` remains the normative copy of the matrix; M3 must not add
   commands without updating it.

## Detailed requirements

### 1. Modes and state machine

```go
type Mode uint8

const (
    ModeNormal Mode = iota
    ModeInsert
    ModeVisualChar
    ModeVisualLine
    ModeVisualBlock
    ModeCommandLine
)

type Vim struct {
    Mode      Mode
    Count     int
    Operator  Operator // pending operator, zero when none
    Motion    Motion   // pending motion while Operator is set
    Register  byte     // 0 = unnamed
    LastChange ChangeRecord
    LastSearch SearchState
    CmdLine   CmdLineState
    JumpList  JumpList
}
```

- Input is consumed one event at a time. `Handle(ev input.Event) Result` returns
  actions for the editor and mode/status changes.
- Pending counts and operators survive until a motion, text object, or cancel
  (`Esc`) resolves them. There are no mappings, so no timeout is required.
- `Esc` cancels a pending operator/count, exits visual/insert/command-line, and
  clears the search prompt. `Ctrl+[` is equivalent to `Esc`.
- Mode changes reset `Count`, `Operator`, and `Motion`.
- Every mode has an explicit indicator string: `NORMAL`, `INSERT`, `VISUAL`,
  `V-LINE`, `V-BLOCK`, `COMMAND`.
- Key events are used in all modes. Text events are used in insert mode and in
  the command-line prompt. Normal and visual modes ignore text events except
  where a command explicitly reads the next rune (`r`, `f`, `t`).

### 2. Command matrix

`docs/VIM.md` must contain these tables verbatim. Any change requires a roadmap
update and a new matrix test.

#### 2.1 Entry, exit, and repeat

| Keys | Behavior |
| --- | --- |
| `i` / `a` | Insert before / after the cursor |
| `I` / `A` | Insert at first non-blank / at end of line |
| `o` / `O` | Open a line below / above and enter insert |
| `v` / `V` / `Ctrl+V` | Visual character / line / block |
| `:` | Command line |
| `Esc`, `Ctrl+[` | Cancel pending state or return to normal |
| `.` | Repeat the last change |
| `u` / `Ctrl+R` | Undo / redo |
| `gv` | Reselect the last visual selection |
| `Ctrl+O` / `Ctrl+I` | Jump back / forward |
| `zz` / `zt` / `zb` | Center / top / bottom the cursor line |

#### 2.2 Motions

| Keys | Behavior |
| --- | --- |
| `h` `j` `k` `l` | Left, down, up, right (cluster-aware) |
| `w` `W` `b` `B` `e` `E` | Word and big-word motions |
| `0` `^` `$` | Line start, first non-blank, line end |
| `gg` `G` | Buffer start / end (or go to count line) |
| `f` `F` `t` `T` `;` `,` | Find character on line, repeat, reverse |
| `%` | Matching bracket, including `()`, `[]`, `{}` |
| `{` `}` | Paragraph backward / forward |
| `H` `M` `L` | Screen top / middle / bottom |
| `Ctrl+D` `Ctrl+U` | Half page down / up |
| `Ctrl+F` `Ctrl+B` | Full page down / up |
| `+` `-` `Enter` | First non-blank of next / previous line |
| `Backspace` `Space` | Previous / next line at the same cell, wrapping at edges |
| `n` `N` | Next / previous search match |
| `*` `#` | Search forward / backward for the word under the cursor |

#### 2.3 Operators

| Keys | Behavior |
| --- | --- |
| `d` `c` `y` | Delete / change / yank, awaiting a motion or text object |
| `dd` `cc` `yy` | Linewise variants |
| `D` `C` `Y` | Delete / change / yank to end of line |
| `x` / `X` | Delete cluster under / before the cursor |
| `s` / `S` | Change cluster / change whole line |
| `r` | Replace one cluster, awaiting a rune |
| `J` | Join the current line with the next |
| `~` | Toggle case of the cluster and move right |
| `>` `<` | Shift a motion/text-object range right/left; `>>`/`<<` for lines |
| `p` / `P` | Put after / before, honoring register type |
| `u` / `Ctrl+R` | Undo / redo |

#### 2.4 Text objects

| Object | Inner | Around |
| --- | --- | --- |
| Word | `iw` | `aw` |
| Big word | `iW` | `aW` |
| Parentheses | `i(` `i)` | `a(` `a)` |
| Braces | `i{` `i}` | `a{` `a}` |
| Brackets | `i[` `i]` | `a[` `a]` |
| Double quotes | `i"` | `a"` |
| Single quotes | `i'` | `a'` |
| Paragraph | `ip` | `ap` |

- Objects operate inside the current line first; multiline bracket/paragraph
  objects scan the buffer.
- `i(` and `i)` are aliases; likewise for braces and brackets.
- Quote objects skip escaped quotes. If the cursor is outside quotes, search
  forward on the line; if none, fail without changing the buffer.

#### 2.5 Visual mode

| Keys | Behavior |
| --- | --- |
| `v` / `V` / `Ctrl+V` | Character / line / block selection |
| `o` | Swap anchor and cursor |
| `gv` | Restore the last selection |
| motions, `gg`, `G`, `0`, `$` | Extend the selection |
| `d` `c` `y` | Delete / change / yank the selection |
| `>` `<` | Shift selected lines; block shifts per line |
| `~` `u` `U` | Toggle / lowercase / uppercase the selection |
| `J` | Join selected lines |
| `p` / `P` | Paste over the selection, preserving the register |
| `I` / `A` | Block only: insert/append on every line of the block |
| `:` | Enter command line with `'<,'>` range |
| `Esc` | Return to normal |

- Block selection is rectangular in **cells**, not runes. A wide glyph is
  either fully inside or fully outside; the block edge never splits it.
- Block `d`/`c`/`y` operate on the rectangle; `>`/`<` shift each covered line
  once.
- Block `I` inserts at the left edge of the block on every covered line; `A`
  appends at the right edge. Text is typed once and mirrored on commit (Esc).
- Block `p` replaces the rectangle; lines shorter than the block are padded with
  spaces.

#### 2.6 Insert mode

| Keys | Behavior |
| --- | --- |
| Printable text | Insert at the cursor |
| `Enter` | Split the line |
| `Backspace` | Delete before the cursor; join at line start |
| `Delete` | Delete the cluster under the cursor |
| `Tab` | Insert spaces to the next tab stop |
| `Ctrl+W` / `Ctrl+U` | Delete the previous word / to line start |
| `Esc`, `Ctrl+[` | Return to normal, moving left one cell |
| Arrows, `Home`, `End` | Move without leaving insert |
| `Ctrl+J` / `Ctrl+K` / `Ctrl+P` | P8SCII kana/puny modes (M6) |
| `Ctrl+Space` | Completion (M5) |
| `Ctrl+N` / `Ctrl+Shift+N` | Next / previous completion item when open (M5) |
| `Tab` / `Enter` | Accept completion when open (M5), otherwise normal behavior |
| `Esc` | Close completion first, then leave insert (M5) |

- Counts before `i`/`a` repeat the inserted text that many times on commit
  (`3iab<Esc>` inserts `ababab`).
- Insert sessions are one undo group.
- Shift+letter is handled by the P8SCII input layer before insert text is
  applied; normal-mode Shift commands are untouched.

#### 2.7 Search

| Keys | Behavior |
| --- | --- |
| `/` / `?` | Forward / backward search prompt |
| `n` / `N` | Next / previous match, honoring direction |
| `*` / `#` | Search the word under the cursor forward / backward |
| `:noh` | Clear search highlighting |

- Search is incremental: the prompt highlights matches as the pattern is
  typed. `Enter` commits; `Esc` cancels and restores the cursor.
- Smartcase: all-lowercase patterns are case-insensitive; any uppercase rune
  makes them case-sensitive.
- Vim magic is translated to Go regexp:
  - `.`, `*`, `[`, `]`, `^`, `$` keep their special meaning.
  - `\(` `\)` become groups; `\|` alternation; `\+` `\?` repetition;
    `\{n,m}` bounds.
  - Unescaped `(` `)` `|` `+` `?` `{` `}` are literals.
  - `\c`/`\C` are accepted as explicit case flags.
- Matches are expanded to cluster boundaries and never split a VS16 sequence.
- History: the last 100 patterns, navigable with Up/Down in the prompt.
- All matches are highlighted until `:noh` or a new pattern.

#### 2.8 Ex commands

| Command | Behavior |
| --- | --- |
| `:w` | Write the current buffer |
| `:q` | Close the current buffer; fail if dirty |
| `:q!` | Discard changes and close |
| `:wq`, `:x` | Write, then close |
| `:e` | Reload the current file; fail if dirty unless `!` |
| `:e!` | Reload, discarding changes |
| `:e <path>` | Open a path in the workspace |
| `:bn` / `:bp` | Next / previous file buffer |
| `:b <n>` | Switch to file buffer number `n` |
| `:ls` | List file buffers |
| `:tabn` / `:tabp` | Next / previous cart code tab |
| `:help` | Open the help overlay |
| `:help <topic>` | Open a help topic (`keys`, `input`, `lsp`, `config`) |
| `:diag` | List diagnostics (M5) |
| `:messages` | Show recent notifications (M7) |
| `:reload` | Reload configuration (M7) |
| `:s/.../.../[gi]` | Substitute (see below) |

- Command names accept unique prefixes (`:w`, `:wq`, `:e`); ambiguous prefixes
  produce an error.
- `:q` with one clean buffer exits the editor. With multiple buffers it closes
  the current one.
- `:e` without a path reloads; with a path it opens the file relative to the
  workspace root. M2's external-change rules apply.
- `:ls` prints numbered buffers with active and modified markers in a popup.
- Command-line editing: Backspace, `Ctrl+U`, `Ctrl+W`, Up/Down history, Tab
  completes command names, Enter executes, Esc cancels.
- Errors use familiar Vim text where practical, for example
  `E37: No write since last change`, `E486: Pattern not found`, and
  `E32: No file name`.

#### 2.9 `:s` substitution

Syntax:

```text
:[range]s/{pattern}/{replacement}/[flags]
```

- Ranges:
  - omitted: current line,
  - `.`: current line,
  - `%`: whole buffer,
  - `'<,'>`: last visual line range (with visual `:`),
  - `N` or `N,M`: absolute line numbers.
- Flags: `g` replaces every match on a line; `i` forces case-insensitive;
  no flag replaces the first match per line.
- Replacement: `&` inserts the whole match; `\1`–`\9` insert capture groups;
  `\/` inserts a literal slash; `\\` a literal backslash.
- Patterns use the same Vim-magic translation and smartcase rules as `/`.
- Each `:s` invocation is one undo entry.
- No match is an error (`E486`) and must not modify the buffer.
- Not in V1: `:g`, `:v`, `:normal`, `:&&`, `~` replacement history, and
  expression replacement.

#### 2.10 Registers and clipboard

| Register | Behavior |
| --- | --- |
| `""` | Unnamed; last yank or delete |
| `"0` | Last yank |
| `"1`–`"9` | Delete history, newest first |
| `"a`–`"z` | Named registers; lowercase replaces |
| `"A`–`"Z` | Append to the matching named register |
| `"_` | Black hole; discard |
| `"+` / `"*` | System clipboard |

- Registers store text and a type: characterwise, linewise, or blockwise.
- `p`/`P` respect the type. Linewise puts go on a new line; blockwise puts
  insert a rectangle.
- `y` writes to `""` and `"0`; `d`/`c`/`x` write to `""` and rotate `"1`–`"9`.
- Clipboard read/write uses the M1 clipboard adapter. If unavailable, show a
  notification and keep the internal register.
- Clipboard text is canonical Unicode with LF newlines.

### 3. Counts and operator application

- A count before an operator and a count before a motion multiply
  (`3d2w` deletes six words).
- `dd`/`cc`/`yy` take an optional count of lines.
- `G` with a count goes to that line; `gg` likewise.
- Visual selections ignore a count except for motions that accept one.
- A count of `0` is the `0` motion unless a nonzero count has already started.
- `100<C-D>` and similar clamps are allowed; motions clamp at buffer edges.

### 4. Dot repeat

- Record the last change as an operator + motion + inserted text, or a simple
  command (`x`, `r`, `J`, `~`, `p`, visual operation).
- `.` repeats at the current cursor position, using the recorded count.
- Changes made in insert mode are replayed as one insertion; `cw` and `s`
  repeat the replacement text without reusing the deleted text.
- `u`, `:commands`, and search do not update the dot record.
- A failed repeat produces a bell/notification and does not change the buffer.

### 5. Undo integration

- Every normal-mode change is one undo group.
- Insert sessions group until Esc, a motion, a mode change, or 500ms idle.
- Visual operations are one group per command.
- `:s` is one group per invocation.
- Undo/redo restores the cursor using M2's `Change.Before`/`After`.
- Undo after a save back to the save point clears the modified flag.

### 6. P8SCII and platform boundaries

- Cursor and selection positions are cluster boundaries from M2.
- Word motions treat Unicode letters, digits, and `_` as keyword characters;
  P8SCII symbol glyphs are punctuation. This matches how PICO-8 identifiers are
  written.
- `%` scans brackets while skipping Lua strings and comments using a small
  lexical scanner; it does not call the LSP.
- Shift+letter key events in normal mode are Vim commands and must never be
  routed to P8SCII glyph insertion.
- In insert mode, the input layer may consume Shift+letter and
  `Ctrl+J`/`Ctrl+K`/`Ctrl+P` before Vim sees them. Vim receives the resulting
  inserted text, not the raw key.
- The editor does not implement P8SCII modes; M6 owns their tables and status
  indicator.

### 7. Jump list

- `Ctrl+O`/`Ctrl+I` traverse a per-session jump list.
- M3 records jumps for `gg`, `G`, `n`, `N`, `*`, `#`, `%`, and `:e`.
- M5 records definition jumps and cross-file jumps.
- Cap the list at 100 entries; jumping forward truncates the forward branch.

## Architecture and files

```text
internal/vim/
  vim.go           state machine entry point
  mode.go          mode types and indicators
  count.go         count parsing
  motion.go        motion resolution and application
  operator.go      operator-pending execution
  textobject.go    text object ranges
  visual.go        visual char/line/block ranges and operations
  register.go      register store and clipboard adapter
  undo.go          grouping boundaries
  dot.go           last-change recording and replay
  search.go        search prompt and highlight state
  regex.go         Vim magic -> Go regexp translation
  ex.go            command parser and executor
  commandline.go   command-line editing and history
  jump.go          jump list
  testdata/        matrix fixtures
```

## Interfaces and state

```go
type Result struct {
    Mode        Mode
    Status      string
    Notify      string // user-visible error or info
    Bell        bool
}

func (v *Vim) Handle(ev input.Event, ed *editor.Editor) Result
```

- `Vim` never owns the buffer; it mutates `editor.Editor` through public
  operations.
- Register storage lives in `Vim` so it survives buffer switches within a
  session.
- Search history and the last pattern live in `Vim`.
- Ex commands call `editor` and, from M5, an `LSPActions` interface passed at
  construction.

## Error handling

- Motions that cannot move leave the cursor unchanged and return `Bell: true`.
- Failed operators (`d` with no motion then Esc) change nothing.
- Invalid regex and `:s` errors preserve the buffer and show a message.
- A register/clipboard failure never aborts the command; internal registers
  still work.
- Ex commands that would lose changes fail with `E37` unless `!` is present.
- No panic on malformed key sequences; fuzz tests enforce this.

## Testing

### Matrix fixtures

Each command in §2 has at least one fixture:

```text
name: dw at word end
mode: normal
buffer: one two three
cursor: 0,4
keys: dw
want.buffer: one three
want.cursor: 0,4
want.mode: normal
```

- The runner loads a fixture, feeds key/text events, and compares buffer,
  cursor, mode, registers, and notifications.
- Every matrix row links to a fixture file by name; a missing fixture fails CI.
- Counts, operator combinations, text objects, visual modes, `:s` ranges, and
  register types each get dedicated cases.

### Fuzz

- Random sequences of keys and text events over a small buffer.
- Invariants: no panic; buffer remains valid UTF-8; cursor stays on a cluster
  boundary; undo/redo of any sequence restores the prior text.
- Seed corpus includes every matrix command.

### Manual

- Edit a real cart tab for 30 minutes using only Vim keys.
- Verify normal-mode Shift commands with P8SCII input enabled (M6).
- Verify `:%s` on a 2,000-line file and undo it.
- Verify visual-block indent and block insert.

### Performance

- A keystroke in a 64KB file completes in under 2ms excluding rendering.
- `:%s` over 8,000 lines completes in under 50ms and is one undo entry.

## Exit criteria

- [ ] Every matrix row has a passing fixture.
- [ ] Normal-mode Shift+letter commands work and are not consumed by P8SCII
      input.
- [ ] Visual block, block `I`/`A`, and `:s` pass their fixtures.
- [ ] Registers, dot repeat, and undo behave as specified.
- [ ] Search with Vim magic and smartcase passes its fixtures.
- [ ] Editing a real cart tab feels natural for a Vim user.
- [ ] Macros, marks, plugins, splits, and `:set` remain out of scope.
- [ ] Fuzz tests and `go test -race ./...` pass.
- [ ] `mise run check` passes.

## Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Vim scope expands without bound | The matrix is closed; additions require a roadmap update and new fixtures. |
| Operator/count interactions are wrong | Table-driven combinations; property tests; compare against Vim for each matrix row. |
| Visual block with wide glyphs is buggy | Block ranges are cell-based; never split a cluster; dedicated golden tests. |
| `:s` regex translation diverges from Vim | Central `regex.go` with a documented translation table and fixtures. |
| Dot repeat is subtly wrong | Record semantic changes, not raw keys; test insert, operator, and visual cases. |
| Shift+letter collision with P8SCII | Input layer consumes only insert-mode events; normal-mode tests assert untouched behavior. |
| Undo grouping feels wrong | Explicit group boundaries (Esc, motion, idle); manual feel test. |
