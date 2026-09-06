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
