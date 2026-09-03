package collection

import (
	"fmt"
	"os"
	"path/filepath"

	"sdio/internal/indexing"
	"sdio/internal/sdwfile"
)

// SkippedPack is a driver pack found on disk whose index couldn't be
// loaded.
type SkippedPack struct {
	Filename string
	Err      error
}

// LoadResult separates driver packs that loaded successfully from
// ones whose index couldn't be read (missing, corrupt, or otherwise
// unreadable) - ported from the file-discovery half of
// Collection::scanfolder, minus reindexing: a driver pack with no
// valid index can't be built by this rewrite yet (genindex's write
// side isn't ported - see go/README.md), so such packs are reported
// in Skipped rather than indexed on the fly.
type LoadResult struct {
	Packs   []*indexing.Driverpack
	Skipped []SkippedPack
}

// indexFilename computes the expected indexes/**/*.bin filename for a
// driver pack, assuming it lives directly under the collection root
// (no subfolder nesting) - the only case a real installation used to
// develop this rewrite exhibits. Driverpack::getindexfilename in
// indexing.cpp also handles a nested-subfolder case (mirroring the
// pack's subfolder into the index filename, with backslashes/spaces
// sanitized to underscores) that isn't replicated here for lack of a
// real example to verify the byte-exact naming against.
func indexFilename(packFilename string) string {
	ext := filepath.Ext(packFilename)
	return packFilename[:len(packFilename)-len(ext)] + ".bin"
}

// LoadCollection scans driverpackDir for .7z driver packs (see
// indexing.ScanDriverpackFolder) and loads each one's compiled index
// from indexDir, ported from the file-discovery and index-loading
// (not reindexing) parts of Collection::scanfolder/
// Driverpack::loadindex.
func LoadCollection(driverpackDir, indexDir string) (LoadResult, error) {
	files, err := indexing.ScanDriverpackFolder(driverpackDir)
	if err != nil {
		return LoadResult{}, err
	}

	var result LoadResult
	for _, f := range files {
		idx, err := loadIndex(filepath.Join(indexDir, indexFilename(f.Filename)))
		if err != nil {
			result.Skipped = append(result.Skipped, SkippedPack{Filename: f.Filename, Err: err})
			continue
		}
		result.Packs = append(result.Packs, &indexing.Driverpack{Path: f.Dir, Filename: f.Filename, Index: idx})
	}
	return result, nil
}

func loadIndex(path string) (*indexing.Index, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	_, payload, err := sdwfile.Decode(f, true)
	if err != nil {
		return nil, fmt.Errorf("decoding %s: %w", path, err)
	}
	idx, err := indexing.DecodeIndex(payload)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return idx, nil
}
