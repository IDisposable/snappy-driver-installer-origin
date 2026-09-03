// Package archive reads driver-pack .7z archives, replacing the
// bundled 7-Zip SDK's C API (SzArEx_Open/SzArEx_Extract, used by
// Driverpack::genindex in indexing.cpp) with
// github.com/bodgit/sevenzip, a pure-Go reader - per this rewrite's
// standing decision to use existing Go libraries for 7-Zip and torrent
// support rather than porting the bundled C SDKs.
package archive

import (
	"fmt"
	"io"
	"strings"

	"github.com/bodgit/sevenzip"
)

// File describes one regular file inside a driver-pack archive.
type File struct {
	Name             string // full path within the archive, e.g. "dt/allx64/DtPort_1.0.0.6/dtport.inf"
	UncompressedSize uint64
}

// Reader wraps an open driver-pack .7z archive.
type Reader struct {
	rc *sevenzip.ReadCloser
}

// Open opens a driver-pack .7z archive for reading.
func Open(path string) (*Reader, error) {
	rc, err := sevenzip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	return &Reader{rc: rc}, nil
}

// Close releases the archive's underlying file handle.
func (r *Reader) Close() error {
	return r.rc.Close()
}

// Files lists every regular (non-directory) file in the archive,
// ported from the SzArEx_IsDir-filtered loop in
// Driverpack::genindex.
func (r *Reader) Files() []File {
	var files []File
	for _, f := range r.rc.File {
		if f.FileInfo().IsDir() {
			continue
		}
		files = append(files, File{Name: f.Name, UncompressedSize: f.UncompressedSize})
	}
	return files
}

// HasSuffixFold reports whether name ends with suffix, ignoring case -
// matching the original's _wcsicmp-based extension checks (looking for
// .inf/.infdrp/.cat files) in Driverpack::genindex.
func HasSuffixFold(name, suffix string) bool {
	return len(name) >= len(suffix) && strings.EqualFold(name[len(name)-len(suffix):], suffix)
}

// Extract reads the full, decompressed contents of the named file from
// the archive, ported from the SzArEx_Extract call in
// Driverpack::genindex.
func (r *Reader) Extract(name string) ([]byte, error) {
	for _, f := range r.rc.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("opening %s in archive: %w", name, err)
		}
		defer rc.Close()

		data, err := io.ReadAll(rc)
		if err != nil {
			return nil, fmt.Errorf("reading %s from archive: %w", name, err)
		}
		return data, nil
	}
	return nil, fmt.Errorf("%s not found in archive", name)
}
