# P8SCII input

PICO-8 input is authentic by default: in insert mode, Shift+letter inserts
P8SCII glyphs and Ctrl+J/K/P switch PICO-8's input modes. Normal-mode Vim
commands are never converted.

The tables in this document were captured from PICO-8 0.2.7 on Linux. The raw
screenshots and the key manifest live in
[`testdata/pico8/input/`](../testdata/pico8/input/). The Go tables carry the
manifest hash so a reviewer can compare them with the capture.

## Mode summary

| Key | Mode | Status label | Meaning |
| --- | --- | --- | --- |
| `Ctrl+J` | Hiragana | `HIRA` | Type romaji; output is Hiragana |
| `Ctrl+K` | Katakana | `KATA` | Type romaji; output is Katakana |
| `Ctrl+P` | Puny | `PUNY` | Unshifted letters insert uppercase ASCII; Shift inserts standard letters |
| `Esc` | Default | — | Leaves insert mode and clears the P8 mode |

The three modes are mutually exclusive. Pressing `Ctrl+J`, `Ctrl+K`, or
`Ctrl+P` again turns the current mode off. `Ctrl+J` from Katakana switches to
Hiragana; `Ctrl+K` from Hiragana switches to Katakana; `Ctrl+P` from either kana
mode switches to Puny.

## Shift+letter

Shift is authentic only when it is the sole modifier. The committed OS text
must be the matching US uppercase letter; otherwise the editor preserves the
layout's text and logs the fallback once at debug level.

| Key | Byte | Glyph | Key | Byte | Glyph |
| --- | --- | --- | --- | --- | --- |
| Shift+A | 128 `0x80` | █ | Shift+N | 141 `0x8d` | ♪ |
| Shift+B | 129 `0x81` | ▒ | Shift+O | 142 `0x8e` | 🅾️ |
| Shift+C | 130 `0x82` | 🐱 | Shift+P | 143 `0x8f` | ◆ |
| Shift+D | 131 `0x83` | ⬇️ | Shift+Q | 144 `0x90` | … |
| Shift+E | 132 `0x84` | ░ | Shift+R | 145 `0x91` | ➡️ |
| Shift+F | 133 `0x85` | ✽ | Shift+S | 146 `0x92` | ★ |
| Shift+G | 134 `0x86` | ● | Shift+T | 147 `0x93` | ⧗ |
| Shift+H | 135 `0x87` | ♥ | Shift+U | 148 `0x94` | ⬆️ |
| Shift+I | 136 `0x88` | ☉ | Shift+V | 149 `0x95` | ˇ |
| Shift+J | 137 `0x89` | 웃 | Shift+W | 150 `0x96` | ∧ |
| Shift+K | 138 `0x8a` | ⌂ | Shift+X | 151 `0x97` | ❎ |
| Shift+L | 139 `0x8b` | ⬅️ | Shift+Y | 152 `0x98` | ▤ |
| Shift+M | 140 `0x8c` | 😐 | Shift+Z | 153 `0x99` | ▥ |

Shift+letter is checked before the committed text is applied, so a Shift+letter
never inserts both a glyph and an uppercase letter. Visual and command-line
modes do not convert Shift+letter.

## Kana modes

P8SCII stores 50 Hiragana (`0x9a`–`0xcb`) and 50 Katakana (`0xcc`–`0xfd`)
glyphs. PICO-8 uses the following romaji sequences. Katakana uses the same
sequences and is derived by shifting the Hiragana bytes by `0x32`.

### Vowels and consonant rows

| Romaji | Hiragana | Romaji | Hiragana | Romaji | Hiragana |
| --- | --- | --- | --- | --- | --- |
| `a` | あ | `i` | い | `u` | う |
| `e` | え | `o` | お | | |

| Row | a | i | u | e | o |
| --- | --- | --- | --- | --- | --- |
| `k` | か | き | く | け | こ |
| `s` | さ | し (`si`) | す | せ | そ |
| `t` | た | ち (`ti`) | つ (`tu`) | て | と |
| `n` | な | に | ぬ | ね | の |
| `h` | は | ひ | ふ (`hu`) | へ | ほ |
| `m` | ま | み | む | め | も |
| `r` | ら | り | る | れ | ろ |
| `g` | か゛ | き゛ | く゛ | け゛ | こ゛ |
| `z` | さ゛ | し゛ (`ji`) | す゛ | せ゛ | そ゛ |
| `d` | た゛ | ち゛ (`di`) | つ゛ (`du`) | て゛ | と゛ |
| `b` | は゛ | ひ゛ | ふ゛ | へ゛ | ほ゛ |
| `p` | は゜ | ひ゜ | ふ゜ | へ゜ | ほ゜ |
| `c` | か | — | く | — | こ |

`y` is `ya` や, `yu` ゆ, `yo` よ. `w` is `wa` わ and `wo` を. `wi`, `wu`, and
`we` have no P8SCII glyph and are discarded. `v` and `q` are literal letters,
so `va` inserts `v` followed by あ.

### Digraphs and small kana

| Romaji | Output | Romaji | Output |
| --- | --- | --- | --- |
| `kya` `kyu` `kyo` | きゃ きゅ きょ | `gya` `gyu` `gyo` | き゛ゃ き゛ゅ き゛ょ |
| `sha` `shu` `sho` | しゃ しゅ しょ | `ja` `ju` `jo` | し゛ゃ し゛ゅ し゛ょ |
| `sya` `syu` `syo` | しゃ しゅ しょ | `zya` `zyu` `zyo` | し゛ゃ し゛ゅ し゛ょ |
| `cha` `chu` `cho` | ちゃ ちゅ ちょ | `dya` `dyu` `dyo` | ち゛ゃ ち゛ゅ ち゛ょ |
| `nya` `nyu` `nyo` | にゃ にゅ にょ | `bya` `byu` `byo` | ひ゛ゃ ひ゛ゅ ひ゛ょ |
| `hya` `hyu` `hyo` | ひゃ ひゅ ひょ | `pya` `pyu` `pyo` | ひ゜ゃ ひ゜ゅ ひ゜ょ |
| `mya` `myu` `myo` | みゃ みゅ みょ | `rya` `ryu` `ryo` | りゃ りゅ りょ |

`tya`, `tyu`, and `tyo` are not valid; PICO-8 spells that row `cha`, `chu`,
`cho`.

Small kana:

| Romaji | Output |
| --- | --- |
| `lya` `lyu` `lyo` | ゃ ゅ ょ |
| `xya` `xyu` `xyo` | ゃ ゅ ょ |
| `ltsu` `xtsu` | っ |

### Doubled consonants and `n`

- A doubled consonant inserts っ and starts the next sequence with the second
  consonant: `kka` → っか, `kkya` → っきゃ, `ppa` → っは゜.
- `nn` inserts ん. `nna` → んあ and `nnka` → んか.
- `n` followed by a vowel is the normal n-row: `na` → な.
- `n` followed by `y` is the ny-row: `nya` → にゃ.
- Any other `n` followed by a consonant or a non-letter discards the pending
  `n` and the following consonant. For example, `nka` → あ and `nkka` → か.
  This is PICO-8's captured behavior, not an editor approximation.
- A pending sequence has no timeout in PICO-8 0.2.7. It is cleared by a
  non-letter key or by leaving insert mode.

### Kana Shift+digits

In both kana modes, Shift+1 through Shift+8 insert bytes `0x10`–`0x17`; Shift+9
inserts `0xfe` and Shift+0 inserts `0xff`. This was verified separately in both
Hiragana and Katakana modes against PICO-8 0.2.7.

Shift+letters keep the §Shift+letter table in both kana modes.

## Puny mode and uppercase identifiers

PICO-8 0.2.7 calls `Ctrl+P` **Puny font**, not a generic literal mode. It changes
input encoding, not rendering:

- unshifted `a`–`z` insert uppercase ASCII `A`–`Z` (the puny glyph slots),
- Shift+letter inserts the standard lowercase ASCII letter,
- Shift+letter does **not** insert the §Shift+letter symbol glyphs while Puny
  mode is active,
- `Ctrl+J`/`Ctrl+K` leave Puny mode and enter the requested kana mode.

Because PICO-8 identifiers case-fold, uppercase is rarely needed. To enter an
uppercase identifier such as `MY_VAR`, press `Ctrl+P` in insert mode and type
`my_var`. To return to normal typing, press `Ctrl+P` again.

The checked-in font atlas already contains PICO-8's puny glyph shapes at
uppercase ASCII (`0x41`–`0x5a`); lowercase ASCII (`0x61`–`0x7a`) uses the
standard glyphs. There is no second atlas or metric change, matching the
captured PICO-8 editor.

## Glyph picker

- Open with `Ctrl+Shift+G` or `:glyph`, or the palette command `Glyph: pick`.
- Filter by name, alias, byte (`0x80`, `128`), category, description, or the
  glyph text itself.
- `Up`/`Down`, `PageUp`/`PageDown`, `Home`/`End`, `Enter`, `Esc` are supported.
- Insert mode inserts at the cursor and stays in insert. Normal mode inserts at
  the cursor and enters insert after the glyph. Visual mode replaces the
  selection and returns to normal.
- The catalog comes from pico-lsp `pico8/glyphs`. If LSP is unavailable the
  picker does not open; a notification explains why.
- Custom entries are forwarded to the server by the settings layer (M7); the
  picker renders whatever the server returns and never invents names.

## `p8_left` completion

Typing `p8_` triggers pico-lsp completion after the normal 100ms debounce.
Accepting an item inserts the canonical Unicode text from the completion edit,
so `p8_left` and its `p8-left` compatibility alias both insert `⬅️`. Kana
entries are completable as `p8_hiragana_a`, `p8_katakana_a`, and so on. Inside a
string literal, ASCII `"` and `\` complete as `\"` and `\\` so the completion
cannot close or escape the string. The picker and completion share the server
catalog; a missing glyph is an upstream catalog issue, not an editor-local
exception.

## Clipboard

- Characterwise copy uses the selected text with LF newlines.
- Linewise copy terminates each selected line with `\n`.
- Blockwise copy pads every line to the widest selection line, preserving
  trailing spaces, and joins with `\n`.
- Paste strips a leading BOM (the clipboard adapter does this), normalizes CRLF
  and lone CR to LF, validates canonical P8SCII glyphs, and preserves unknown
  runes with a one-line warning.
- System bindings: `Ctrl+Shift+C` in visual mode; `Ctrl+Shift+V` in insert and
  visual modes; `"+y`/`"*y` and `"+p`/`"*p` in normal and visual modes.
- Clipboard failures are notifications; they never block editing.

## IME limitation

Ebitengine exposes committed text only; there is no stable preedit or candidate
window API. The editor accepts committed UTF-8 text and preserves unknown runes.
Kana input does not depend on an OS IME.

## Capture

The capture used PICO-8 0.2.7 at 4× scale on Linux. Keys were sent with
`xdotool` inside an Xvfb container; the editor was saved with `Ctrl+S` and the
resulting `.p8` file was compared with the screenshots. See
[`testdata/pico8/input/README.md`](../testdata/pico8/input/README.md) for the
manifest.
