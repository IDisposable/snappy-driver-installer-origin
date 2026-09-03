# SDIO Go rewrite

A rewrite of the Snappy Driver Installer Origin engine in Go, replacing the
C++Builder/VCL codebase under `../source`. Developed on branch `go-rewrite`.

## Scope

- Core engine: hardware detection, driver-pack indexing, matching, and
  install logic. Ported module by module from `../source/*.cpp`.
- A new front end (TUI or plain CLI) will replace the VCL GUI
  (`gui.cpp`/`draw.cpp`/`theme*.cpp`) entirely rather than porting it.
  Matching the old GUI's look is explicitly not a goal.
- Windows-only target (`GOOS=windows`). 7-Zip and torrent support use
  existing Go libraries rather than porting `project/7zip` and the
  bundled libtorrent glue.

## Hard constraint: on-disk compatibility

Users must not have to rebuild their existing driver-pack collection or
lose existing config/state files. Concretely:

- **`sdio.cfg`**: the original engine used colon-glued, underscored
  switches (`-drp_dir:value`). This rewrite uses the standard library
  `flag` package with idiomatic syntax (`-drp-dir=value`) for actual
  command-line parsing, but `Settings.LoadFile` translates an existing
  file's old-style switches on read (`internal/settings/compat.go`), so
  nothing needs to be hand-edited. `Settings.Save` always writes the new
  syntax.
- **Filter bits**: `-filters:N` is persisted as a raw integer. Its bit
  positions in `internal/settings/filters.go` intentionally match the
  original's GUI-menu-item-ID-based numbering rather than being cleaned
  up, so existing values decode correctly.
- **Binary index files** (`indexes/**/*.bin`): these use an `"SDW"` +
  LZMA container format (see below). Reading and writing them must stay
  byte-compatible once `indexing.cpp` is ported, so existing indexed
  driver packs don't need to be rebuilt. State snapshots (`logs/*.snp`)
  use the same container but live under the log directory, which is not
  required to match - `model.cpp`'s State::save/load can be freely
  redesigned.

### The SDW container format

Reverse-engineered by hex-dumping real `.snp` and `.bin` files from a
running installation:

```
offset 0:  "SDW"        3 bytes, magic
offset 3:  type byte     1 byte  (0x02 seen in .snp, 0x05 seen in .bin)
offset 4:  int32 LE      4 bytes (1 in .snp, 2 in .bin; meaning unconfirmed)
offset 8:  LZMA props    5 bytes (lc/lp/pb byte + 4-byte dictionary size LE)
offset 13: uncompressed  8 bytes LE
           size
offset 21: LZMA payload  raw LZMA1 stream, length known from the header
```

The 13-byte block at offset 8 is the classic LZMA-alone header; the
`"SDW"` + type + int32 prefix is this project's own container around it,
written by `encode()`/`decode()` in `common.cpp` via the 7-Zip SDK's
`Lzma86_Encode`/`Lzma86_Decode`. Not yet confirmed: the exact meaning of
the type byte and the int32 that follows it, or whether this layout holds
across all `.bin`/`.snp` variants. Verify against more samples before
finalizing a reader/writer. Candidate library: `github.com/ulikunitz/xz/lzma`
(supports raw LZMA1 with explicit properties/dict-size/uncompressed-size).

## Module port status

Ported bottom-up by dependency; each module is an idiomatic redesign, not
a mechanical line-by-line translation, with its own tests.

| Source module | Go package | Status | Notes |
|---|---|---|---|
| `common.cpp` | `internal/common` | Done | `Version`, `BytesToStr`. Dropped C-string buffer classes (unneeded in Go) and a stale `year>2015` date-validity cutoff. |
| `logging.cpp` | `internal/logging` | Done | `zerolog`-based `Logger`; verbosity bitmask collapsed to one level threshold. `Timers` uses `time.Time`/`time.Duration`. Crash/exception handlers not ported (no Go equivalent need). |
| `settings.cpp` | `internal/settings` | Done | `flag.FlagSet`-based parsing; legacy cfg syntax supported on read (see above). GUI presentation fields (theme, scale, window geometry, hint delay, license, expert mode) dropped. |
| `baseboard.cpp` | `internal/hardware` | Done | Raw COM/WMI calls replaced with `github.com/yusufpapurcu/wmi`. Verified against a real machine via `scripts/test-windows.sh`. |
| `enum.cpp`: `WinVersions`, `State::getsysinfo_fast`, `State::isnotebook_a` | `internal/hardware` | Done | `GetSysInfoFast()` (battery/monitors/OS version/env) and `IsLaptop()`. Windows version read from the registry instead of the manifest-gated `GetVersionEx`. Verified against a real machine. |
| `enum.cpp`: `Device`, `Driver`, rest of `State` (SetupAPI device enumeration, registry-based installed-driver lookup, `.inf` scanning) | `internal/hardware` or `internal/enum` (planned) | Not started | The largest remaining piece of hardware detection. |
| `system.cpp` (`SystemImp`, `FilemonImp`) | - | Not started (as-needed) | Mostly thin OS-utility wrappers (file/dir ops, restore points, process launch) that map directly to Go stdlib; being pulled in incrementally as later modules need each piece rather than ported as one grab-bag class. |
| `indexing.cpp`, `matcher.cpp` | - | Not started | Driver-pack indexing and hardware-to-driver matching; the SDW/LZMA binary format compatibility work lands here (indexes/**/*.bin only - not logs/*.snp). |
| `manager.cpp` | - | Not started | Orchestration. |
| `install.cpp` | - | Not started | Driver installation. |
| `script.cpp` | - | Not started | Driver-pack script format. |
| `update.cpp` | - | Not started | Planned: `github.com/anacrolix/torrent` instead of porting the libtorrent glue. |
| `gui.cpp`, `draw.cpp`, `theme*.cpp`, `welcome.cpp`, `usbwizard.cpp`, `license.cpp` | `cmd/sdi` + TUI (planned) | Not started | Replaced by a Bubble Tea TUI, not ported. |

## Verifying changes

From `go/`:

```sh
go build ./...
go vet ./...
go test ./...
GOOS=windows GOARCH=amd64 go build ./...   # this is a Windows-only app
```
