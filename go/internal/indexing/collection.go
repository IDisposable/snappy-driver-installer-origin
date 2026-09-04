package indexing

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// DriverpackFile is one discovered driver-pack archive on disk.
type DriverpackFile struct {
	Dir      string // directory containing the file
	Filename string
}

// Path joins Dir and Filename.
func (f DriverpackFile) Path() string {
	return filepath.Join(f.Dir, f.Filename)
}

// ScanDriverpackFolder recursively finds every .7z file under root,
// ported from the packed-driver-pack case of Collection::scanfolder in
// indexing.cpp (using filepath.WalkDir instead of manual
// FindFirstFile/FindNextFile recursion). The "unpacked" mode - scanning
// loose .inf/.cat files directly, used only when a driver pack has
// already been extracted via -PATH - isn't ported; see go/README.md.
func ScanDriverpackFolder(root string) ([]DriverpackFile, error) {
	var found []DriverpackFile
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(d.Name()), ".7z") {
			found = append(found, DriverpackFile{Dir: filepath.Dir(path), Filename: d.Name()})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scanning %s: %w", root, err)
	}
	return found, nil
}

// CountDriverpacksNeedingIndex counts how many of files need a fresh
// index, ported from Collection::scanfolder_count. If forceReindex is
// set (COLLECTION_FORCE_REINDEXING), every file counts; otherwise
// hasValidIndex is consulted per file (e.g. checking
// sdwfile.PeekVersion on that driver pack's indexes/<name>.bin).
func CountDriverpacksNeedingIndex(files []DriverpackFile, forceReindex bool, hasValidIndex func(DriverpackFile) bool) int {
	if forceReindex {
		return len(files)
	}
	count := 0
	for _, f := range files {
		if !hasValidIndex(f) {
			count++
		}
	}
	return count
}
