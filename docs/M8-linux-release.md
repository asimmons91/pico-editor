# M8 — Linux release

**Status:** Draft
**Depends on:** M7
**Blocks:** none
**Roadmap:** [ROADMAP.md](ROADMAP.md)
**Estimate:** 1–2 weeks

## Outcome

A tagged `v1.0.0` Linux release that a new user can install from the release
page, run without a Go/Node/Rust toolchain, and use to open, edit, and save a
real cart. Artifacts are reproducible, checksummed, documented, and covered by
a clean-machine smoke test.

**Demo:** On a clean Ubuntu LTS machine, download the AppImage, make it
executable, run it, open a cart, edit, save, use completion, and quit. Repeat
with the `tar.gz` on Fedora. The release page lists checksums, install steps,
known limitations, the MIT license, and the PICO-8 trademark disclaimer.

## Scope

### In scope

- `goreleaser` for Linux `amd64` and `arm64` tar.gz archives and checksums.
- AppImage packaging for both architectures.
- App icon, desktop entry, version metadata, and release notes.
- Install/run documentation for common distributions.
- Clean-machine smoke tests on Ubuntu and Fedora, X11 and Wayland.
- `v1.0.0` tag and GitHub release.
- License and trademark notices in every artifact and on the release page.

### Out of scope

- macOS and Windows packaging.
- `.deb`, `.rpm`, Snap, Flatpak, Homebrew, AUR, and distro repositories.
- Auto-update and crash reporting.
- Code signing and reproducible-build attestations beyond checksums (optional
  attestation is allowed if it adds no release risk).
- Run-cart and full file watching, which remain post-V1.

## Deliverables

1. `.goreleaser.yaml` for builds, archives, checksums, changelog, and release.
2. `scripts/build-appimage.sh` (or an equivalent pinned tool) producing
   AppImages for `amd64` and `arm64`.
3. `assets/icon.png` (256×256), `assets/pico-editor.desktop`, and an `AppRun`
   entry that starts the binary.
4. `.github/workflows/release.yml` triggered by a `v*` tag.
5. `docs/install.md` covering AppImage, tar.gz, dependencies, Wayland/X11, and
   first run.
6. `docs/release-checklist.md` with the smoke-test matrix and sign-off.
7. Release notes for `v1.0.0`.
8. A published GitHub release with checksums and artifacts.

## Detailed requirements

### 1. Versioning

- Semantic versioning; V1 starts at `v1.0.0`.
- `internal/version.Version` is set from the tag without the leading `v`.
  `Commit` is the short SHA; `BuildDate` is RFC 3339 UTC.
- `pico-editor --version` prints all three values.
- The window title shows `pico-editor <version>`; the status bar/about dialog
  shows the same.
- Development builds report `dev` and must never be published.

### 2. Builds

- Targets: `linux/amd64` and `linux/arm64`.
- `CGO_ENABLED=0`, `-trimpath`, and the Go version from `mise.toml`.
- Set `SOURCE_DATE_EPOCH` from the commit timestamp for reproducible archives.
- Strip symbols only if stack traces remain useful; prefer keeping them and
  compressing the binary.
- The binary must start on a machine with no Go toolchain, no Node, and no Rust
  runtime.
- Runtime system libraries (OpenGL, X11/Wayland, ALSA/Pulse) are documented
  dependencies and are not statically bundled in V1. The binary loads them
  dynamically.

### 3. goreleaser

```yaml
version: 2
builds:
  - id: pico-editor
    main: ./cmd/pico-editor
    env: [CGO_ENABLED=0]
    flags: [-trimpath]
    ldflags:
      - -s -w
      - -X github.com/asimmons91/pico-editor/internal/version.Version={{.Version}}
      - -X github.com/asimmons91/pico-editor/internal/version.Commit={{.ShortCommit}}
      - -X github.com/asimmons91/pico-editor/internal/version.BuildDate={{.Date}}
archives:
  - formats: [tar.gz]
    files:
      - LICENSE
      - NOTICE
      - README.md
      - docs/install.md
      - assets/pico-editor.desktop
      - assets/icon.png
checksum:
  name_template: checksums.txt
changelog:
  use: git
release:
  draft: false
  prerelease: auto
```

Adjust field names to the current goreleaser schema; the pinned version in
`mise.toml` is authoritative. `goreleaser check` runs in CI.

### 4. AppImage

Goreleaser does not build AppImages natively; use a pinned
`appimagetool`/`linuxdeploy` step in `scripts/build-appimage.sh`.

AppDir contents:

```text
pico-editor.AppDir/
  AppRun                 -> usr/bin/pico-editor
  pico-editor.desktop
  pico-editor.png        (256×256)
  usr/bin/pico-editor
  usr/share/doc/pico-editor/{LICENSE,NOTICE,README.md,install.md}
```

- `AppRun` sets `APPDIR` and execs `usr/bin/pico-editor "$@"`.
- The desktop entry uses `Name=pico-editor`, `Categories=Development;TextEditor;`,
  `Terminal=false`, and `Icon=pico-editor`.
- Build one AppImage per architecture on the matching runner (or with a
  cross-build plus `appimagetool --architecture`).
- Test both FUSE execution and `--appimage-extract-and-run`; document the
  fallback for systems without FUSE.
- AppImages do not bundle Mesa, X11, Wayland, or audio servers; document the
  system dependencies.

### 5. Smoke tests

Run on clean, current Ubuntu LTS and Fedora releases:

| Check | Ubuntu | Fedora |
| --- | --- | --- |
| AppImage starts under X11 | required | required |
| AppImage starts under Wayland | required | required |
| tar.gz starts with no Go/Node/Rust | required | required |
| Open a real cart, edit, save | required | required |
| Semantic highlighting and diagnostics | required | required |
| Completion, hover, definition | required | required |
| P8SCII Shift+letter and picker | required | required |
| Clipboard copy/paste | required | required |
| Crash recovery after `kill -9` | required | required |
| `--version` matches the tag | required | required |

- Use a disposable VM or container with a desktop session for GUI checks.
  Headless `xvfb` is acceptable for startup and screenshots, but at least one
  real Wayland session is required.
- Record OS version, GPU/llvmpipe, session type, and artifact checksum in
  `docs/release-checklist.md`.
- A failure on one distro blocks the release until fixed or explicitly
  documented as a supported-limitation decision.

### 6. Install documentation

`docs/install.md` covers:

- AppImage: download, `chmod +x`, run, FUSE fallback.
- tar.gz: extract, run `./pico-editor`, optional `~/.local/bin` install.
- Dependencies:
  - Debian/Ubuntu: `libgl1`, `libx11-6`, `libxcursor1`, `libxrandr2`,
    `libxinerama1`, `libxi6`, `libasound2t64` (name varies by release),
  - Fedora: `mesa-libGL`, `libX11`, `libXcursor`, `libXrandr`, `libXinerama`,
    `libXi`, `alsa-lib`,
  - Wayland users also need the XWayland compatibility layer for X11-only
    backends, or a native Wayland session if supported.
- First run: open a folder or cart, `:help`, config path.
- Where logs and recovery files live.
- Known limitations: no `.p8.png`, committed-text-only IME, no macOS/Windows,
  no full file watching, no run-cart.
- Troubleshooting pointer.

### 7. Release workflow

`.github/workflows/release.yml`:

1. Trigger on tags matching `v*`.
2. Check out the tag and install mise.
3. Run `mise run check`.
4. Run `goreleaser check`.
5. Build archives and checksums with `goreleaser release --clean`.
6. Build AppImages for both architectures.
7. Upload AppImages to the GitHub release.
8. Publish release notes with:
   - highlights,
   - install links,
   - checksums,
   - known limitations,
   - license and trademark disclaimer.

- The workflow must use the same Go and lint pins as CI.
- Do not publish if `mise run check` fails or if the tag does not match
  `internal/version`.
- Keep the release commit immutable after tagging. Fixes require `v1.0.1`.

### 8. Icon and branding

- `assets/icon.png` is an original 256×256 image in the PICO-8 palette. Do not
  use the PICO-8 logo or any Lexaloffle trademark.
- The name is `pico-editor`; the README and release notes state that PICO-8 is
  a trademark of Lexaloffle Games and that this project is not affiliated.
- Include the icon in the AppImage, desktop entry, README, and release page.

### 9. License and notices

- Ship `LICENSE` (MIT) and `NOTICE` in both artifacts.
- `NOTICE` includes the CC-0 PICO-8 font attribution, the trademark
  disclaimer, and the statement that no Rune code is included.
- The release page repeats the disclaimer.
- A missing notice file fails the release checklist.

### 10. Release checklist and rollback

`docs/release-checklist.md`:

- [ ] `mise run check` green on the release commit.
- [ ] `goreleaser check` green.
- [ ] Tag matches `--version` for both architectures.
- [ ] Checksums match downloaded artifacts.
- [ ] Ubuntu and Fedora smoke matrices complete.
- [ ] AppImage FUSE and extract-and-run both work.
- [ ] License/NOTICE present in every artifact.
- [ ] Release notes reviewed for known limitations and disclaimer.
- [ ] Post-release download test from a logged-out browser.

Rollback:

- If an artifact is broken, mark the release as a pre-release or delete it,
  publish `v1.0.1`, and post a short notice. Never retag `v1.0.0`.
- If only docs are wrong, update the release notes without rebuilding.

## Architecture and files

```text
.goreleaser.yaml
.github/workflows/release.yml
assets/
  icon.png
  pico-editor.desktop
scripts/
  build-appimage.sh
docs/
  install.md
  release-checklist.md
docs/releases/v1.0.0.md
```

## Interfaces and state

There is no runtime interface specific to M8. The release injects:

```go
internal/version.Version   // "1.0.0"
internal/version.Commit    // short SHA
internal/version.BuildDate // RFC 3339 UTC
```

## Error handling

- A failed build or check aborts the release; do not upload partial artifacts.
- A missing architecture or AppImage fails the release checklist.
- A checksum mismatch removes the artifact and blocks publication.
- A broken release is superseded by a patch release; tags are never moved.
- A failed smoke test blocks the tag unless the failure is documented as a
  supported limitation and approved before release.

## Testing

### Automated

- `goreleaser build --snapshot --clean` succeeds for both architectures.
- `scripts/build-appimage.sh` is shellchecked and runs in CI for `amd64`.
- Archive contents include the binary, LICENSE, NOTICE, README, install docs,
  desktop file, and icon.
- `pico-editor --version` matches the tag.
- Checksums verify.

### Manual

- Full smoke matrix from §5.
- Download each artifact from the published release and verify its checksum.
- Run as a non-root user with a fresh home directory; no pre-existing config or
  recovery files.
- Run with `HOME` unwritable to verify a clear error, not a crash.

## Exit criteria

- [ ] A new user can install and open a cart using only the release docs.
- [ ] AppImage and tar.gz both start without a Go/Node/Rust runtime.
- [ ] Both architectures build and pass smoke tests.
- [ ] CI builds and tests the release commit.
- [ ] Checksums, license, and notices are published.
- [ ] The release page states known limitations and the trademark disclaimer.
- [ ] `v1.0.0` is tagged and the GitHub release is public.
- [ ] `mise run check` is green on the tag.

## Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Ebitengine runtime libraries are missing on a clean machine | Document dependencies per distro; test AppImage and tar.gz on clean Ubuntu/Fedora. |
| AppImage FUSE is unavailable | Test `--appimage-extract-and-run`; document it prominently. |
| Wayland backend differences | Test a real Wayland session; document XWayland fallback and any known issue. |
| Cross-architecture build breaks | Build on native runners; pin goreleaser and appimagetool; smoke-test arm64. |
| Artifact contents drift | Automated archive-content test and release checklist. |
| Broken release is retagged | Never move a tag; publish a patch release. |
| Trademark or license omission | NOTICE in every artifact; release-page disclaimer; checklist sign-off. |
