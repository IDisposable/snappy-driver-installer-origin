# TODO

Outstanding work tracked here instead of only in commit messages, so
it survives between sessions. See `docs/PORTING_NOTES.md` and
`docs/reference-manual-notes.md` for the full module-by-module and
flag-by-flag gap lists - this file is for specific, actionable items
someone intends to pick up next, not a general status table.

## -onlyupdates

Not implemented - `internal/settings/flags.go`'s help text says so
directly. Needs `internal/update/bulk.go` wiring:

- Original semantics (`SDIO-baseline/source/update.cpp:2022-2028`, `getnewver`/
  `getcurver` at `update.cpp:395-425`): only download a pack if a file
  with the same base name (revision suffix stripped) already exists in
  the drp directory *and* the torrent's copy has a higher revision -
  skip packs never downloaded before. This is narrower than the
  current (unused) flag help text implies ("newer than what's on
  disk") - that description is the *default* filter's behavior
  (`newver>oldver`), not what `-onlyupdates` adds on top of it
  (`&&oldver`, i.e. "and I already had one").
- The revision-number parse (`getnewver`) is already ported as
  `packVersionNumber` in `internal/indexing/altsectscore.go:42` -
  unexported, written for `CalcAltSectScore`. Needs exporting (or a
  small duplicate in `internal/update` if cross-package coupling isn't
  wanted) rather than reimplementing.
- No Go equivalent of `getcurver`'s directory scan (find a file in
  `s.DrpDir` whose base name matches, ignoring revision) exists yet -
  write one.
- Wire the result into a `DriverPackFilter` closure over `s.DrpDir`,
  same pattern as `cmd/sdigo`'s `thisMachineDriverPacksFilter`
  (`cmd/sdigo/downloadmenu.go`) - a filter that needs local disk state
  can't be a stateless `func(filename string) bool` like
  `AllDriverPacks`/`NetworkDriverPacks`.
- Update the flag's help text and `reference-manual-notes.md`'s
  `-onlyupdates` row once done.
