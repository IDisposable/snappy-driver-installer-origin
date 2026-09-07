package indexing

import (
	"strings"

	"sdio/internal/common"
)

// TxtBuilder builds the string and offset blob for a new index. It appends
// values because offsets only need stable byte locations.
type TxtBuilder struct {
	buf []byte
}

// InternString appends s null-terminated and returns its offset.
func (b *TxtBuilder) InternString(s string) ofst {
	off := ofst(len(b.buf))
	b.buf = append(b.buf, s...)
	b.buf = append(b.buf, 0)
	return off
}

// InternOffsets appends a sequence of raw little-endian uint32 text-
// blob offsets and returns the offset of the first one - the format
// Manufacturer.Sections points at (see Driverpack.SectionAtPos/
// readOffsets): offsets[0] is the section root name, offsets[1:] are
// decoration suffixes ("ntamd64", not "root.ntamd64").
func (b *TxtBuilder) InternOffsets(offsets []ofst) ofst {
	start := ofst(len(b.buf))
	for _, o := range offsets {
		b.buf = append(b.buf, byte(o), byte(o>>8), byte(o>>16), byte(o>>24))
	}
	return start
}

// Bytes returns the interned blob so far.
func (b *TxtBuilder) Bytes() []byte { return b.buf }

// toRawVersion converts a parsed common.Version to the on-disk field
// layout InfFile.Version stores - a plain field-for-field copy, since
// both already use the same "V1<0 means unset" convention.
func toRawVersion(v common.Version) rawVersion {
	return rawVersion{
		Day: int32(v.Day), Month: int32(v.Month), Year: int32(v.Year),
		V1: int32(v.V1), V2: int32(v.V2), V3: int32(v.V3), V4: int32(v.V4),
	}
}

// splitInfPath splits a 7z archive entry's path (forward slashes, e.g.
// "dt/allx64/dtport.inf") into the backslash-separated directory (with
// a trailing backslash) and bare filename InfFile.InfPath/InfFilename
// store, matching the convention Driverpack.InfPath/InfName already
// read back out.
func splitInfPath(archivePath string) (dir, name string) {
	winPath := strings.ReplaceAll(archivePath, "/", `\`)
	i := strings.LastIndex(winPath, `\`)
	if i < 0 {
		return "", winPath
	}
	return winPath[:i+1], winPath[i+1:]
}

// catIndex is a lookup from a .inf's declared CatalogFile* value to
// the matching .cat file's OS-attribute string (see FindOSAttr).
// catIndex maps each catalog file's directory-qualified name to its
// embedded OS-attribute string. Directory-qualified keys avoid collisions
// between same-named catalogs in different driver folders.
type catIndex map[string]string

// buildCatIndex runs FindOSAttr once per .cat file in catFiles (keyed
// by its archive path, forward-slash-separated, converted to the same
// backslash-plus-trailing-slash directory/bare-filename split
// splitInfPath already uses for .inf entries), keyed by the lowercased
// concatenation of the two - see catIndex's doc comment.
func buildCatIndex(catFiles map[string][]byte) catIndex {
	ci := make(catIndex, len(catFiles))
	for catPath, data := range catFiles {
		dir, name := splitInfPath(catPath)
		ci[strings.ToLower(dir+name)] = FindOSAttr(data)
	}
	return ci
}

// lookup resolves rawFieldValue (a .inf's declared CatalogFile* value,
// exactly as written, no basename extraction) against a .cat found in
// infDir (the .inf's own directory, in InfFile.InfPath's backslash-
// plus-trailing-slash form) - see catIndex's doc comment.
func (ci catIndex) lookup(infDir, rawFieldValue string) string {
	return ci[strings.ToLower(infDir+rawFieldValue)]
}

// BuildIndex scans every extracted .inf file into a fresh Index. It
// discovers sections, resolves manufacturer variants, and records one
// Desc/HWID pair for each device line and hardware ID. osDecorationSuffixes
// should be
// matcher.OSDecorations[:] (accepted as a parameter, not imported
// directly, to avoid an internal/matcher <-> internal/indexing import
// cycle - ScanInstalledInf already does the same).
//
// catFiles (keyed the same way as infFiles) is every .cat file
// extracted from the same pack, used to populate InfFile.Cats: for
// each non-empty CatalogFile*/Fields entry, the corresponding .cat
// file's embedded OS-attribute string (see FindOSAttr/catIndex)
// is interned into the same slot, so a freshly built index scores
// catalog validity exactly like one shipped with the pack (see
// IsValidCatForDriver). Pass nil/empty if the pack's .cat files aren't
// available - every entry then scores as uncatalogued, same as before
// this was wired up.
func BuildIndex(infFiles map[string][]byte, catFiles map[string][]byte, osDecorationSuffixes []string) *Index {
	idx := &Index{}
	var txt TxtBuilder
	cats := buildCatIndex(catFiles)

	for infPath, data := range infFiles {
		dir, name := splitInfPath(infPath)

		sections, crc := DiscoverSections(data)
		strs := ParseStrings(data, sections)
		verInfo := ParseVersionSection(data, sections, strs)

		inffileIndex := uint32(len(idx.InfFiles))
		infFile := InfFile{
			InfPath:     txt.InternString(dir),
			InfFilename: txt.InternString(name),
			Version:     toRawVersion(verInfo.Version),
			InfSize:     int32(len(data)),
			InfCRC:      int32(crc),
		}
		for i := 0; i < NumVerNames; i++ {
			if verInfo.Fields[i] == "" {
				continue
			}
			infFile.Fields[i] = txt.InternString(verInfo.Fields[i])
			if i < FieldCatalogFile || i > FieldCatalogFileNTAMD64 {
				continue
			}
			if attr := cats.lookup(dir, verInfo.Fields[i]); attr != "" {
				infFile.Cats[i] = txt.InternString(attr)
			}
		}
		idx.InfFiles = append(idx.InfFiles, infFile)

		for _, me := range ParseManufacturers(data, sections, strs) {
			if len(me.Sections) == 0 || me.SectionRoot == "" {
				continue
			}
			manufIndex := uint32(len(idx.Manufacturers))

			offsets := make([]ofst, len(me.Sections))
			offsets[0] = txt.InternString(me.SectionRoot)
			for i := 1; i < len(me.Sections); i++ {
				suffix := strings.TrimPrefix(strings.TrimPrefix(me.Sections[i], me.SectionRoot), ".")
				offsets[i] = txt.InternString(suffix)
			}
			idx.Manufacturers = append(idx.Manufacturers, Manufacturer{
				InffileIndex: inffileIndex,
				Manufacturer: txt.InternString(me.Name),
				Sections:     txt.InternOffsets(offsets),
				SectionsN:    int32(len(offsets)),
			})

			for pos, secName := range me.Sections {
				lastDecoration := ""
				if pos > 0 {
					lastDecoration = strings.TrimPrefix(strings.TrimPrefix(secName, me.SectionRoot), ".")
				}
				for _, dev := range ResolveManufacturerSection(data, sections, secName, lastDecoration, strs, osDecorationSuffixes) {
					descIndex := uint32(len(idx.Descs))
					idx.Descs = append(idx.Descs, Desc{
						ManufacturerIndex: manufIndex,
						SectPos:           int32(pos),
						Desc:              txt.InternString(dev.Description),
						Install:           txt.InternString(dev.Install),
						InstallPicked:     txt.InternString(dev.InstallPicked),
						Feature:           uint32(dev.Feature),
					})
					for hwidPos, hwid := range dev.HWIDs {
						idx.HWIDs = append(idx.HWIDs, HWID{
							DescIndex: descIndex,
							InfPos:    int32(hwidPos),
							HWID:      txt.InternString(hwid),
						})
					}
				}
			}
		}
	}

	idx.Text = Txt{Data: txt.Bytes()}
	idx.Hashes = buildHashtable(idx)
	return idx
}

// buildHashtable builds the HWID search hashtable a fresh Index needs,
// ported from Driverpack::genhashes' second half. Keys are hashed
// uppercased, matching collection.Match's own uppercase-before-hash
// lookup (internal/collection/match.go) - a lowercase or mixed-case
// key here would make every real lookup miss.
func buildHashtable(idx *Index) Hashtable {
	var h Hashtable
	h.Reset(int32(len(idx.HWIDs)/2 + 1))
	for i, hw := range idx.HWIDs {
		key := int32(APHash([]byte(strings.ToUpper(idx.Text.GetString(hw.HWID)))))
		h.AddItem(key, int32(i))
	}
	return h
}
