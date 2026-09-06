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

// ExtractOptions limits the amount of data extracted from one archive.
type ExtractOptions struct {
	MaxFileBytes  uint64
	MaxTotalBytes uint64
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

// Files lists every regular (non-directory) file in the archive.
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
// archive entries for .inf/.infdrp/.cat files can carry mixed-case
// extensions, so an exact-case suffix check would silently miss some.
func HasSuffixFold(name, suffix string) bool {
	return len(name) >= len(suffix) && strings.EqualFold(name[len(name)-len(suffix):], suffix)
}

// Extract reads the full, decompressed contents of the named file from
// the archive.
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
	return r.ExtractPrefixWithOptions(prefix, destDir, ExtractOptions{})
}

// ExtractPrefixWithOptions extracts files while enforcing path containment
// and optional per-file and total byte limits.
func (r *Reader) ExtractPrefixWithOptions(prefix, destDir string, options ExtractOptions) (int, error) {
	n := 0
	var totalBytes uint64
	for _, f := range r.rc.File {
		if f.FileInfo().IsDir() || !strings.HasPrefix(f.Name, prefix) {
			continue
		}
		rel := strings.TrimPrefix(f.Name, prefix)
		rel = filepath.Clean(filepath.FromSlash(rel))
		if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return n, fmt.Errorf("unsafe archive path %q", f.Name)
		}
		dest := filepath.Join(destDir, rel)
		if options.MaxFileBytes > 0 && f.UncompressedSize > options.MaxFileBytes {
			return n, fmt.Errorf("archive file %s exceeds the %d-byte limit", f.Name, options.MaxFileBytes)
		}
		if options.MaxTotalBytes > 0 && (f.UncompressedSize > options.MaxTotalBytes || totalBytes > options.MaxTotalBytes-f.UncompressedSize) {
			return n, fmt.Errorf("archive extraction exceeds the %d-byte limit", options.MaxTotalBytes)
		}

		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return n, fmt.Errorf("creating %s: %w", filepath.Dir(dest), err)
		}

		rc, err := f.Open()
		if err != nil {
			return n, fmt.Errorf("opening %s in archive: %w", f.Name, err)
		}
		tmp, err := os.CreateTemp(filepath.Dir(dest), ".sdio-extract-*")
		if err != nil {
			rc.Close()
			return n, fmt.Errorf("creating %s: %w", dest, err)
		}
		var source io.Reader = rc
		if options.MaxFileBytes > 0 {
			source = io.LimitReader(rc, int64(options.MaxFileBytes)+1)
		}
		_, copyErr := io.Copy(tmp, source)
		rc.Close()
		closeErr := tmp.Close()
		if copyErr != nil {
			os.Remove(tmp.Name())
			return n, fmt.Errorf("writing %s: %w", dest, copyErr)
		}
		if closeErr != nil {
			os.Remove(tmp.Name())
			return n, fmt.Errorf("closing %s: %w", dest, closeErr)
		}
		if options.MaxFileBytes > 0 {
			if info, err := os.Stat(tmp.Name()); err != nil {
				os.Remove(tmp.Name())
				return n, fmt.Errorf("checking %s: %w", dest, err)
			} else if uint64(info.Size()) > options.MaxFileBytes {
				os.Remove(tmp.Name())
				return n, fmt.Errorf("archive file %s exceeds the %d-byte limit", f.Name, options.MaxFileBytes)
			}
		}
		if err := os.Rename(tmp.Name(), dest); err != nil {
			os.Remove(tmp.Name())
			return n, fmt.Errorf("placing %s: %w", dest, err)
		}
		totalBytes += f.UncompressedSize
		n++
	}
	if n == 0 {
		return 0, fmt.Errorf("no files found with prefix %q", prefix)
	}
	return n, nil
}
