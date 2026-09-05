// Package sdwfile reads and writes the "SDW" container format used by
// SDIO's driver-pack index cache (indexes/**/*.bin).
//
// Layout: a 3-byte magic "SDW", a 4-byte little-endian format version,
// then a payload that is either raw bytes or - in the default
// configuration (COLLECTION_USE_LZMA) - the 7-Zip SDK's Lzma86_Encode
// output: a 1-byte filter mode (0 = none, 1 = BCJ x86 filter applied),
// followed by a classic LZMA-alone stream (5-byte properties + 8-byte
// uncompressed size + compressed data). The filter-mode byte is easy to
// miss: without skipping it, an LZMA-alone reader misparses it as the
// first byte of the properties, producing a nonsensical multi-gigabyte
// dictionary size. Confirmed byte-for-byte against real index files
// from a production installation - about 45% of real files use filter
// mode 1 (see bcjx86.go for the inverse filter), so both modes need to
// round-trip correctly, not just mode 0.
//
// This container format is also used by logs/*.snp state snapshots,
// but byte-compatibility for those is explicitly not required (see
// docs/COMPATIBILITY.md) - only indexes/**/*.bin needs to round-trip.
package sdwfile

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/ulikunitz/xz/lzma"
)

var magic = [3]byte{'S', 'D', 'W'}

const headerLen = 7 // 3-byte magic + 4-byte version

// Lzma86 filter-mode byte values.
const (
	modeNone = 0 // no filter
	modeBCJ  = 1 // x86 BCJ (branch conversion) filter applied
)

// PeekVersion reads just the SDW header (magic + version), without
// touching the LZMA payload - a cheap "is this a valid, current-format
// index file" check used when deciding whether a driver pack needs
// reindexing, without paying for a full decompression.
func PeekVersion(r io.Reader) (version int32, ok bool) {
	var header [headerLen]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, false
	}
	if !bytes.Equal(header[:3], magic[:]) {
		return 0, false
	}
	return int32(binary.LittleEndian.Uint32(header[3:7])), true
}

// Decode reads an SDW container, returning its format version and
// decompressed (or raw, if lzmaCompressed is false) payload. r is
// wrapped in a bufio.Reader internally: the LZMA decoder reads its
// input a byte at a time, so an unbuffered reader (e.g. an *os.File
// passed directly) turns decompression into one syscall per byte -
// catastrophically slow rather than merely inefficient.
func Decode(r io.Reader, lzmaCompressed bool) (version int32, payload []byte, err error) {
	r = bufio.NewReader(r)

	var header [headerLen]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, fmt.Errorf("reading SDW header: %w", err)
	}
	if !bytes.Equal(header[:3], magic[:]) {
		return 0, nil, fmt.Errorf("not an SDW container (magic %q)", header[:3])
	}
	version = int32(binary.LittleEndian.Uint32(header[3:7]))

	if !lzmaCompressed {
		payload, err = io.ReadAll(r)
		if err != nil {
			return version, nil, fmt.Errorf("reading SDW payload: %w", err)
		}
		return version, payload, nil
	}

	var mode [1]byte
	if _, err := io.ReadFull(r, mode[:]); err != nil {
		return version, nil, fmt.Errorf("reading Lzma86 filter mode: %w", err)
	}
	if mode[0] != modeNone && mode[0] != modeBCJ {
		return version, nil, fmt.Errorf("unsupported Lzma86 filter mode %d (only 0 and 1 are implemented)", mode[0])
	}

	lr, err := lzma.NewReader(r)
	if err != nil {
		return version, nil, fmt.Errorf("reading LZMA header: %w", err)
	}
	payload, err = io.ReadAll(lr)
	if err != nil {
		return version, nil, fmt.Errorf("decompressing SDW payload: %w", err)
	}

	if mode[0] == modeBCJ {
		bcjX86Decode(payload)
	}
	return version, payload, nil
}

// Encode writes an SDW container with the given format version and
// payload, LZMA-compressing it (with Lzma86 filter mode 0, i.e. no BCJ
// filter) unless lzmaCompressed is false. w is wrapped in a
// bufio.Writer internally, for the same reason Decode wraps its
// reader: the LZMA encoder writes a byte at a time.
func Encode(w io.Writer, version int32, payload []byte, lzmaCompressed bool) error {
	bw := bufio.NewWriter(w)

	var header [headerLen]byte
	copy(header[:3], magic[:])
	binary.LittleEndian.PutUint32(header[3:7], uint32(version))
	if _, err := bw.Write(header[:]); err != nil {
		return fmt.Errorf("writing SDW header: %w", err)
	}

	if !lzmaCompressed {
		if _, err := bw.Write(payload); err != nil {
			return err
		}
		return bw.Flush()
	}

	if _, err := bw.Write([]byte{modeNone}); err != nil {
		return fmt.Errorf("writing Lzma86 filter mode: %w", err)
	}

	lw, err := lzma.NewWriter(bw)
	if err != nil {
		return fmt.Errorf("writing LZMA header: %w", err)
	}
	if _, err := lw.Write(payload); err != nil {
		return fmt.Errorf("compressing SDW payload: %w", err)
	}
	if err := lw.Close(); err != nil {
		return fmt.Errorf("closing LZMA writer: %w", err)
	}
	return bw.Flush()
}
