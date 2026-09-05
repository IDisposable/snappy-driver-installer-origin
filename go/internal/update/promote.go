package update

import (
	"os"
	"path/filepath"
)

// PromotePendingIndex renames a driver pack's pending (underscore-
// prefixed) index file to its final DP_*.bin name, once that pack's
// own .7z has finished downloading. A pending index's HWID content is
// already the real, final index data - see
// collection.LoadOnlineIndexes - only its .7z payload was still in
// flight, so once that payload lands, the pack should be matched via
// its real index on every future scan instead of reporting a missing
// index (and, since the pack is no longer treated as pending, no
// longer needing a download at all). Does nothing (not an error) if
// no pending index for packFilename exists, e.g. -reindex builds one
// locally instead.
//
// Uses SaveFile rather than a bare os.Rename: reported live, renaming
// straight after a driver pack's own .7z had just landed failed with
// "Access is denied" on Windows for several packs in the same batch -
// the same transient just-written-file lock (antivirus scanning it,
// or a lingering handle) SaveFile's own retry loop already exists to
// ride out, not something specific to the torrent client's data
// directory as its doc comment's original context implied.
func PromotePendingIndex(indexDir, packFilename string) error {
	ext := filepath.Ext(packFilename)
	pendingPath := filepath.Join(indexDir, "_"+packFilename[1:len(packFilename)-len(ext)]+".bin")
	if _, err := os.Stat(pendingPath); err != nil {
		return nil
	}
	realPath := filepath.Join(indexDir, packFilename[:len(packFilename)-len(ext)]+".bin")
	return SaveFile(pendingPath, realPath)
}
