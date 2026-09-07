package indexing

import (
	"fmt"
	"io"
)

// WriteHumanReadable writes a plain-text listing of drp's index to w -
// the human-readable counterpart -index-hr asks for alongside the
// binary index. This is a debugging format, not a compatibility contract.
// It lists each INF, manufacturer, section, device ID, install section, and
// feature number grouped by INF.
func WriteHumanReadable(drp *Driverpack, w io.Writer) error {
	idx := drp.Index
	manufByInf := make(map[uint32][]int)
	descByManuf := make(map[uint32][]int)
	hwidsByDesc := make(map[uint32][]HWID)
	for i, m := range idx.Manufacturers {
		manufByInf[m.InffileIndex] = append(manufByInf[m.InffileIndex], i)
	}
	for i, d := range idx.Descs {
		descByManuf[d.ManufacturerIndex] = append(descByManuf[d.ManufacturerIndex], i)
	}
	for _, h := range idx.HWIDs {
		hwidsByDesc[h.DescIndex] = append(hwidsByDesc[h.DescIndex], h)
	}
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

		for _, manufIdx := range manufByInf[uint32(infIdx)] {
			m := idx.Manufacturers[manufIdx]
			if _, err := fmt.Fprintf(w, "  {%s}\n", idx.Text.GetString(m.Manufacturer)); err != nil {
				return err
			}

			for pos := 0; pos < int(m.SectionsN); pos++ {
				if _, err := fmt.Fprintf(w, "    [%s]\n", drp.SectionAtPos(manufIdx, pos)); err != nil {
					return err
				}
				for _, descIdx := range descByManuf[uint32(manufIdx)] {
					d := idx.Descs[descIdx]
					if int(d.SectPos) != pos {
						continue
					}
					if _, err := fmt.Fprintf(w, "      %-50s -> %s (feature %#x)\n",
						idx.Text.GetString(d.Desc), idx.Text.GetString(d.InstallPicked), d.Feature); err != nil {
						return err
					}
					for _, h := range hwidsByDesc[uint32(descIdx)] {
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
