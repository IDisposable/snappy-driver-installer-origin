package indexing

import (
	"strings"

	"sdio/internal/common"
)

// Driverpack is one indexed driver pack: its on-disk path plus the
// decoded index built from its indexes/**/*.bin file. It wraps Index
// with the "join" navigation (HWID -> Desc -> Manufacturer -> InfFile)
// scoring code needs to go from a hardware-ID index straight to the
// strings/version/manufacturer it resolves to, without every caller
// re-deriving that chain itself. Building a new index from scratch
// (genindex's write-side orchestration) isn't part of this type or
// ported anywhere else - see docs/PORTING_NOTES.md.
type Driverpack struct {
	Path     string
	Filename string
	Index    *Index

	// Pending is true when this pack's index was loaded but its .7z
	// file isn't present locally yet (DRIVERPACK_TYPE_UPDATE in the
	// original - see Collection::loadOnlineIndexes and
	// Hwidmatch::getdrp_packontorrent). A device can still be matched
	// against a pending pack (its index is fully loaded), but
	// installing it needs to download Filename first - see
	// docs/PORTING_NOTES.md's update.cpp entry.
	Pending bool
}

// entry bundles the three record indices a HWID entry resolves to,
// computed once per lookup instead of being re-derived by every
// getter, unlike the original (each getdrp_* getter repeats the same
// three-step lookup independently).
type entry struct {
	descIndex    uint32
	manufIndex   uint32
	inffileIndex uint32
}

func (d *Driverpack) resolve(hwidIndex int) entry {
	h := d.Index.HWIDs[hwidIndex]
	desc := d.Index.Descs[h.DescIndex]
	manuf := d.Index.Manufacturers[desc.ManufacturerIndex]
	return entry{descIndex: h.DescIndex, manufIndex: desc.ManufacturerIndex, inffileIndex: manuf.InffileIndex}
}

// SectionAtPos returns the pos'th section-name variant recorded for a
// manufacturer (0 is the undecorated root section; pos>0 returns
// "<root>.<decoration>").
func (d *Driverpack) SectionAtPos(manufIndex, pos int) string {
	m := d.Index.Manufacturers[manufIndex]
	offsets := readOffsets(d.Index.Text.Data, m.Sections, int(m.SectionsN))
	if pos == 0 {
		return d.Index.Text.GetString(offsets[0])
	}
	return d.Index.Text.GetString(offsets[0]) + "." + d.Index.Text.GetString(offsets[pos])
}

// readOffsets reads n consecutive little-endian uint32 text-blob
// offsets starting at start.
func readOffsets(data []byte, start ofst, n int) []ofst {
	out := make([]ofst, n)
	for i := 0; i < n; i++ {
		p := int(start) + i*4
		if p+4 > len(data) {
			break
		}
		out[i] = ofst(data[p]) | ofst(data[p+1])<<8 | ofst(data[p+2])<<16 | ofst(data[p+3])<<24
	}
	return out
}

// Section returns the install section a HWID entry was found under.
func (d *Driverpack) Section(hwidIndex int) string {
	e := d.resolve(hwidIndex)
	desc := d.Index.Descs[e.descIndex]
	return d.SectionAtPos(int(e.manufIndex), int(desc.SectPos))
}

// InfPath returns the .inf file's path within the driver pack.
func (d *Driverpack) InfPath(hwidIndex int) string {
	e := d.resolve(hwidIndex)
	return d.Index.Text.GetString(d.Index.InfFiles[e.inffileIndex].InfPath)
}

// InfName returns the .inf file's name.
func (d *Driverpack) InfName(hwidIndex int) string {
	e := d.resolve(hwidIndex)
	return d.Index.Text.GetString(d.Index.InfFiles[e.inffileIndex].InfFilename)
}

// Field returns .inf version-block field n (see the Field* constants),
// or "" if unset.
func (d *Driverpack) Field(hwidIndex, n int) string {
	e := d.resolve(hwidIndex)
	off := d.Index.InfFiles[e.inffileIndex].Fields[n]
	if off == 0 {
		return ""
	}
	return d.Index.Text.GetString(off)
}

// Cat returns catalog-file slot n's OS-attribute string (see FindOSAttr),
// or "" if this driver pack has no such catalog file.
func (d *Driverpack) Cat(hwidIndex, n int) string {
	e := d.resolve(hwidIndex)
	off := d.Index.InfFiles[e.inffileIndex].Cats[n]
	if off == 0 {
		return ""
	}
	return d.Index.Text.GetString(off)
}

// Version returns the .inf file's declared driver version.
func (d *Driverpack) Version(hwidIndex int) common.Version {
	e := d.resolve(hwidIndex)
	rv := d.Index.InfFiles[e.inffileIndex].Version
	return common.Version{
		Day: int(rv.Day), Month: int(rv.Month), Year: int(rv.Year),
		V1: int(rv.V1), V2: int(rv.V2), V3: int(rv.V3), V4: int(rv.V4),
	}
}

// InfSize returns the .inf file's size in bytes.
func (d *Driverpack) InfSize(hwidIndex int) int32 {
	e := d.resolve(hwidIndex)
	return d.Index.InfFiles[e.inffileIndex].InfSize
}

// InfCRC returns the .inf file's CRC.
func (d *Driverpack) InfCRC(hwidIndex int) int32 {
	e := d.resolve(hwidIndex)
	return d.Index.InfFiles[e.inffileIndex].InfCRC
}

// Manufacturer returns the [Manufacturer]-section entry's name.
func (d *Driverpack) Manufacturer(hwidIndex int) string {
	e := d.resolve(hwidIndex)
	return d.Index.Text.GetString(d.Index.Manufacturers[e.manufIndex].Manufacturer)
}

// Desc returns the device description line's text.
func (d *Driverpack) Desc(hwidIndex int) string {
	e := d.resolve(hwidIndex)
	return d.Index.Text.GetString(d.Index.Descs[e.descIndex].Desc)
}

// Install returns the device description line's raw (unresolved)
// install-section name.
func (d *Driverpack) Install(hwidIndex int) string {
	e := d.resolve(hwidIndex)
	return d.Index.Text.GetString(d.Index.Descs[e.descIndex].Install)
}

// InstallPicked returns the resolved install section SDIO picked for
// this device line (see ResolveManufacturerSection).
func (d *Driverpack) InstallPicked(hwidIndex int) string {
	e := d.resolve(hwidIndex)
	return d.Index.Text.GetString(d.Index.Descs[e.descIndex].InstallPicked)
}

// Feature returns the device line's feature-score byte: a "feature_N"
// segment in the .inf path overrides the indexed value.
func (d *Driverpack) Feature(hwidIndex int) int {
	if i := strings.Index(strings.ToLower(d.InfPath(hwidIndex)), "feature_"); i >= 0 {
		return atoiPrefix(d.InfPath(hwidIndex)[i+len("feature_"):])
	}
	e := d.resolve(hwidIndex)
	return int(d.Index.Descs[e.descIndex].Feature & 0xFF)
}

// atoiPrefix parses the leading run of decimal digits in s, C atoi()
// style: 0 if s doesn't start with a digit, no error for trailing junk.
func atoiPrefix(s string) int {
	n := 0
	for i := 0; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		n = n*10 + int(s[i]-'0')
	}
	return n
}

// InfPos returns the HWID entry's position within its .inf file's
// hardware-ID list.
func (d *Driverpack) InfPos(hwidIndex int) int32 {
	return d.Index.HWIDs[hwidIndex].InfPos
}

// HWID returns the hardware ID string itself.
func (d *Driverpack) HWID(hwidIndex int) string {
	return d.Index.Text.GetString(d.Index.HWIDs[hwidIndex].HWID)
}

// CatalogFileBits reports which catalog-file fields (FieldCatalogFile..
// FieldCatalogFileNTAMD64) are non-empty for this HWID entry's .inf
// file, as a bitmask (1<<field). The bit positions match
// matcher.CatalogFileBit..CatalogFileNTAMD64Bit.
func (d *Driverpack) CatalogFileBits(hwidIndex int) int {
	r := 0
	for i := FieldCatalogFile; i <= FieldCatalogFileNTAMD64; i++ {
		if d.Field(hwidIndex, i) != "" {
			r |= 1 << i
		}
	}
	return r
}
