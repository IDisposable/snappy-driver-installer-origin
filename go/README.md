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

Implemented in `internal/sdwfile`. Confirmed byte-for-byte against every
index file (130+, both filter modes) and every state snapshot (9 files,
two different machines) in a real installation - see that package's
tests.

```
offset 0:  "SDW"           3 bytes, magic
offset 3:  format version  4 bytes, int32 LE (e.g. 0x205 for .bin files)
offset 7:  Lzma86 mode     1 byte (0 = none, 1 = x86 BCJ filter applied)
offset 8:  LZMA props      5 bytes (lc/lp/pb byte + 4-byte dictionary size LE)
offset 13: uncompressed    8 bytes LE
           size
offset 21: LZMA payload    raw LZMA1 stream, length known from the header
```

The block at offset 7 (mode byte + 13-byte LZMA-alone header) is exactly
the 7-Zip SDK's `Lzma86_Encode` output format; the `"SDW"` + version
prefix is this project's own container around it, written by
`encode()`/`decode()` in `common.cpp`. Two things only showed up against
real files, not from reading the source: the mode byte is easy to miss
(skipping it makes an LZMA-alone reader misparse it as the first
properties byte, producing a nonsensical dictionary size), and about 45%
of real index files use mode 1 (x86 BCJ filter) - `internal/sdwfile/
bcjx86.go` ports the inverse filter directly from
`external/SevenZ/build/C/Bra86.c` (bundled in this repo, public domain).
Library used for the LZMA-alone layer: `github.com/ulikunitz/xz/lzma`.

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
| `enum.cpp`: `Device` (SetupAPI device enumeration) | `internal/hardware` | Done | `ScanDevices()` via `x/sys/windows`' typed SetupAPI/CfgMgr32 wrappers, which decode `REG_MULTI_SZ` properties straight to `[]string`. Verified against a real machine: found 375 present devices, spot-checked one against the reference log. |
| `enum.cpp`: `Driver` registry-reading half (`calc_dev_pos`, the registry constructor) | `internal/hardware` | Done | `InstalledDriver`, `OpenInstalledDriver`, `MatchDeviceID`. Reuses `indexing.ParseDate`/`ParseVersionNumber` for `DriverDate`/`DriverVersion`, the same functions used for `.inf` fields. Verified against 5 real installed drivers on a real machine. |
| `enum.cpp`: `Driver::scaninf` (matching an installed driver back to a driver pack's `.inf`) | - | Not started | Needs the not-yet-built `Driverpack`/inf-cache orchestration in `genindex`. |
| `system.cpp` (`SystemImp`, `FilemonImp`) | - | Not started (as-needed) | Mostly thin OS-utility wrappers (file/dir ops, restore points, process launch) that map directly to Go stdlib; being pulled in incrementally as later modules need each piece rather than ported as one grab-bag class. |
| `indexing.cpp`: on-disk container format (`checkindex`/`loadindex`/`saveindex`) | `internal/sdwfile` | Done | `Decode`/`Encode` for the `"SDW"` + Lzma86 container (see above), including the x86 BCJ filter. This is the byte-compatibility-critical piece; verified against every real index file and snapshot available. |
| `indexing.cpp`: record layout (`data_inffile_t`, `data_manufacturer_t`, `data_desc_t`, `data_HWID_t`, `Txt`, `Hashtable`) | `internal/indexing` | Done | `DecodeIndex`/`EncodeIndex` parse `sdwfile`'s decompressed payload into structured records via a generic `readBlock[T]`/`writeBlock[T]` (replacing `loadable_vector<T>`). Verified against every index file in a real installation (230+): each decodes with zero trailing bytes and correctly-resolved hardware ID strings. |
| `indexing.cpp`: `.inf` tokenizer (`Parser`) | `internal/indexing` | Done | `InfParser`: `ParseItem`/`ParseField` plus `ParseNumber`/`ParseDate`/`ParseVersionNumber`/`ParseHexByte`. Verified against real `.inf` files (Windows inbox and a real driver pack). |
| `indexing.cpp`: section discovery, `[Strings]`/`[Version]`/`[Manufacturer]` parsing | `internal/indexing` | Done | `DiscoverSections`, `ParseStrings`, `ParseVersionSection`, `ParseManufacturers`. |
| `indexing.cpp`: install-section resolution (the `.nt`/bare/decoration/336-suffix fallback chain) | `internal/indexing` + `internal/matcher` (`OSDecorations`, matcher.cpp's `nts[]` table) | Done | `ResolveManufacturerSection`. First content of the new `matcher` package - pulled in early because indexing needs its decoration table, not because matching itself is ported. |
| `indexing.cpp`: `.7z` archive reading (`genindex`'s `SzArEx_*` calls) | `internal/archive` (`github.com/bodgit/sevenzip`) | Done | Just the extraction primitive; `genindex`'s threading and orchestration aren't ported. |
| **Full read-side pipeline, end-to-end** | `internal/archive` + `internal/indexing` + `internal/matcher` | Verified | Real driver-pack `.7z` -> extracted `.inf` -> sections -> strings -> version -> manufacturer -> resolved install section + hardware IDs, checked against a real driver pack's actual content (`DP_Ports_SDIO01_26083.7z`), both from a trimmed fixture and from live-extracted archive bytes. |
| `matcher.cpp`: `Hwidmatch`'s `getdrp_*` field navigation (HWID -> Desc -> Manufacturer -> InfFile) | `internal/indexing` (`Driverpack`) | Done | Wraps an already-decoded `Index` with the same join Hwidmatch's getters perform, without the full `Driverpack`/`Collection` object graph (which also owns `genindex` orchestration - not ported). Verified against every HWID entry (6.6M+) across every real index file (219) with no panics, plus a deep field-by-field check against the known `dtport.inf` device, including the two-section-variant (`DtHw`/`DtHw.NTamd64`) case. |
| `indexing.cpp`: `Hashtable::Find`/`FindNext`/`AddItem` | `internal/indexing` | Done | Completes the on-disk `Hashtable` type. Verified by rebuilding a real driver-pack index's hash table from scratch and confirming 200 real hardware IDs all resolve back to themselves. |
| `indexing.cpp`: `.cat` catalog file parsing (`findosattr`, `Driverpack::parsecat`) | `internal/indexing` | Done | `FindOSAttr`, `IsValidCat` (the latter also covers `Driver::isvalidcat` in enum.cpp and `Hwidmatch::isvalidcat` in matcher.cpp, which are byte-identical). Not a PKCS#7/ASN.1 parse - a raw byte scan for a UTF-16LE "OSAtt" marker, confirmed against a real `.cat` file's exact byte layout. |
| `indexing.cpp`: `genindex` threading/orchestration, `Txt` interning to build new `.bin` files (`Driverpack::saveindex`'s write path) | - | Not started | The read side (`DecodeIndex`) is done; this is the write side, to build/update a collection from scratch. |
| `matcher.cpp`/`enum.cpp`: driver-ranking scoring (`calc_decorscore`, `calc_markerscore`, `genmarker`, `calc_score`, `calc_identifierscore`, `calc_secttype`) | `internal/matcher` | Done | `DecorationScore`, `MarkerScore`, `NotebookOEMMarker`, `Score`, `IdentifierScore`, `SectionDecorationIndex`, plus 4 more extracted tables (`nts_version/build/arch/score`, `markers[]`, `Filter_1..22`). |
| `matcher.cpp`: validity/comparison primitives from `Hwidmatch::calc_altsectscore`/`cmp`/`isdup`/`isdrivervalid` | `internal/matcher` | Done (primitives only) | `IsValidVer`, `IsBlacklisted` (+ the Realtek blacklist constants), `IsValidUSB3Hub` (+ the Intel gen2/gen4 hub-ID tables and `IntelPathUsesSDIPrefix`), `CalcNotebookValid`, `CmpUnsigned`, and a `Result` type (`Cmp`/`IsDup`/`IsDriverValid`) standing in for `Hwidmatch`'s scoring fields. |
| `matcher.cpp`: full match orchestration (`Hwidmatch`/`Devicematch`/`Matcher` construction, `calc_status`, `calc_altsectscore`'s section-iteration and control flow, `cmpnames`) | - | Not started | Needs the not-yet-ported `Driverpack`/`Collection` object graph (which section list a driver pack exposes, `getdrp_*` field access) to wire the primitives above together; `Settings.flags&FLAG_FILTERSP` also needs threading through. |
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
