package collection

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
// from indexDir - reindexing isn't supported (see LoadResult's doc),
// so a pack with no valid index is only reported, never rebuilt. It
// also loads any pending (not-yet-downloaded) packs found via
// LoadOnlineIndexes, appended to LoadResult.Packs with Pending set.
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

	pending, err := LoadOnlineIndexes(indexDir, result.Packs)
	if err != nil {
		return result, err
	}
	for _, p := range pending {
		p.Path = driverpackDir
	}
	result.Packs = append(result.Packs, pending...)

	return result, nil
}

// expectedPackFilename reconstructs the driver-pack .7z filename an
// underscore-prefixed pending index file (e.g.
// "_P_Ports_SDIO01_26083.bin") stands in for: the leading "_" replaces
// the "D" of the "DP_..." naming convention, and the extension is
// swapped from .bin to .7z. LoadOnlineIndexes relies on this exact
// reversal to recognize a pack that's already been downloaded; get it
// wrong and an up-to-date pack would keep showing as pending forever.
func expectedPackFilename(pendingIndexFilename string) string {
	base := "D" + pendingIndexFilename[1:]
	return strings.TrimSuffix(base, filepath.Ext(base)) + ".7z"
}

// LoadOnlineIndexes finds driver packs whose index has been
// downloaded but whose .7z data hasn't, so they can still be matched
// against - installing one needs a torrent download first (see
// go/README.md's update.cpp entry). Ported from
// Collection::loadOnlineIndexes: such packs are marked by an
// underscore-prefixed index filename under indexDir (see
// expectedPackFilename). alreadyLoaded lists the driver packs
// LoadCollection already found locally; a pending entry is skipped if
// its reconstructed .7z filename is already among them (matching the
// original's System.FileExists check) - that means it's already been
// downloaded and its underscore-prefixed placeholder is just stale,
// as most real installations accumulate over time.
func LoadOnlineIndexes(indexDir string, alreadyLoaded []*indexing.Driverpack) ([]*indexing.Driverpack, error) {
	have := make(map[string]bool, len(alreadyLoaded))
	for _, drp := range alreadyLoaded {
		have[strings.ToLower(drp.Filename)] = true
	}

	matches, err := filepath.Glob(filepath.Join(indexDir, "_*.bin"))
	if err != nil {
		return nil, fmt.Errorf("scanning %s: %w", indexDir, err)
	}

	var pending []*indexing.Driverpack
	for _, path := range matches {
		packFilename := expectedPackFilename(filepath.Base(path))
		if have[strings.ToLower(packFilename)] {
			continue
		}

		idx, err := loadIndex(path)
		if err != nil {
			continue // a corrupt or incomplete pending index shouldn't block the rest from loading
		}
		pending = append(pending, &indexing.Driverpack{Filename: packFilename, Index: idx, Pending: true})
	}
	return pending, nil
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
