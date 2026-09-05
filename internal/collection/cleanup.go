package collection

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// oldDriverPackRevision matches a driver-pack filename's trailing
// revision segment: exactly 5 characters between the last underscore
// and ".7z", ported from del_old_driverpacks.bat's "?????.7z" glob -
// the batch file wildcards exactly 5 characters there rather than
// requiring digits, so this does the same instead of assuming a
// numeric revision.
var oldDriverPackRevision = regexp.MustCompile(`(?i)^(.+)_.{5}\.7z$`)

// OldDriverPack is one superseded driver-pack file: same base name as
// KeptFile, but not the highest-revision one on disk.
type OldDriverPack struct {
	Path     string // full path to the superseded file
	KeptFile string // base filename of the one being kept instead
}

// FindOldDriverPacks groups every "<prefix>_?????.7z" file directly
// under dir by prefix (the batch file's :clean3 through :clean7
// cases, which only differ in how many underscore-delimited segments
// the prefix has) and returns every file that isn't the
// alphabetically-last one in its group - matching
// del_old_driverpacks.bat's own "dir /b /on, keep the last line"
// logic for picking the most recent revision. Files whose name
// doesn't end in a 5-character revision segment aren't touched,
// same as the batch file leaves them alone (no :cleanN case matches).
func FindOldDriverPacks(dir string) ([]OldDriverPack, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", dir, err)
	}

	groups := map[string][]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		m := oldDriverPackRevision.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		key := strings.ToLower(m[1])
		groups[key] = append(groups[key], name)
	}

	var old []OldDriverPack
	for _, names := range groups {
		if len(names) < 2 {
			continue
		}
		sort.Strings(names)
		keep := names[len(names)-1]
		for _, name := range names[:len(names)-1] {
			old = append(old, OldDriverPack{Path: filepath.Join(dir, name), KeptFile: keep})
		}
	}
	return old, nil
}

// CleanOldDriverPacks reports (and, unless dryRun, removes) every
// superseded driver-pack file FindOldDriverPacks finds under dir,
// ported from del_old_driverpacks.bat. Returns how many files were
// found (dryRun) or actually removed.
func CleanOldDriverPacks(dir string, dryRun bool, out io.Writer) (int, error) {
	old, err := FindOldDriverPacks(dir)
	if err != nil {
		return 0, err
	}

	verb := "would remove"
	if !dryRun {
		verb = "removing"
	}
	removed := 0
	for _, o := range old {
		fmt.Fprintf(out, "%s %s (superseded by %s)\n", verb, filepath.Base(o.Path), o.KeptFile)
		if dryRun {
			continue
		}
		if err := os.Remove(o.Path); err != nil {
			fmt.Fprintf(out, "warning: removing %s: %v\n", o.Path, err)
			continue
		}
		removed++
	}
	if dryRun {
		return len(old), nil
	}
	return removed, nil
}
