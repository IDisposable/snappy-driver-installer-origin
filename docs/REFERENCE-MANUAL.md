# SDIO Go Reference Manual

## Scope

`sdigo` is a Windows-only driver matching and installation tool. It scans
Plug and Play devices, matches them against driver-pack indexes, downloads
selected data when needed, and can install selected drivers.

The Go rewrite does not read the original SDIO configuration format.

## Start

Run `sdigo.exe` for the interactive terminal interface.

Use `-nogui` for a plain text report:

```text
sdigo.exe -nogui
```

Use `-json` with `-nogui` for machine-readable output:

```text
sdigo.exe -nogui -json
```

The main scan does not install drivers. Installation requires `-install` in
headless mode or an explicit selection in the terminal interface.

## Data Directories

The default portable layout is:

| Directory | Purpose |
| --- | --- |
| `drivers` | Completed driver-pack archives. |
| `indexes` | Driver-pack index files. |
| `updates` | Torrent payloads and torrent client state while files download. |
| `logs` | Log files and state snapshots. |
| `%TEMP%\\SDIO` | Extracted driver files during installation. |

If the executable is installed without portable markers, the application uses
`%LOCALAPPDATA%\\SDIO` for application data. A working directory or executable
directory containing `sdigo.cfg`, `drivers`, or `indexes` selects portable mode.

Completed driver packs and indexes replace files with the same name when a
newer revision is downloaded. This is expected update behavior.

## Configuration

The program reads `sdigo.cfg` before command-line parsing. Command-line values
override file values. The program writes current-format flag syntax such as:

```text
-drp-dir="drivers"
-index-dir="indexes"
-updates-dir="updates"
-filters=531
```

The old SDIO configuration format is not supported.

Important path options:

| Option | Meaning |
| --- | --- |
| `-drp-dir=<path>` | Driver-pack directory. |
| `-index-dir=<path>` | Index directory. |
| `-output-dir=<path>` | Human-readable index output directory. |
| `-updates-dir=<path>` | Torrent storage directory. It contains payloads and `.torrent.db`. |
| `-log-dir=<path>` | Log directory. `%VAR%` references are expanded. |
| `-extractdir=<path>` | Extract-only directory. It also enables extract-only mode. |
| `-torrent-file=<source>` | Local torrent file, magnet URI, or HTTPS torrent URL. |
| `-torrent-file=*` | Fetch the mutable torrent metadata from the project `main` seed. |

An empty `-torrent-file` value selects the embedded torrent metadata. A user
source or `*` overrides the embedded metadata.

## Display Filters

`-filters=N` controls which scan rows are visible. The current bit layout uses
one bit per filter in declaration order:

| Bit | Option screen name | Meaning |
| ---: | --- | --- |
| 0 | `missing` | A device has no installed driver. |
| 1 | `newer` | The candidate is newer than the installed driver. |
| 2 | `current` | The candidate has the same date as the installed driver. |
| 3 | `old` | The candidate is older than the installed driver. |
| 4 | `better` | The candidate outranks the installed driver. |
| 5 | `worse-rank` | The candidate ranks below the installed driver. |
| 6 | `nf-missing` | No candidate exists and no driver is installed. |
| 7 | `nf-unknown` | No candidate exists and the installed driver is unknown. |
| 8 | `nf-standard` | No candidate exists and a standard driver is installed. |
| 9 | `one` | Show one best candidate per device. |
| 10 | `dup` | Show duplicate candidates. |
| 11 | `invalid` | Show structurally invalid candidates. |

The default filter value is `531`.

## Commands

| Command | Purpose |
| --- | --- |
| `sdigo cleandrivers` | List superseded driver packs. |
| `sdigo cleandrivers -delete` | Remove superseded driver packs. |
| `sdigo hwdump` | Print hardware and install API diagnostics. |
| `sdigo torrenttest` | Test a torrent source and selected files. |

`torrenttest` uses `-storage-dir=<path>` for torrent payloads and client state.
It is a diagnostic command, not a normal scan operation.

## Scan Options

| Option | Meaning |
| --- | --- |
| `-nogui` | Print a report instead of starting the terminal interface. |
| `-json` | Print JSON with `-nogui`. |
| `-device-list=<path>` | Write a tab-separated device report after scanning. |
| `-json` with `-device-list` | Write the device report as JSON. |
| `-filters=<number>` | Select display categories. |
| `-ls=<path>` | Replay a saved state snapshot instead of scanning hardware. |
| `-arch=32` or `-arch=64` | Match against a virtual architecture. |
| `-virtual-os-version=<code>` | Match against a virtual Windows version. |
| `-filtersp` | Restrict matches to service-pack validated dates. |
| `-reindex` | Rebuild driver-pack indexes. |
| `-index-hr` | Write human-readable output when indexes are rebuilt. |
| `-nosnapshot` | Do not save a state snapshot after scanning. |
| `-nologfile` | Do not write a log file. |

## Download Options

| Option | Meaning |
| --- | --- |
| `-checkupdates` | Refresh the index catalog. |
| `-onlyupdates` | Download only higher revisions of packs already present locally. |
| `-autoupdate` | Download all driver packs once after the first scan. |
| `-keepseeding` | Continue seeding after a download completes. |
| `-torrentalerts` | Log torrent warning and error events. |

Downloads select only the required torrent files. A completed file is verified
by the torrent library before it is moved into `drivers` or `indexes`.

## Installation

Headless installation:

```text
sdigo.exe -nogui -install
```

The install flow:

1. Downloads pending driver packs when a torrent source is available.
2. Creates a restore point unless disabled.
3. Extracts the matched driver folder.
4. Calls the Windows driver installation API.
5. Removes the extraction directory after a real install.

Useful install options:

| Option | Meaning |
| --- | --- |
| `-disableinstall` | Scan, extract, and report without installing or creating a restore point. |
| `-extractdir=<path>` | Extract without installing and retain the extracted files. |
| `-norestorepnt` | Do not create a restore point. |
| `-nostop` | Continue if restore-point creation fails. |
| `-delextrainfs` | Remove extra `.inf` files from the extracted folder. |
| `-finish-reboot` | Request a Windows reboot after a successful install. |

Driver installation requires elevation. Scanning does not.

## USB Copy

Use `-nogui -usb=<destination>` to copy the executable, driver packs, and
indexes without scanning hardware:

```text
sdigo.exe -nogui -usb=E:\
```

The destination option is required for headless USB copy. It cannot be
combined with `-install`.

The terminal interface can copy the executable, driver packs, and indexes to a
removable drive. The copy operation overwrites files with the same destination
path and leaves unrelated files in place. It does not format the drive or
delete existing files.

## Files and State

- Index files use the `SDW` container and remain compatible with existing
  driver-pack indexes.
- State snapshots use the `SDW` container with a Go-native JSON payload.
- `.torrent.db` is torrent client state. It is not a driver pack or index.
- Interrupted downloads keep staged data in `updates` so the next run can
  resume.

## Exit Behavior

`-nogui` returns a nonzero exit code when scanning or installation fails.
The interactive interface shows operation errors in its operation log.
