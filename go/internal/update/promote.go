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
func PromotePendingIndex(indexDir, packFilename string) error {
	ext := filepath.Ext(packFilename)
	pendingPath := filepath.Join(indexDir, "_"+packFilename[1:len(packFilename)-len(ext)]+".bin")
	if _, err := os.Stat(pendingPath); err != nil {
		return nil
	}
	realPath := filepath.Join(indexDir, packFilename[:len(packFilename)-len(ext)]+".bin")
	return os.Rename(pendingPath, realPath)
}
