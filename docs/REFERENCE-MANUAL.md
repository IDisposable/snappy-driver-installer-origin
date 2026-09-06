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

## Terminal Interface

The main device table uses these keys:

| Key | Action |
| --- | --- |
| `d` | Open the download menu. |
| `u` | Open the removable-drive USB copy screen. |
| `o` | Open engine options. |
| `f` | Open display filters. |
| `space` | Select or clear the current device. |
| `enter` | Open device details. |
| `i` | Start installation for selected devices. |
| `?` | Open the about screen. |
| `q` | Quit. |

The download menu contains index refresh, network-driver, machine-driver,
and full-collection download actions. The full-collection action starts
immediately because it is an explicit user choice. Use `d`, `esc`, or `q` to
return to the device table. Use `up` and `down` to move, and `enter` or
`space` to select.

### Startup

The program shows a short splash screen, then scans hardware and loads the
driver-pack collection. The scan screen shows collection progress. On a first
run, the download menu opens after the scan. If `-checkupdates` or
`-autoupdate` is set, its download starts after the first scan.

### Device Table

The table shows the devices included by the active filters. The Status column
uses two comparison axes:

| Status | Date axis | Match axis | Meaning |
| --- | --- | --- | --- |
| `Newer, better` | Candidate date is newer. | Candidate score is better. | Recommended upgrade with a newer release date. |
| `Same date, better` | Candidate date is the same. | Candidate score is better. | Recommended upgrade based on catalog, feature, or match quality. |
| `Older, better` | Candidate date is older. | Candidate score is better. | Recommended because match quality outranks the installed driver. |
| `Better match` | No installed driver to compare. | Candidate is valid and actionable. | A driver is available for a device without an installed driver. |
| `No match` | Not applicable. | No actionable candidate. | No valid upgrade is available. |

The date axis compares driver dates. The match axis compares catalog
validation, feature score, and hardware-ID match quality. The displayed
driver version is a separate four-part version value and is shown in the
Version column and detail screen.

The detail screen compares the installed driver and candidate provider, date,
version, matched ID, INF file, section, and score.

Selections use device instance IDs, so changing filters does not lose a
selection. `a` selects eligible rows, and `n` clears all selections. Microsoft
provided drivers are excluded from select-all, but a user can select one row
manually.

### Options

The options screen contains all registered engine flags. Use `up` and `down`
to move and `space` or `enter` to toggle. Persistent flags are saved to
`sdigo.cfg` when the program exits. Most engine flags affect the next scan.

The filters screen is separate from engine options. Filter changes apply to the
table immediately and are saved through the `-filters` value.

### Downloads

The download screen reports progress for indexes or driver packs. `esc`
cancels a running download. Files that completed and passed torrent
verification are retained. A completed download is moved into `indexes` or
`drivers`, then the collection is loaded again so the table reflects the new
files.

### Installation Flow

Press `i` in the table after selecting one or more devices. The confirmation
screen lists every selected candidate. Press `y` or `enter` to continue, or
`n`, `esc`, or `q` to cancel. The program requests elevation only when the
installation is confirmed. If elevation is needed, the selection is carried
to the elevated process.

The operation log remains visible after installation. Press `esc` or `q` to
dismiss it. Pressing `enter` does not dismiss an unread error log.

### USB Copy Flow

Press `u` to list removable drives. Select a drive, review the available and
required space, then confirm with `y` or `enter`. The operation copies the
running executable, driver packs, and indexes. It overwrites files with the
same destination path and does not format the drive or remove unrelated files.

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
newer revision is downloaded.

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
