package indexing

import (
	"strings"

	"sdio/internal/common"
)

// SectionRange is a byte range within an .inf file's content
// corresponding to one "[Name]" section, excluding the "[Name]" header
// line itself.
type SectionRange struct {
	Begin, End int
}

// InfSections maps lowercased section names to their byte ranges. A name can map to
// more than one range if the same "[Name]" appears more than once in
// the file (legal but unusual in real .inf files).
type InfSections map[string][]SectionRange

// DiscoverSections scans data for "[Name]" section headers. It also
// returns a simple additive checksum of the file's content bytes - the
// bytes inside "[...]" brackets plus every non-blank, non-comment
// line's content, excluding the brackets and line endings themselves -
// used elsewhere to detect unchanged .inf content across driver-pack
// versions.
func DiscoverSections(data []byte) (InfSections, int32) {
	sections := InfSections{}
	var crc int32

	haveCurrent := false
	var curName string
	var curBeg int

	finish := func(end int) {
		if haveCurrent {
			sections[curName] = append(sections[curName], SectionRange{curBeg, end})
		}
	}

	i := 0
	for i < len(data) {
		switch data[i] {
		case ' ', '\t', '\n', '\r':
			i++
		case ';':
			i++
			for i < len(data) && data[i] != '\n' && data[i] != '\r' {
				i++
			}
		case '[':
			finish(i)
			i++
			nameBeg := i
			for i < len(data) && data[i] != ']' {
				crc += int32(data[i])
				i++
			}
			nameEnd := i
			if i < len(data) {
				i++ // skip ']'
			}
			curName = strings.ToLower(string(data[nameBeg:nameEnd]))
			curBeg = i
			haveCurrent = true
		default:
			for i < len(data) && data[i] != '\n' && data[i] != '\r' {
				crc += int32(data[i])
				i++
			}
		}
	}
	finish(len(data))

	return sections, crc
}

// ParseStrings extracts %name%->value substitutions from a driver
// pack's [Strings] section(s). Multiple [Strings] sections (unusual
// but legal) are merged into one map. The map being built is also fed
// back in as the substitution source while parsing it, so a later
// [Strings] entry may reference an earlier one via %name%.
func ParseStrings(data []byte, sections InfSections) map[string]string {
	result := map[string]string{}
	for _, r := range sections["strings"] {
		p := NewInfParser(data, r.Begin, r.End, result)
		for {
			key, ok := p.ParseItem()
			if !ok {
				break
			}
			if val, ok := p.ParseField(); ok {
				result[strings.ToLower(key)] = val
			}
		}
	}
	return result
}

// verFieldNames gives each Field* constant its [Version]-section key
// name, in NumVerNames order - index i here corresponds to Field
// constant i.
var verFieldNames = [NumVerNames]string{
	"classguid", "class", "provider",
	"catalogfile", "catalogfile.nt", "catalogfile.ntx86", "catalogfile.ntia64", "catalogfile.ntamd64",
	"driverver", "driverpackagedisplayname", "driverpackagetype",
}

// InfVersionInfo holds the fields of interest from an .inf file's
// [Version] section.
type InfVersionInfo struct {
	// Fields holds the raw (substituted) text of each named field in
	// verFieldNames/Field* order, "" if the field is absent.
	// FieldDriverVer is not populated here - see Version instead.
	Fields [NumVerNames]string
	// Version is set from DriverVer's date and four-part version
	// number fields, or left invalid (see common.Version) if DriverVer
	// is absent.
	Version common.Version
}

// ParseVersionSection extracts an .inf file's [Version] section.
// DriverVer is special-cased into a date+version pair (see
// InfVersionInfo.Version) rather than stored as raw text like every
// other field, since date/version comparison is exactly what driver
// ranking needs it for.
func ParseVersionSection(data []byte, sections InfSections, stringList map[string]string) InfVersionInfo {
	var info InfVersionInfo
	info.Version.SetInvalid()

	for _, r := range sections["version"] {
		p := NewInfParser(data, r.Begin, r.End, stringList)
		for {
			key, ok := p.ParseItem()
			if !ok {
				break
			}
			key = strings.ToLower(key)

			matched := -1
			for i, name := range verFieldNames {
				if name == key {
					matched = i
					break
				}
			}

			switch matched {
			case FieldDriverVer:
				if dateField, ok := p.ParseField(); ok {
					info.Version = ParseDate(dateField)
				}
				if verField, ok := p.ParseField(); ok {
					v := ParseVersionNumber(verField)
					info.Version.SetVersion(v.V1, v.V2, v.V3, v.V4)
				}
			default:
				if matched >= 0 {
					if val, ok := p.ParseField(); ok {
						info.Fields[matched] = val
					}
				}
			}

			// Discard any fields beyond what the matched case above
			// (or no match at all) already consumed.
			for {
				if _, ok := p.ParseField(); !ok {
					break
				}
			}
		}
	}
	return info
}

// ManufacturerEntry is one "%Name%=SectionRoot[,decoration]..." line
// from a driver pack's [Manufacturer] section.
type ManufacturerEntry struct {
	// Name is the %key% substituted manufacturer display name.
	Name string
	// SectionRoot is the lowercased section-name root (e.g.
	// "microsoft"), used to find that manufacturer's model sections
	// (e.g. "[Microsoft.NTamd64]").
	SectionRoot string
	// Sections lists every candidate section name to try, in a fixed
	// order that matters: the bare root first (in case the file
	// declares an undecorated "[SectionRoot]" section), then
	// "root.decoration" for each OS decoration listed on the line.
	Sections []string
}

// ParseManufacturers enumerates a driver pack's [Manufacturer]
// section(s) and the candidate section names each one implies -
// resolving those candidates against the file's actual sections to
// find the real per-device install section (with its "featurescore"
// hex flag) and building the resulting Desc/HWID records is
// resolveInstallSection's job, not this function's: the caller must
// resolve InfSections["<one of entry.Sections>"] and drive an
// InfParser over the matched section itself.
func ParseManufacturers(data []byte, sections InfSections, stringList map[string]string) []ManufacturerEntry {
	var result []ManufacturerEntry

	for _, r := range sections["manufacturer"] {
		p := NewInfParser(data, r.Begin, r.End, stringList)
		for {
			name, ok := p.ParseItem()
			if !ok {
				break
			}
			entry := ManufacturerEntry{Name: name}

			root, ok := p.ParseField()
			if ok {
				root = strings.ToLower(root)
				entry.SectionRoot = root
				entry.Sections = append(entry.Sections, root)

				for {
					decoration, ok := p.ParseField()
					if !ok {
						break
					}
					decoration = strings.ToLower(decoration)
					if decoration == "" {
						break
					}
					entry.Sections = append(entry.Sections, root+"."+decoration)
				}
			}

			if entry.SectionRoot != "" && len(entry.Sections) > 0 {
				result = append(result, entry)
			}
		}
	}
	return result
}
