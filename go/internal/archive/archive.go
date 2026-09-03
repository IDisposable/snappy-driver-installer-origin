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
	"os"
	"path/filepath"
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

// ExtractPrefix extracts every file whose name starts with prefix
// (archive paths use "/", e.g. "dt/allx64/DtPort_1.0.0.6/") into
// destDir, preserving the path structure below prefix. This is the
// step driver_install (install.cpp) needs before it can call
// UpdateDriverForPlugAndPlayDevices: Windows requires an .inf's
// supporting files (.sys/.dll/.cat) to already be on disk alongside
// it, not just the .inf itself. Returns the number of files
// extracted.
func (r *Reader) ExtractPrefix(prefix, destDir string) (int, error) {
	n := 0
	for _, f := range r.rc.File {
		if f.FileInfo().IsDir() || !strings.HasPrefix(f.Name, prefix) {
			continue
		}
		rel := strings.TrimPrefix(f.Name, prefix)
		dest := filepath.Join(destDir, filepath.FromSlash(rel))

		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return n, fmt.Errorf("creating %s: %w", filepath.Dir(dest), err)
		}

		rc, err := f.Open()
		if err != nil {
			return n, fmt.Errorf("opening %s in archive: %w", f.Name, err)
		}
		out, err := os.Create(dest)
		if err != nil {
			rc.Close()
			return n, fmt.Errorf("creating %s: %w", dest, err)
		}
		_, copyErr := io.Copy(out, rc)
		rc.Close()
		closeErr := out.Close()
		if copyErr != nil {
			return n, fmt.Errorf("writing %s: %w", dest, copyErr)
		}
		if closeErr != nil {
			return n, fmt.Errorf("closing %s: %w", dest, closeErr)
		}
		n++
	}
	if n == 0 {
		return 0, fmt.Errorf("no files found with prefix %q", prefix)
	}
	return n, nil
}
