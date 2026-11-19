# pico-editor V1 Vim command matrix

**Status:** Frozen by M0. M3 implements this matrix; it must not grow without a
[ROADMAP.md](ROADMAP.md) update and a new matrix fixture.

This document is normative. The editor is not a full Vim: everything not listed
here is out of scope for V1 unless a later roadmap revision says otherwise.
Cross-cutting design lives in [ARCHITECTURE.md](ARCHITECTURE.md).

## Key notation

Key names are used by this matrix, by tests, and by `config.toml`. A binding is
written as zero or more modifiers followed by a key name:

```text
[ctrl+][alt+][shift+][meta+]<key>
```

- Modifiers are lowercase and appear in the order above. The final component is
  a US-layout key name.
- Single-character key names are lowercase letters and digits: `a`, `z`, `0`.
- Named keys: `esc`, `enter`, `space`, `tab`, `backspace`, `delete`, `home`,
  `end`, `pageup`, `pagedown`, `left`, `right`, `up`, `down`, `insert`, `f1`
  through `f12`.
- Examples: `ctrl+q`, `ctrl+shift+p`, `alt+enter`, `shift+letter`.
- In the matrix, `Ctrl+Q`-style spellings are documentation only; the canonical
  config spelling is `ctrl+q`. `Esc` and `Ctrl+[` are equivalent inputs.
- A bare letter means the unshifted key. Uppercase letters in commands such as
  `J`, `K`, `P`, `X`, `D`, `C`, and `Y` mean Shift+letter and are normal-mode
  commands, never P8SCII glyph input.
- `Shift+letter` insertion is an insert-mode input rule owned by the P8SCII
  layer (M6). It does not change any normal-mode binding here.

## 2. Command matrix

### 2.1 Entry, exit, and repeat

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

### 2.2 Motions

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

### 2.3 Operators

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

### 2.4 Text objects

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

### 2.5 Visual mode

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

### 2.6 Insert mode

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

### 2.7 Search

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

### 2.8 Ex commands

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

### 2.9 `:s` substitution

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

### 2.10 Registers and clipboard

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

## Counts, dot repeat, and undo

These rules are part of the frozen matrix:

- A count before an operator and a count before a motion multiply
  (`3d2w` deletes six words).
- `dd`/`cc`/`yy` take an optional count of lines.
- `G` with a count goes to that line; `gg` likewise.
- Visual selections ignore a count except for motions that accept one.
- A count of `0` is the `0` motion unless a nonzero count has already started.
- A count before `i`/`a` repeats the inserted text; insert sessions are one undo
  group.
- `.` repeats the last change at the cursor. `cw` and `s` repeat the replacement
  text without reusing deleted text. `u`, ex commands, and search do not update
  the dot record.
- Every normal-mode change is one undo group. Undo/redo restores the cursor at
  the change boundaries and clears the modified flag when the save point is
  reached again.
- `:s` is one undo entry per invocation.

## Out of scope

The following are explicitly not implemented in V1:

- Macros (`q`, `@`), marks (`m`, `` ` ``, `'`), sessions, and persistent undo.
- Plugins, scripts, `:source`, `:normal`, `:g`, `:v`, `:terminal`, `:set`,
  `:map`, autocommands (`:autocmd`), abbreviations, and expression registers.
- Splits, windows, Vim tabs, folding, and diff mode.
- `R` replace mode, `g?` rot13, `gq` formatting, and `=` reindent filters.
- References, rename, formatting, code actions, inlay hints, and code lens
  (except as listed for M5/LSP UI).
- Multi-cursor and visual-block `$`/`o` edge cases beyond the matrix.
- Additional `:s` features (`:&&`, `~` replacement history, expression
  replacement) and `:global`/`:vglobal`.
- P8SCII input modes themselves: this matrix only defines where normal-mode
  commands and insert-mode text are separated. M6 owns the tables and UI.

## Change control

- Adding, removing, or changing a row requires a roadmap update and at least one
  new fixture in `internal/vim/testdata/`.
- Every row must map to a named fixture; a missing fixture fails CI.
- The matrix is closed for V1. New Vim surface area goes to the post-V1
  backlog rather than being smuggled into an implementation PR.
