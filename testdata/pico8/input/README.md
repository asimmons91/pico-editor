# PICO-8 0.2.7 input capture

These files document the one-time manual capture used by M6. PICO-8 is a
proprietary binary and is not a CI dependency; CI tests the derived Go tables,
not PICO-8 behavior.

## Method

- PICO-8 0.2.7, Linux, 4x scale, inside an Xvfb container.
- The editor was driven with `xdotool`; files were saved with `Ctrl+S`.
- Each screenshot is a 512x512 window capture; each `*-capture.txt` is the
  `__lua__` body of the saved cart after entering the listed keys.
- A reviewer compares the screenshots and capture files with the tables in
  `internal/input/p8scii.go` and `docs/INPUT.md`.

## Manifest

See `manifest.json`. `keys` is the raw key sequence; `expected` is the saved
canonical P8SCII text or the observed status label. Shift+letter entries are
one per line in the order A-Z.

## Files

- `shift-letters.png` — Shift+A through Shift+Z, one glyph per line.
- `hiragana-romaji.png` — the Hiragana romaji list.
- `katakana-romaji.png` — the Katakana romaji list.
- `hiragana-ka.png` — `Ctrl+J`, `k`, `a`; status `HIRAGANA MODE (CTRL-J)`.
- `katakana-mode-status.png` — `Ctrl+K`; status `KATAKANA MODE (CTRL-K)`.
- `puny-mode-status.png` — `Ctrl+P`; status `PUNY FONT MODE (CTRL-P)`.
- `hiragana-keys.txt`, `katakana-keys.txt` — the typed romaji lines.
- `hiragana-capture.txt`, `katakana-capture.txt` — the saved code body.
