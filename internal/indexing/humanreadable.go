package indexing

import (
	"fmt"
	"io"
)

// WriteHumanReadable writes a plain-text listing of drp's index to w -
// the human-readable counterpart -index-hr asks for alongside the
// binary index. Ported in spirit, not byte-for-byte, from
// Driverpack::print_index_hr (indexing.cpp): that function's exact
// nested-loop layout (skip-detection between HWID runs, per-decoration
// counts, "!!!" markers for an unrecognized section type) is a
// debugging aid, not a compatibility-critical format - only
// indexes/**/*.bin itself needs to round-trip (see
// docs/COMPATIBILITY.md). This covers the same substance - every .inf
// file, its manufacturers and section variants, and every device's
// hardware IDs/install string/feature number - grouped by .inf file
// rather than reproducing the original's exact formatting.
func WriteHumanReadable(drp *Driverpack, w io.Writer) error {
	idx := drp.Index
	if _, err := fmt.Fprintf(w, "%s (%d inf files, %d HWIDs)\n", drp.Filename, len(idx.InfFiles), len(idx.HWIDs)); err != nil {
		return err
	}

	for infIdx, inf := range idx.InfFiles {
		if _, err := fmt.Fprintf(w, "\n%s%s\n", idx.Text.GetString(inf.InfPath), idx.Text.GetString(inf.InfFilename)); err != nil {
			return err
		}
		v := inf.Version
		if _, err := fmt.Fprintf(w, "  date %04d-%02d-%02d  version %d.%d.%d.%d\n",
			v.Year, v.Month, v.Day, v.V1, v.V2, v.V3, v.V4); err != nil {
			return err
		}

		for manufIdx, m := range idx.Manufacturers {
			if m.InffileIndex != uint32(infIdx) {
				continue
			}
			if _, err := fmt.Fprintf(w, "  {%s}\n", idx.Text.GetString(m.Manufacturer)); err != nil {
				return err
			}

			for pos := 0; pos < int(m.SectionsN); pos++ {
				if _, err := fmt.Fprintf(w, "    [%s]\n", drp.SectionAtPos(manufIdx, pos)); err != nil {
					return err
				}
				for descIdx, d := range idx.Descs {
					if d.ManufacturerIndex != uint32(manufIdx) || int(d.SectPos) != pos {
						continue
					}
					if _, err := fmt.Fprintf(w, "      %-50s -> %s (feature %#x)\n",
						idx.Text.GetString(d.Desc), idx.Text.GetString(d.InstallPicked), d.Feature); err != nil {
						return err
					}
					for _, h := range idx.HWIDs {
						if h.DescIndex != uint32(descIdx) {
							continue
						}
						if _, err := fmt.Fprintf(w, "        %-2d %s\n", h.InfPos, idx.Text.GetString(h.HWID)); err != nil {
							return err
						}
					}
				}
			}
		}
	}
	return nil
}
