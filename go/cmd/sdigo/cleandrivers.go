package main

import (
	"flag"
	"fmt"
	"os"

	"sdio/internal/collection"
	"sdio/internal/settings"
)

// cleanDrivers removes superseded driver-pack files ("sdigo
// cleandrivers"), replacing del_old_driverpacks.bat - a batch file
// has no place to report progress or ask before deleting, so this
// lists what it found and only deletes with -delete, unlike the batch
// file's unconditional del.
func cleanDrivers(args []string) int {
	s := settings.New()
	if _, err := s.LoadDefaultCfgResolved(); err != nil {
		fmt.Fprintln(os.Stderr, "warning: loading sdio.cfg:", err)
	}
	s.ExpandDirs()

	fs := flag.NewFlagSet("cleandrivers", flag.ContinueOnError)
	drpDir := fs.String("drp-dir", s.DrpDir, "driver pack directory to clean")
	del := fs.Bool("delete", false, "actually remove superseded driver packs (default: list only)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	n, err := collection.CleanOldDriverPacks(*drpDir, !*del, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if n == 0 {
		fmt.Fprintln(os.Stdout, "nothing to clean up")
	} else if !*del {
		fmt.Fprintf(os.Stdout, "\n%d file(s) would be removed - rerun with -delete to actually remove them\n", n)
	}
	return 0
}
