# Current data contract

The Go rewrite uses its own current configuration format. It does not
read the original SDIO configuration format.

- **`sdigo.cfg`** uses standard Go flag syntax such as
  `-drp-dir="drivers"` and `-filters=1062`. The program writes this file
  when settings change. Command-line values override file values.
- **Filter bits**: `-filters=N` is persisted as a raw integer. Bits use
  the declaration order in `internal/settings/filters.go`.
- **Torrent source**: the embedded torrent metadata is the default.
  `-torrent-file=*` selects the mutable project seed and another
  `-torrent-file` value selects a user source.
- **Binary index files** (`indexes/**/*.bin`): these use an `"SDW"` +
  LZMA container format (see below). Reading and writing them stays
  byte-compatible with existing indexed driver packs. State snapshots
  (`logs/*.snp`) use the same container but contain a Go-native payload.

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
