# Release process

`cmd/sdigo` is the single binary intended for release (WinGet
packaging). Its Windows app manifest (`cmd/sdigo/winres/winres.json`)
requests no elevation at launch (`asInvoker`) - scanning and browsing
results needs no admin rights, only an actual driver install does, and
that's requested on demand (see `internal/install`'s
`IsElevated`/`RelaunchElevated`, used by `cmd/sdigo`'s confirm-install
step).

## Building a release

```sh
./scripts/release.sh [version]
```

This runs the test suite, generates the manifest/version resource with
[go-winres](https://github.com/tc-hib/go-winres) (installing it first
if needed), and cross-compiles `sdigo.exe`. `version` defaults to
`git-tag`, which go-winres resolves from the nearest git tag - pass one
explicitly (e.g. `0.2.0`) if the checkout doesn't have the right tag.

The generated `rsrc_windows_amd64.syso` is gitignored, not committed:
embedding it changes what a locally built dev binary needs to launch,
which breaks the WSL PE interop technique used throughout development
(see the top-level README). Plain `go build ./...` during development
therefore produces a binary without the embedded manifest/version
resource; only a release build goes through `./scripts/release.sh`.

`.github/workflows/release.yml` runs this same script on a `v*.*.*` tag
push and attaches the resulting `sdigo.exe` to a GitHub Release.

`.github/workflows/verify.yaml` runs on pull requests, pushes to `main` or
`go-rewrite`, and manual dispatch. It runs the test suite and vet, checks the
release script syntax and embedded torrent seed, cross-builds the Windows
executable, computes a SHA-256 checksum, and uploads the verification build.
It does not launch the TUI or run the release script.

`.github/workflows/verify-windows.yaml` is manual-only and runs on a hosted
Windows machine. It runs Windows-native tests and vet, builds `sdigo.exe`,
runs `hwdump`, and verifies headless USB copy. Its optional `run_scan` input
runs the real `-nogui -json -device-list` path against the hosted Windows
hardware and the mutable torrent seed. It does not install drivers, reboot,
or automate the interactive TUI by default. A separate opt-in confirmation
input can attempt driver installation on the ephemeral hosted runner. This
may request elevation, may find no actionable driver, and may change the
runner while the job runs.

## Pre-tag checklist

Before pushing a release tag:

- Run `go test ./...` and confirm the command exits successfully.
- Run `go vet ./...` and confirm the command exits successfully.
- Run `GOOS=windows GOARCH=amd64 go build ./...`.
- Build `sdigo.exe` with `scripts/release.sh` and inspect the generated
  manifest and version information.
- Confirm the executable contains the current embedded torrent metadata.
- Run `sdigo.exe -nogui -json` on a supported Windows machine.
- Run `sdigo.exe -nogui -device-list=<path>` and the same command with
  `-json`; inspect both output files.
- Run `sdigo.exe -nogui -usb=<destination>` against a test destination and
  confirm the executable, driver packs, and indexes copy correctly.
- Open the TUI and verify `d`, `u`, `o`, `f`, `i`, `?`, and `q` actions.
- Verify elevation handoff and driver installation only on a disposable test
  machine with a restore point available.
- Verify `-finish-reboot` only after a successful test installation.
- Compute a SHA-256 checksum for `sdigo.exe` before publishing it.

Do not run the TUI through a pipe or redirected standard input. The TUI needs
a real interactive terminal.

## Driver-pack bootstrap torrent

The 1.36 MB `seed/SDIO_Update.torrent` metadata is embedded in the
executable. It describes the mutable driver and index collection, but it
does not contain the driver-pack payloads.

The embedded metadata is the default. `-torrent-file=*` fetches the
current metadata from the mutable `main` seed. Another `-torrent-file`
value accepts a
user-selected local file, magnet URI, or HTTPS URL and overrides both
defaults.

The project may update the seed and tracker list without rebuilding older
executables. Older executables keep their embedded metadata unless the
user selects `-torrent-file=*` or another source.
