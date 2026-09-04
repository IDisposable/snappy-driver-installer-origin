# On-disk compatibility

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

## The SDW container format

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
