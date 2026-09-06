# SDIO Go rewrite

Snappy Driver Installer: Go Forth - a Go rewrite of the Snappy Driver
Installer Origin engine, replacing the C++Builder/VCL codebase under
`../source`. Developed on branch `go-rewrite`.

## What this is

- Core engine (`internal/*`): hardware detection, driver-pack
  indexing, matching, and install logic.
- `cmd/sdigo`: the single release binary - an interactive TUI by
  default, a plain-text or JSON report with `-nogui`, `cleandrivers` to
  remove superseded driver-pack files, and `hwdump`/`torrenttest` as
  dev/diagnostic subcommands.
- Windows-only target (`GOOS=windows`). 7-Zip and torrent support use
  existing Go libraries rather than porting `project/7zip` and the
  bundled libtorrent glue.
- A new front end (TUI or plain CLI) replaces the VCL GUI
  (`gui.cpp`/`draw.cpp`/`theme*.cpp`) entirely rather than porting it;
  matching the old GUI's look is explicitly not a goal.

See [docs/COMPATIBILITY.md](docs/COMPATIBILITY.md) for the current
on-disk data contract (filter bit layout and index/snapshot file format), and
[docs/PORTING_NOTES.md](docs/PORTING_NOTES.md) for module-by-module
traceability back to the original C++ source.

## Building and running

```sh
go build ./...
go vet ./...
go test ./...
GOOS=windows GOARCH=amd64 go build ./...   # this is a Windows-only app
```

Two scripts build and run `cmd/sdigo`, forwarding any arguments as-is:

- `scripts/run-windows.sh [args...]` - from WSL: cross-compiles and
  runs the result directly via PE interop, no Windows-side Go
  toolchain needed. This is how everything in this rewrite has been
  verified against a real machine during development.
- `scripts/run-windows.cmd [args...]` - from an actual Windows command
  prompt: builds natively, so it needs Go installed on Windows.

```sh
scripts/run-windows.sh -nogui -drp-dir=D:\drivers -index-dir=D:\indexes
scripts/run-windows.sh hwdump
```
```bat
scripts\run-windows.cmd -nogui -drp-dir=D:\drivers -index-dir=D:\indexes
```

The default update source is the embedded `seed/SDIO_Update.torrent`
metadata. Use `-torrent-file=*` to fetch the current mutable seed from
the project `main` branch. Use `-torrent-file=<path-or-URL>` to select a
local torrent, magnet URI, or HTTPS torrent URL. A user-selected source
overrides both built-in choices.

Settings use the current `sdigo.cfg` syntax written by the program. The
old SDIO configuration format is not supported.

`-device-list=<path>` writes a tab-separated device report after scanning.
Add `-device-list-json` to write the structured JSON form. `-finish-reboot`
requests a Windows reboot after a successful `-install` run.

See [docs/RELEASE.md](docs/RELEASE.md) for building the release binary.
