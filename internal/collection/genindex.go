package collection

import (
	"fmt"
	"os"

	"sdio/internal/archive"
	"sdio/internal/indexing"
	"sdio/internal/matcher"
	"sdio/internal/sdwfile"
)

// indexFormatVersion is VER_INDEX from main.h - the format version
// SaveIndex writes and loadIndex/PeekVersion expect indexes/**/*.bin
// files to carry.
const indexFormatVersion = 0x205

// BuildIndexFromArchive scans packPath's .7z contents into a fresh
// index, ported from Driverpack::genindex's orchestration
// (indexing.cpp): extract every .inf, parse it with the same pipeline
// ScanInstalledInf/LoadCollection's read side already uses, and
// record every manufacturer/section/hardware-ID found. Also extracts
// every .cat file in the pack so indexing.BuildIndex can cross-
// reference each .inf's declared CatalogFile* value against its
// signed OS-attribute string (see indexing.BuildIndex's catFiles
// parameter) - a pack with no .cat files at all (genuinely unsigned)
// still indexes fine, just scores as uncatalogued like the original
// would for the same pack. Does not write anything - see SaveIndex.
func BuildIndexFromArchive(packPath string) (*indexing.Index, error) {
	r, err := archive.Open(packPath)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	infFiles := map[string][]byte{}
	catFiles := map[string][]byte{}
	for _, f := range r.Files() {
		switch {
		case archive.HasSuffixFold(f.Name, ".inf"):
			data, err := r.Extract(f.Name)
			if err != nil {
				return nil, fmt.Errorf("extracting %s: %w", f.Name, err)
			}
			infFiles[f.Name] = data
		case archive.HasSuffixFold(f.Name, ".cat"):
			data, err := r.Extract(f.Name)
			if err != nil {
				return nil, fmt.Errorf("extracting %s: %w", f.Name, err)
			}
			catFiles[f.Name] = data
		}
	}
	if len(infFiles) == 0 {
		return nil, fmt.Errorf("no .inf files found in %s", packPath)
	}

	return indexing.BuildIndex(infFiles, catFiles, matcher.OSDecorations[:]), nil
}

// SaveIndex writes idx to indexPath in the same "SDW" + LZMA container
// format (see internal/sdwfile) every real indexes/**/*.bin file uses,
// so it loads back through the exact same loadIndex/DecodeIndex path
// as one shipped with a driver pack.
func SaveIndex(idx *indexing.Index, indexPath string) error {
	payload, err := indexing.EncodeIndex(idx)
	if err != nil {
		return fmt.Errorf("encoding index: %w", err)
	}

	f, err := os.Create(indexPath)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := sdwfile.Encode(f, indexFormatVersion, payload, true); err != nil {
		return fmt.Errorf("writing %s: %w", indexPath, err)
	}
	return nil
}
