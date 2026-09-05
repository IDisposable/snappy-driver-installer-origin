# Reference Manual notes

Source: `docs/SDIO Reference Manual.pdf` (32 pages, dated 13 August 2026,
by Glenn Delahoy). The manual states it is "a work in progress" with
possible "knowledge gaps," so absence of a topic in the manual does not
prove the engine lacks it.

This file records what the manual says about engine behavior, and
cross-checks it against the Go rewrite as it stood when only
`go/internal/settings/flags.go`, `filters.go`, `settings.go`,
`compat.go`, `parse.go`, `persistence.go`, and `go/README.md` existed -
before `matcher.cpp`, `indexing.cpp`, `install.cpp`, and `update.cpp`
were ported. **Every "Not done" verdict below that cites one of those
four modules as the reason is now wrong** - see
`go/docs/PORTING_NOTES.md` for their real (mostly Done) status. A few
of the more consequential ones were corrected in place (marked below);
the rest still need a real re-pass against the manual, which this file
doesn't attempt - re-verifying ~30 manual sections against
now-much-larger Go packages is its own task, not a small fix.

Status labels used below:
- **Done** - Go behavior matches the manual.
- **Partial** - Go has some of the behavior, or stores the setting but
  does not yet act on it.
- **Not done** - Go has no equivalent yet. Treat any instance of this
  label citing indexing/matching/install/update as unverified (see
  the warning above) rather than trusting it at face value.
- **Not documented** - the Go rewrite has this, but the manual does not
  mention it at all.

## 1. Command-line switches

### Switches with no engine-setting behavior (out of scope for `internal/settings`)

These select a one-shot program action rather than a setting, and the
Go README already notes they belong to a future CLI dispatch layer, not
`Settings`:

| Switch | Manual meaning | Go status |
|---|---|---|
| `-?` | Show the help window | Not done (README says `-h`/`-help` will replace it) |
| `-script:<scriptfile> [options]` | Run a script; all args before it are dropped, following 9 args become `%1`-`%9`, `%0` is the script path | Not done - `script.cpp` not ported |
| `-cfg:<filename>` | Load configuration from the given file instead of `sdio.cfg` | Not done in `internal/settings` as a self-selecting switch, but `Settings.LoadFile` already takes an arbitrary path, so the underlying capability exists |
| `-7z` | Run a bundled 7-Zip command, e.g. `SDIO.exe -7z x DP_x.7z`; exit code 2 usually means "file not found" | Not done |
| `-install <hwid> <inffile>` | Install one driver directly by hardware ID and `.inf` path | Not done - `install.cpp` not ported |
| `-HWIDInstalled:<hwid>=<file>` | Undocumented beyond the signature line (no body text follows it in the manual) | Not done |
| `-save-installed-id[:<file>]` | Undocumented beyond the signature line | Not done |
| `-getdevicelist:<file>` | Write a text file with all installed devices and drivers | **Partial**: `Settings.DeviceListFilename` field and `-device-list=<file>` flag exist and the legacy `-getdevicelist:` syntax is translated in `compat.go`, but nothing writes the file yet (needs `enum.cpp`'s `Driver` port) |

### Persistent behavior flags (round-tripped through `sdio.cfg`)

These map 1:1 to `boolFlagDefs` entries with `persist: true` in
`flags.go`. All are **Done** for parsing/storage; the underlying engine
behavior they gate (update checking, restore points, virus warnings,
etc.) is not yet implemented since indexing/install/update are not
ported.

| Switch | Manual meaning |
|---|---|
| `-norestorepnt` | Do not create a system restore point |
| `-nostop` | Do not stop if creating a restore point fails |
| `-novirusalerts` | Suppress virus alerts |
| `-keepunpackedindex` | Prevents updating indexes for unpacked drivers |

The manual's Command Line Reference chapter documents `-nostop` and
`-novirusalerts` only in the Configuration File Reference chapter (they
are settable in `sdio.cfg`, not called out separately as CLI switches),
but the mechanism is the same either way.

`-checkupdates`, `-onlyupdates`, and `-torrentalerts` are also
`persist: true` in `flags.go` (ported from the original's flag enum),
but **the manual does not document any of these three as a user-facing
switch anywhere** (not in Command Line Reference, not in Configuration
File Reference). Flag as "not documented" - implemented ahead of the
manual, presumably matching original engine behavior in
`source/settings.cpp`, but unverifiable against this manual alone.

### One-shot switches (`persist: false`)

| Switch | Manual meaning | Go status |
|---|---|---|
| `-delextrainfs` | Delete unused `.inf` files after extracting | Done - `installflow.InstallOne` calls `install.RemoveExtraInfs` when set |
| `-nologfile` | Suppress creating a log file | Done (stored only - `cmd/sdigo` doesn't call `internal/logging` at all yet, so there's no log file to suppress either way) |
| `-nosnapshot` | Suppress creating a snapshot | Done (stored only - no `.snp` state-snapshot writer exists yet) |
| `-nostamp` | Create logs and snapshots without timestamps in the file name | Done (stored only, same as above) |
| `-reindex` | Force reindexing of all driver packs | Done (stored; indexing itself not ported) |
| `-index_hr` | Also write a human-readable (text) index | Done, registered as `-index-hr`; legacy `-index_hr` translated in `compat.go`. Stored only - the index write path isn't ported. |
| `-failsafe` | Disable indexing `WINDOWS\Inf` | Done (stored only) - not needed by `ScanInstalledInf`'s registry-scan port so far |
| `-disableinstall` | **Disables driver installation AND restore-point creation** | Done - `installflow.Run` skips restore-point creation when either `FlagDisableInstall` or `FlagNoRestorePoint` is set, and its help text in `flags.go` states both halves |
| `-keeptempfiles` | Do not delete extracted driver-pack files | Done (stored only) - nothing deletes extracted files either way, so there's nothing for this to prevent yet |
| `-preservecfg` | Do not overwrite `sdio.cfg` on exit | Done |

`-nogui`, `-autoinstall`, `-autoclose`, `-autoupdate` are implemented in
`flags.go` (`persist: false`) but, like the three flags above, **are
not documented anywhere in this manual** as command-line or cfg-file
switches. These look like ports of internal flags from the original
`settings.h` enum rather than manual-documented user switches.

### Torrent / update tuning switches

| Switch | Manual meaning | Go status |
|---|---|---|
| `-activetorrent:<num>` | Select active update torrent: 1 = app + driver-pack updates (default), 2 = driver-pack updates only, which refresh more often | Not done - `update.cpp` not ported |
| `-port:<num>` | Incoming torrent port, default 50171 | Not done; `compat.go` silently drops `-port:` from legacy cfg files |
| `-minport:<num>` / `-maxport:<num>` | Allowed outgoing torrent port range, default 0/0 = unrestricted | Not done; both dropped by `compat.go` |
| `-downlimit:<num>` / `-uplimit:<num>` | Torrent download/upload speed limit in KB/s, 0 = unlimited | Not done; dropped by `compat.go` |
| `-connections:<num>` | Max torrent connections, 0 = unlimited | Not done; dropped by `compat.go` |

All of these are intentionally dropped rather than erroring, per the
`legacyDroppedPrefixes` comment in `compat.go` ("torrent tuning not yet
ported"). Correct interim behavior; revisit once `update.cpp` lands
(README says `github.com/anacrolix/torrent` is the planned library).

### Virtual OS / architecture emulation

| Switch | Manual meaning | Go status |
|---|---|---|
| `-a:32` / `-a:64` | Emulate a 32-bit or 64-bit Windows environment | Partial - parsed into `VirtualArchType` (`-arch=32`/`-arch=64`, legacy exact-match translated in `compat.go`) and logged, but nothing in `internal/scan`/`internal/collection` reads it to actually match against a virtual architecture instead of the real detected one - the setting is stored, the emulation effect isn't implemented |
| `-v:<version>` | Emulate a given non-server Windows version. Documented codes: 50=2000, 51=XP, 52=XP64, 60=Vista, 61=7, 62=8, 63=8.1, 64=10 Tech Preview, 100=10, 110=11 | Partial - `VirtualOSVersion`'s code->name lookup is wired (`hardware.FindWindowsVersionName`, called from `virtualOSVersionValue.Set` in `parse.go`), but same as `-a:`: nothing in `internal/scan`/`internal/collection` reads `VirtualOSVersion`/`VirtualWindowsVersionName` to actually match against the virtual version instead of the real detected one |

### Snapshot / state replay

| Switch | Manual meaning | Go status |
|---|---|---|
| `-ls:<file>` | Load a snapshot instead of scanning the real PC | Done - `stateFileValue` sets `StateFile` and flips `StateMode` to `StateModeEmul`; legacy `-ls:` translated |

### Extraction directory

| Switch | Manual meaning | Go status |
|---|---|---|
| `-extractdir:<dir>` | Directory used to extract driver packs; default `%temp%\SDIO` | Done - `Settings.ExtractDirRaw`/`ExtractDir` (default `%TEMP%\SDIO`, matching the manual), `-extractdir` flag, legacy `-extractdir:` translated in `compat.go`; `installflow.InstallOne` extracts into it for real |

### Finish-command hooks

| Switch | Manual meaning | Go status |
|---|---|---|
| `-finish_cmd` | Command to run after driver installation completes | Done, ported to `-finish-cmd` |
| `-finishrb_cmd` | Command to run after install completes and a reboot is needed | Done, ported to `-finish-reboot-cmd` |
| `-finish_upd_cmd` | Command to run after driver-pack updates complete | Done, ported to `-finish-update-cmd` |

### Logging verbosity

| Switch | Manual meaning | Go status |
|---|---|---|
| `-verbose:<flags>` | Bitmask log detail level: 0x0001 ARGS, 0x0002 SYSINFO, 0x0004 DEVICES, 0x0008 MATCHER, 0x0010 MANAGER, 0x0020 DRP, 0x0040 TIMES, 0x0080 LOG_ERR, 0x0100 LOG_CON, 0x0200 LAGCOUNTER, 0x0400 DEVSYNC, 0x0800 BATCH, 0x1000 DEBUG, 0x2000 TORRENT | **Not done as documented.** `compat.go` explicitly drops legacy `-verbose:` on cfg read, and `go/README.md` says the "verbosity bitmask collapsed to one level threshold" in the `internal/logging` port. This is a deliberate simplification, not an oversight, but it means the fine-grained per-section log control the manual describes does not exist in the Go rewrite. Worth a design decision: keep the simplified single-level approach, or restore section-level control if users depend on filtering specific log sections. |

## 2. Filter categories

Manual's "Configuration File Reference" chapter documents `-filters:<flags>`
as a sum of the following bit values (page 21):

| Category | Manual value | Meaning per manual |
|---|---|---|
| Not Installed | 2 | Show devices where no driver is installed |
| Newer | 4 | Show drivers newer than installed |
| Current | 8 | Show drivers the same as installed |
| Older | 16 | Show drivers older than installed |
| Better Match | 32 | Show drivers ranked better than installed |
| Worse Match | 64 | Show drivers ranked worse than installed |
| Not installed (absent from driver packs) | 128 | Show devices with no driver found in any driver pack |
| Unknown | 256 | Show devices with an unknown driver installed |
| Standard | 512 | Show devices with a standard (generic) driver installed |
| Show Only Best | 1024 | Show only the best match per device |
| Show Duplicates | 2048 | Show duplicate drivers |
| Show Invalid | 4096 | Show drivers incompatible with the system |

**Status: Done, exactly.** `internal/settings/filters.go` reproduces
every bit value verbatim:
`FilterMissing=2, FilterNewer=4, FilterCurrent=8, FilterOld=16,
FilterBetter=32, FilterWorseRank=64, FilterNFMissing=128,
FilterNFUnknown=256, FilterNFStandard=512, FilterOne=1024, FilterDup=2048,
FilterInvalid=4096`. Only the Go constant names differ cosmetically
(`FilterOld` vs. manual's "Older", `FilterWorseRank` vs. "Worse Match").
This is deliberate per the file's own comment: bit positions must
round-trip unchanged for existing `sdio.cfg` files, since `-filters:N`
is stored as a raw sum.

Default filters: the manual's "Using SDIO" chapter says the default
filters are *Not installed* and *Better Match* (page 6), but the
screenshot on the same page shows four checkboxes ticked by default:
*Not Installed*, *Better match* (found-in-pack side), plus *Not
installed* and *Show only best* (absent/other side). `settings.go`'s
`DefaultFilters = FilterMissing | FilterBetter | FilterNFMissing |
FilterOne` matches the screenshot exactly, so the prose in the manual
undercounts its own screenshot. Go is Done here; note this only because
the manual's two descriptions of its own default disagree with each
other.

The manual also states in prose (page 7): "Newer is not always better.
If you want to be cautious, uncheck the *Newer* filter so you only
install missing and better matched drivers" - confirming *Newer* is
excluded from the shipped default, consistent with the bit-sum above
(2+32+128+1024 = 1186, no `Newer` bit).

Matching/ranking logic itself (what makes a driver "Better Match" vs.
"Worse Match" vs. "Current") is not documented anywhere in the manual
beyond the category names - the actual ranking algorithm lives in
`source/matcher.cpp`, not this manual. That module is **Not done** in
Go (`matcher.cpp` listed as not started in `go/README.md`).

## 3. Folder / file layout conventions

| Item | Manual says | Go status |
|---|---|---|
| `sdio.cfg` | Default config file name; `-cfg:<filename>` loads an alternate one; legacy switches are colon-glued and underscored (`-drp_dir:value`) | Done - `compat.go` translates old syntax on read, `Settings.Save` always writes new syntax, matching README's stated hard constraint |
| `-drp_dir` (drivers dir) | Default `drivers`, relative/absolute/network/mapped path all allowed | Done |
| `-index_dir` (indexes dir) | Default `indexes` | Done |
| `-output_dir` | Human-readable index output, default `indexes\txt` | Done |
| `-data_dir` | Translations and themes, default `tools\SDIO` | Done (translations/themes UI not built, but the path setting exists) |
| `-log_dir` | Logs and snapshots, default `logs` | Done |
| Network-share layout | Index files embed path info tied to the configured directories; if paths at index-build time and index-use time disagree, indexes get deleted and rebuilt. Recommends building indexes in the exact same path scenario (local vs. UNC) the tool will run under. Logs can live on a separate writable UNC share (`-log_dir:\\server\sdio-logs`) even when the main share is read-only. | Not done - this behavior belongs to `indexing.cpp`, not started. Worth remembering for that port: index invalidation is path-sensitive, not just content-sensitive. |
| `hwid-ignore` file | Manual (Context Menu Options, page 11) says: "Adds your selected hardware id to your local ignore list. This is a text file called `hwid-ignore.txt` in the current directory." | **Manual is imprecise, Go matches the real (original engine) behavior, not the manual's prose.** The original C++ source (`source/settings.cpp:452,483`) builds the filename as `hwid-ignore_%s.txt` with `%s` = hostname, not a bare `hwid-ignore.txt`. `internal/settings/persistence.go`'s `ignoreListFilename()` reproduces this exactly (`fmt.Sprintf("hwid-ignore_%s.txt", host)`). So Go is on-disk-compatible with the real original engine; only the manual's own description is wrong/simplified. Flag this for whoever maintains the manual. |
| Binary index files (`indexes/**/*.bin`) | Not discussed at the byte level in the manual at all (out of scope for a user manual) | Not done - `indexing.cpp` not started. The `"SDW"`+LZMA container format is separately reverse-engineered in `go/README.md`, not sourced from this manual. |
| State snapshots (`logs/*.snp`) | `-ls:<file>` loads one; `-nosnapshot` suppresses creation; script commands `snapshot [filename]` / `loadsnapshot <filename>` / `unloadsnapshot` manage them; default snapshot file name in script mode has no timestamp unless `-nostamp` is absent | Partial - `internal/scan.Prepare` now writes a timestamped `logs/<ts>state.snp` after every real hardware scan (`internal/scan/snapshot.go`), honoring `-nosnapshot`, using the SDW container with a Go-native JSON payload (redesigned per README/COMPATIBILITY.md, not the original's raw struct dump). `Settings.StateFile`/`StateMode` exist and are wired to `-ls:`, but nothing reads a `.snp` back yet (`State::load`'s Go equivalent) - script mode's `snapshot`/`loadsnapshot`/`unloadsnapshot` commands are also not done (`script.cpp` not started) |
| `%VAR%` expansion in paths | `-log_dir` (and by extension other dir settings) can contain environment-variable references, expanded Windows-style | Done - `expandWindowsEnv` in `parse.go` implements `%VAR%` (not `$VAR`/`${VAR}`) expansion, matching Windows `ExpandEnvironmentStrings` convention that `sdio.cfg` relies on |
| Extraction directory | Default `%temp%\SDIO`, overridable by `-extractdir:<dir>` (CLI) or `extractdir <directory>` (script, must precede `init`) | **Not done** - no Go setting exists yet (see above) |

## 4. Driver-pack concepts

### What a driver pack is, and naming convention

The manual does not give a formal definition of a "driver pack," but
the Updates dialog screenshot (page 8) lists real driver-pack file
names, which establish the convention:

```
DP_Videeo_AMD_DCH31_26054.7z
DP_Camera_SDIO001_26081.7z
DP_Chipset_26061.7z
DP_Displayintel_SDIO01_26081.7z
DP_Displaynvidia_SDIO01_26081.7z
DP_Sound_CMedia_26040.7z
DP_Sound_Creative_26044.7z
```

Pattern: `DP_<category>[_<vendor/variant>]_<numeric version>.7z`. The
Scripting chapter's `select` command confirms the middle segment
(between underscores) is a stable "driver pack filter" key - it gives
`lan`, `chipset`, `printer`, `video`, `wlan-wifi`, `wwan-4g` as example
filter values matching pack names like `DP_LAN_xxxxx.7z`. This
category/filter vocabulary is not enumerated exhaustively anywhere in
the manual - only examples are given.

Status: **Not done.** No driver-pack model exists in Go yet
(`indexing.cpp` not started). When it lands, the category-extraction
logic for script `select <filter>` support should parse this
underscore-delimited convention.

### Matching / ranking behavior (Missing/Newer/Current/Older/Better/Worse)

The manual documents the *categories* (see section 2 above) but not the
*algorithm*. It gives operational guidance only:
- "Newer is not always better" - suggests ranking is not a strict
  version-newest-wins policy; a "Better Match" driver can rank above a
  numerically newer one.
- USB 3 host-controller drivers (`iusb3hub`, `iusb3xhc`, `iusb3hcs`)
  must all be installed at the same version together, and not while
  running SDIO itself from a USB 3 port - implies some interdependency
  logic exists (or should exist) between related devices in the same
  chipset family, though the manual does not say whether SDIO enforces
  this automatically or only recommends it to the user. Given the
  imperative wording ("should always be installed together"), this
  currently reads as user discipline, not enforced engine behavior.

Status: **Not done** - belongs to `matcher.cpp` (not started).

### Notebook vs. desktop detection

The manual does not mention notebook/laptop detection anywhere in its
32 pages (no keyboard action, filter, or switch references it). The Go
rewrite already has `IsLaptop()` in `internal/hardware` (ported from
`enum.cpp`'s `State::isnotebook_a`, per README), which is presumably
used by the *original* engine to prefer notebook-specific driver-pack
variants during matching, but this manual gives no confirmation of that
usage. Not a discrepancy - just an area the manual is silent on.

### Virtual OS / architecture emulation

Covered in section 1 above (`-a:`, `-v:`). Purpose per manual: match
driver packs as if running under a different Windows version/bitness
than the host, presumably for building an index or picking driver
packs for a machine other than the one physically running SDIO. Ties
into `Settings.StateMode`/`VirtualOSVersion`/`VirtualArchType`, all
present in Go as storage, `VirtualOSVersion` interpretation not yet
implemented.

### Script mode

Fully documented (manual pages 25-30); **entirely not done** in Go
(`script.cpp` not started). This is a complete mini command language,
recorded here in full since it will need re-implementing:

- Invocation: `SDIO.exe -script:<scriptfile> [option1] [option2] ...].`
  If `-script` appears anywhere on the command line, everything before
  it is ignored and everything is driven by the script.
- Script file: plain text, one command per line, no leading `-`/`/`.
  `#` or `;` at line start = comment. `:` at line start = label
  (goto target). `< >` = required argument, `[ ]` = optional argument.
- Positional parameters: `%0` = script file path, `%1`-`%9` = the 9
  arguments following `-script:<file>` on the command line. Usable
  anywhere in the script, e.g. inside a `goto %1`.
- Script-mode defaults (independent of any `sdio.cfg` - the cfg file is
  ignored entirely once `-script` is used): no log file, no snapshot,
  driver dir `drivers`, log dir `logs`, index dir `indexes`, extract
  dir `%temp%\SDIO`, verbose off, torrent port 50171.
- Commands:
  - `init [reindex]` - load indexes/drivers, scan the PC. Must run
    before most other commands; can be re-run mid-script to pick up
    new driver packs or path changes. `reindex` forces a full rebuild.
  - `checkupdates` - download and read the update torrent into memory.
    Must precede any `get` command, or updates fail. Skip this command
    entirely in environments where the torrent client must not run.
  - `get <app|indexes|driverpacks <all|missing|updates|selected>|everything>`
    - `app`: latest application/tools/languages/themes (non-driver,
      non-index files).
    - `indexes`: latest online indexes.
    - `driverpacks all|missing|updates|selected`: `all` = missing +
      updated packs, `missing` = only absent packs, `updates` = only
      updates to packs already present, `selected` = only the
      already-selected missing/updated packs.
    - `everything`: app + indexes + driver packs, all latest.
  - `select <[missing newer current older better worse]> [drpfilters]`
    - Chooses drivers to install; GUI equivalent of setting expert
      filters then "Select All."
    - Six recognized regular filter keywords: `missing newer current
      older better worse` (note: lowercase, singular forms of the
      `-filters:` category names above - no `worserank`/`nf*` keyword
      equivalents documented for script mode).
    - Anything else given is treated as a driver-pack name filter
      (matched against the pack's underscore-delimited category
      segment, e.g. `lan`, `chipset`, `wlan-wifi`).
    - Regular filters and pack filters can be freely combined
      (`select missing better lan wlan-wifi`).
    - An unrecognized regular-filter-shaped token, or a pack filter
      that matches nothing, selects nothing (silent no-op, not an
      error) - the manual is explicit about this failure mode.
    - If only pack filters are given with no regular filter, nothing is
      selected either.
  - `install` - install whatever `select` chose. Missing driver packs
    auto-download if `checkupdates` already succeeded; no `get` needed
    first.
  - `snapshot [filename]` - save a snapshot; default location is the
    log directory with a timestamped name if `filename` omitted.
  - `loadsnapshot <filename>` - must appear immediately before `init`;
    makes the following `init` load a snapshot instead of scanning.
  - `unloadsnapshot` - must appear immediately before `init`; returns
    to scanning real hardware.
  - `writedevicelist <filename>` - dumps all devices+drivers to a file
    (script-mode equivalent of `-getdevicelist:`).
  - `restorepoint [description]` - create a restore point now, with an
    optional custom description.
  - `logdir <directory>` - must precede the `logging` command.
  - `drpdir <directory>` / `indexdir <directory>` / `extractdir
    <directory>` - must precede `init`.
  - `torrentport <port>` - default 50171.
  - `activetorrent <num>` - 1 = SDIO app+driver updates (default), 2 =
    driver-pack-only updates.
  - `echo [text]` - print to console.
  - `debug [on|off]` - same as `LOG_VERBOSE_DEBUG` bit.
  - `logging [on|off]` - toggles writing to a log file.
  - `verbose [verbositiness]` - same bitmask as `-verbose:<flags>`
    (see section 1).
  - `enableinstall [on|off]` - dry-run switch: `off` runs every step up
    to, but not including, actual restore-point creation and driver
    installation. Explicitly framed as a safe way to test a script
    "without trashing your PC."
  - `reboot [ifneeded]` - reboot now, or only if the last `install`
    reported a reboot was required.
  - `runlatest [arguments]` - re-launches the newest downloaded SDIO
    build, preserving the current process's bitness (32-bit stays
    32-bit, 64-bit stays 64-bit), optionally with extra CLI args (e.g.
    to chain into another script). Manual recommends following it with
    `end` to shut down the old instance since the new one keeps running
    independently.
  - `pause` - halt until a key is pressed.
  - `cmd <command>` - run a shell command.
  - `onerror <end|goto <label>>` - error-handling for the *previous*
    command only (not global): `end` aborts the script, `goto <label>`
    jumps (label reference may or may not include the leading `:`).
  - `goto <label>` - unconditional jump.
  - `end` - terminate the script.

This whole surface maps to `FlagScriptMode` in `flags.go` (currently
just a bit constant with no CLI switch and no interpreter behind it).

### Auto-install mode

`-autoinstall` exists as a `flags.go` constant ("install matched
drivers without prompting") but, as noted in section 1, the manual
never documents this switch directly - it only describes the
equivalent interactive flow (check devices, click Install). Not done
either way (install not ported), but flagged since the manual gives no
independent confirmation of the flag's exact semantics beyond what the
Go doc-comment already asserts (inherited from the original enum name,
not from this manual).

## 5. Install / uninstall, restore points, signing, update mechanism

### Install / uninstall

The manual describes only the interactive flow: check devices, click
*Install*; SDIO extracts drivers from packs and installs them;
recommends not installing too many drivers at once since a bad install
can only be rolled back by restoring *all* driver installations via
the restore point, not selectively. No uninstall workflow is
documented anywhere in the manual (SDIO appears to be install-only from
a user perspective; uninstalling drivers is presumably done via native
Windows tools accessible from the Tools submenu, e.g. Device Manager).

Status: **Partial** - `install.cpp` is ported (`internal/install.Driver`
wraps `newdev.dll!UpdateDriverForPlugAndPlayDevicesW` directly,
extraction/orchestration in `internal/installflow`), so the described
install flow works for real. Uninstall is correctly still not done -
neither Go nor (per this manual) the original engine itself has one.

### Restore points

- Manual strongly recommends always checking "Create a restore point"
  before installing, calling restore points "cheap" (a few seconds to
  create) and "a life saver."
  - Note: multiple driver installs in one run share **one** restore
    point; rolling back undoes all of them together, not individually.
- `-norestorepnt` / `norestorepnt` (script/cfg) disables restore-point
  creation entirely.
- `-nostop` / `nostop`: if restore-point creation fails, continue
  anyway instead of aborting.
- `-disableinstall` also disables restore-point creation (Go now
  matches this - see section 1's `-disableinstall` row above).
- Script `restorepoint [description]` command creates one on demand,
  independent of the install flow, with an optional custom description
  string (falls back to an unspecified default description if omitted).

Status: **Partial** - `internal/install.CreateRestorePoint`/
`GetRestorePointCreationFrequency`/`SetRestorePointCreationFrequency`
port `SrClient.dll!SRSetRestorePointW` and the registry-throttle-bypass
sequence from `Manager::thread_install`, wired into `installflow.Run`
(one restore point before every install run, honoring
`FlagDisableInstall`/`FlagNoRestorePoint`). The standalone script
`restorepoint [description]` command has no equivalent - script mode
itself isn't ported.

### Driver signing / catalog files

**Not mentioned anywhere in the manual.** No discussion of signature
validation, `.cat` files, or unsigned-driver warnings/overrides. This
may be handled silently by the underlying Windows driver-install API
(`DiInstallDriver`/`SetupCopyOEMInf` etc.) without user-facing
configuration, or it may simply be outside this manual's scope. Cannot
confirm or deny engine behavior here from this document; check
`source/install.cpp` directly when that module is ported.

### Update mechanism (torrent-based)

- Two kinds of updates: application updates and driver-pack updates,
  both distributed via torrent (using the bundled BSD-licensed
  libtorrent, credited on the Tools/credits page).
- Update checking happens automatically on every launch; results shown
  via an "Updates" bar on the main window.
- Clicking the bar opens a dialog listing every driver pack with
  size/version/"for this PC?" columns, selectable individually or via
  bulk actions: *Check All*, *Uncheck All*, *This Computer Only*
  (selects packs relevant to the current machine - falls back to
  selecting indexes if none downloaded yet), *Network Only* (all
  Net/LAN/WWAN/Wifi packs, for getting a fresh install online quickly).
- *Only Show Updates*: restrict the list to packs already present
  locally that have newer versions available.
- *Keep Sharing New Driver Packs* (a.k.a. Seeding): after download
  completes, keep the torrent running so others can pull from you;
  packs are not usable/moved into place until you click Stop, at which
  point they're relocated and indexed.
- *Share Mode* (separate feature, System Menu-adjacent): proactively
  re-seeds your entire existing local driver-pack collection to the
  swarm, independent of doing any new download; can run in background
  indefinitely; explicit warning against enabling it on limited
  connections or in sensitive network environments.
- `-activetorrent:<num>` / script `activetorrent <num>`: selects which
  of the two torrents (1 = app+driver-pack combined index, 2 =
  driver-pack-only, which updates more frequently) governs `get`/update
  operations.
- Torrent tuning: `-port`, `-minport`/`-maxport`, `-downlimit`/
  `-uplimit`, `-connections` (all cfg-only per the Command Line
  Reference chapter's placement, i.e. no dedicated CLI-only forms
  beyond what's already listed in section 1).
- Two build variants ship specifically because of the torrent code:
  `SDIO-XP` builds omit torrent support entirely (works down to Windows
  XP/Vista; the torrent library itself dropped XP/Vista support) and
  because some corporate/sensitive network environments flag
  always-on torrent activity as suspicious. Recommended workflow for
  that case: do updates on a normal build outside the sensitive
  network, then run SDIO-XP inside it to install only.

Status: **Not done** - `update.cpp` not started; README notes
`github.com/anacrolix/torrent` as the planned replacement for the
bundled libtorrent glue. The SDIO-XP/full-build split is a packaging
concern (two executables from largely the same source, gated by a
build tag or similar), not yet reflected in `go/README.md`'s module
table - worth a line item there once build/release tooling is
addressed.

## 6. Exit codes, log conventions, error handling

### Exit codes

The manual documents exactly one exit/error code, and it belongs to the
bundled 7-Zip tool, not to SDIO itself: `-7z` sub-command errors "of 2
usually means File Not Found" (i.e., 7-Zip's own exit code passed
through). **No SDIO-specific exit codes are documented anywhere in the
manual.**

### Log file conventions

- Default log directory: `logs`, overridable via `-log_dir`/`log_dir`
  (supports `%VAR%` expansion, UNC paths).
- `-nologfile` / `nologfile`: suppress log file creation.
- `-nostamp` / `nostamp`: omit timestamp from log (and snapshot) file
  names. Implies the default naming scheme otherwise includes a
  timestamp.
- `-verbose:<flags>` / script `verbose [verbositiness]`: bitmask
  controlling which log sections are written (see full table in
  section 1). Default with no flag given is effectively "nothing" per
  the script-mode defaults list.
- `debug [on|off]` (script only): shortcut for the `LOG_VERBOSE_DEBUG`
  bit (0x1000) rather than the full bitmask syntax.
- `Control+Z` (interactive keyboard action): manually insert a
  timestamped divider line into the log - a user-triggered log
  annotation feature, not applicable to a headless/CLI engine but worth
  remembering if a TUI front end wants an equivalent.
- `F7` (interactive keyboard action): dump "all desktop windows
  information" to the log - described as useful for "catching rogue
  installer dialogs" (i.e., diagnosing installers that pop up
  unexpected UI during a silent install). GUI/diagnostic feature, not
  an engine-layer concern for this rewrite yet.

Status: **Partial.** `internal/logging` (per README) is `zerolog`-based
with "verbosity bitmask collapsed to one level threshold" - deliberate
simplification versus the documented 13-section bitmask. `-nologfile`,
`-nosnapshot`, `-nostamp` are parsed/stored in `internal/settings` as
one-shot flags; nothing yet acts on them since no logger wiring or
snapshot writer exists to check them against.

### Error handling

- Script mode's `onerror <end|goto <label>>` is the only structured
  error-handling construct the manual documents, and it is scoped to
  "the previous command" only - not a global handler. No other error
  codes, retry behavior, or failure semantics are documented for any
  other mode (interactive or CLI).
- `-nostop` is the one flag that changes error-handling behavior
  outside script mode: it downgrades a restore-point-creation failure
  from fatal to non-fatal.
- The manual gives no guidance on what happens if a `-drp_dir`,
  `-index_dir`, etc. path is invalid or unwritable, beyond the general
  installation note that a read-only environment simply means "updates
  won't work" (no error code specified, just a capability limitation).

Status: **Not done** beyond flag storage - no script interpreter, no
error-handling logic implemented yet in Go.

## Summary of gaps worth acting on

1. ~~`-extractdir` has no Go equivalent at all~~ **Done**:
   `Settings.ExtractDirRaw`/`ExtractDir`, the `-extractdir` flag, and
   real extraction into it (`installflow.InstallOne`) all exist now.
2. ~~`-disableinstall`'s help text is incomplete~~ **Done**: its help
   text and `installflow.Run` both cover restore-point suppression now.
3. **Several `flags.go` constants have zero manual documentation**
   (`checkupdates`, `onlyupdates`, `torrentalerts`, `nogui`,
   `autoinstall`, `autoclose`, `autoupdate`). Not necessarily wrong -
   likely ported faithfully from `source/settings.h` - but this manual
   cannot confirm their exact semantics. Verify against
   `source/settings.cpp`'s parse/save logic directly when implementing
   the behavior behind each.
4. **`hwid-ignore` file name**: manual says `hwid-ignore.txt`; both the
   original engine and the Go port actually use
   `hwid-ignore_<hostname>.txt`. Go is correct (matches real behavior);
   only the manual text is wrong. No code change needed - flagged so
   nobody "fixes" `persistence.go` to match the manual by mistake.
5. ~~`-v:<version>` code table ... not yet implemented as a lookup~~
   **Partially done**: the lookup itself is transcribed
   (`hardware.FindWindowsVersionName`, wired from `parse.go`). What's
   still missing is the actual emulation effect - neither `-a:`'s
   `VirtualArchType` nor `-v:`'s `VirtualOSVersion` is read anywhere in
   `internal/scan`/`internal/collection` to match against a virtual
   environment instead of the real detected one.
6. **Verbose logging bitmask**: manual documents 13 distinct log
   sections; Go collapsed this to one threshold level. Confirm this
   simplification is an accepted product decision, not an oversight,
   before more log call sites depend on the collapsed model.
