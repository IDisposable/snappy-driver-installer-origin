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
go/scripts/release.sh [version]
```

This runs the test suite, generates the manifest/version resource with
[go-winres](https://github.com/tc-hib/go-winres) (installing it first
if needed), and cross-compiles `go/sdigo.exe`. `version` defaults to
`git-tag`, which go-winres resolves from the nearest git tag - pass one
explicitly (e.g. `0.2.0`) if the checkout doesn't have the right tag.

The generated `rsrc_windows_amd64.syso` is gitignored, not committed:
embedding it changes what a locally built dev binary needs to launch,
which breaks the WSL PE interop technique used throughout development
(see the top-level README). Plain `go build ./...` during development
therefore produces a binary without the embedded manifest/version
resource; only a release build goes through `scripts/release.sh`.

`.github/workflows/release.yml` runs this same script on a `v*.*.*` tag
push and attaches the resulting `sdigo.exe` to a GitHub Release. Not
yet run for real - review before pushing the first tag.

## Driver-pack bootstrap torrent

Not yet implemented. The plan is to host the bootstrap `.torrent` file
(what a fresh install's Welcome screen downloads the initial index
catalog/driver packs from) on this project's own GitHub release page,
independent of local-cache/offline operation. The original project's
`../trackers.txt` is a maintained list of public BitTorrent tracker
announce URLs used when building its own update torrent - copied here
as `release/trackers.txt` for whichever tool ends up generating ours,
since tracker health matters for peer discovery and needs occasional
rotation the same way upstream's does. Nothing in this repo currently
creates a `.torrent` file; `internal/update` only consumes one already
built elsewhere.
