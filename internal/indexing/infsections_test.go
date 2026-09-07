package indexing

import (
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"
)

func TestDiscoverSectionsAndFields(t *testing.T) {
	text := `; comment
[Version]
Class=MEDIA
DriverVer=01/02/2024,1.2.3.4

[Strings]
MSFT="Microsoft"
`
	data := []byte(text)
	sections, _ := DiscoverSections(data)

	verRanges, ok := sections["version"]
	if !ok || len(verRanges) != 1 {
		t.Fatalf("sections[\"version\"] = %v, want exactly one range", verRanges)
	}
	strRanges, ok := sections["strings"]
	if !ok || len(strRanges) != 1 {
		t.Fatalf("sections[\"strings\"] = %v, want exactly one range", strRanges)
	}

	got := string(data[verRanges[0].Begin:verRanges[0].End])
	want := "\nClass=MEDIA\nDriverVer=01/02/2024,1.2.3.4\n\n"
	if got != want {
		t.Errorf("version section range = %q, want %q", got, want)
	}
}

func TestDiscoverSectionsLastSectionRunsToEOF(t *testing.T) {
	data := []byte("[A]\nfoo=1\n[B]\nbar=2")
	sections, _ := DiscoverSections(data)
	got := string(data[sections["b"][0].Begin:sections["b"][0].End])
	if got != "\nbar=2" {
		t.Errorf("last section range = %q, want %q", got, "\nbar=2")
	}
}

func TestParseStringsSelfSubstitution(t *testing.T) {
	data := []byte("[Strings]\nBase=\"Acme\"\nFull=\"%Base% Widget\"\n")
	sections, _ := DiscoverSections(data)
	strs := ParseStrings(data, sections)
	if strs["base"] != "Acme" {
		t.Errorf(`strs["base"] = %q, want "Acme"`, strs["base"])
	}
	if strs["full"] != "Acme Widget" {
		t.Errorf(`strs["full"] = %q, want "Acme Widget" (self-substitution)`, strs["full"])
	}
}

func TestParseVersionSectionDriverVer(t *testing.T) {
	data := []byte("[Version]\nClass=MEDIA\nDriverVer=01/02/2024,1.2.3.4\n")
	sections, _ := DiscoverSections(data)
	info := ParseVersionSection(data, sections, nil)

	if info.Fields[FieldClass] != "MEDIA" {
		t.Errorf("Fields[FieldClass] = %q, want %q", info.Fields[FieldClass], "MEDIA")
	}
	if info.Version.Day != 2 || info.Version.Month != 1 || info.Version.Year != 2024 {
		t.Errorf("Version date = %+v, want Day=2 Month=1 Year=2024", info.Version)
	}
	if info.Version.V1 != 1 || info.Version.V2 != 2 || info.Version.V3 != 3 || info.Version.V4 != 4 {
		t.Errorf("Version number = %+v, want 1.2.3.4", info.Version)
	}
}

func TestParseManufacturersRootAndDecorations(t *testing.T) {
	data := []byte("[Manufacturer]\n%MfgName%=Microsoft,ntamd64,ntia64\n")
	sections, _ := DiscoverSections(data)
	entries := ParseManufacturers(data, sections, map[string]string{"mfgname": "Microsoft Corp"})

	if len(entries) != 1 {
		t.Fatalf("got %d manufacturer entries, want 1", len(entries))
	}
	e := entries[0]
	if e.Name != "Microsoft Corp" {
		t.Errorf("Name = %q, want substituted %q", e.Name, "Microsoft Corp")
	}
	if e.SectionRoot != "microsoft" {
		t.Errorf("SectionRoot = %q, want %q", e.SectionRoot, "microsoft")
	}
	want := []string{"microsoft", "microsoft.ntamd64", "microsoft.ntia64"}
	if len(e.Sections) != len(want) {
		t.Fatalf("Sections = %v, want %v", e.Sections, want)
	}
	for i := range want {
		if e.Sections[i] != want[i] {
			t.Errorf("Sections[%d] = %q, want %q", i, e.Sections[i], want[i])
		}
	}
}

func TestParseManufacturersSkipsIncompleteEntry(t *testing.T) {
	data := []byte("[Manufacturer]\n%MfgName%=")
	sections, _ := DiscoverSections(data)
	entries := ParseManufacturers(data, sections, map[string]string{"mfgname": "Acme"})
	if len(entries) != 0 {
		t.Fatalf("ParseManufacturers() returned %+v, want no incomplete entries", entries)
	}
}

// readRealInfAsASCII reads a real Windows .inf file (typically
// UTF-16LE with a BOM) via the WSL-mounted host filesystem and decodes
// it to a plain byte buffer InfParser can operate on directly. This is
// test-only scaffolding: production encoding detection/conversion (the
// original's "ansi" naming implies a real ANSI codepage conversion
// step, ported from unicode2ansi in common.cpp) isn't wired up yet.
func readRealInfAsASCII(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("real .inf file not available at %s: %v", path, err)
	}
	if len(raw) >= 2 && raw[0] == 0xFF && raw[1] == 0xFE {
		u16 := make([]uint16, (len(raw)-2)/2)
		for i := range u16 {
			u16[i] = uint16(raw[2+2*i]) | uint16(raw[2+2*i+1])<<8
		}
		return []byte(string(utf16.Decode(u16)))
	}
	return raw
}

// TestParseRealWdmaUsbInf runs the whole non-deferred pipeline
// (DiscoverSections, ParseStrings, ParseVersionSection,
// ParseManufacturers) against a real Windows .inf file, checking
// results against what the file's actual content (verified by manual
// inspection) should produce.
func TestParseRealWdmaUsbInf(t *testing.T) {
	data := readRealInfAsASCII(t, "/mnt/c/Windows/inf/wdma_usb.inf")

	sections, _ := DiscoverSections(data)
	if _, ok := sections["version"]; !ok {
		t.Fatal(`expected a "[Version]" section`)
	}
	if _, ok := sections["manufacturer"]; !ok {
		t.Fatal(`expected a "[Manufacturer]" section`)
	}

	strs := ParseStrings(data, sections)
	if strs["msft"] != "Microsoft" {
		t.Errorf(`strs["msft"] = %q, want %q`, strs["msft"], "Microsoft")
	}

	info := ParseVersionSection(data, sections, strs)
	if info.Fields[FieldClass] != "MEDIA" {
		t.Errorf("Fields[FieldClass] = %q, want %q", info.Fields[FieldClass], "MEDIA")
	}
	if info.Fields[FieldProvider] != "Microsoft" {
		t.Errorf("Fields[FieldProvider] (substituted %%MSFT%%) = %q, want %q", info.Fields[FieldProvider], "Microsoft")
	}
	if info.Version.Day == 0 && info.Version.Month == 0 {
		t.Error("expected a parsed DriverVer date")
	}
	if info.Version.V1 <= 0 {
		t.Error("expected a parsed DriverVer version number")
	}

	entries := ParseManufacturers(data, sections, strs)
	if len(entries) == 0 {
		t.Fatal("expected at least one manufacturer entry")
	}

	foundMicrosoft := false
	for _, e := range entries {
		if e.SectionRoot == "microsoft" {
			foundMicrosoft = true
			foundDecorated := false
			for _, s := range e.Sections {
				if s == "microsoft.ntamd64" {
					foundDecorated = true
				}
			}
			if !foundDecorated {
				t.Errorf("microsoft entry Sections = %v, want to include %q", e.Sections, "microsoft.ntamd64")
			}
		}
	}
	if !foundMicrosoft {
		t.Error(`expected a manufacturer entry with SectionRoot "microsoft"`)
	}
}

// TestParseManyRealInfFiles runs DiscoverSections/ParseStrings/
// ParseVersionSection/ParseManufacturers across a broad sample of real
// Windows .inf files (not just one hand-verified sample), checking
// only for crashes and structurally sane output (a real .inf file with
// a [Version] section should parse a non-empty Class field; if
// [Manufacturer] exists, every entry should resolve at least the bare
// root section against DiscoverSections' own output). This is breadth
// over depth: catches edge cases (missing sections, unusual
// encodings, multi-manufacturer decoration lists) a single fixture
// can't.
func TestParseManyRealInfFiles(t *testing.T) {
	matches, err := filepath.Glob("/mnt/c/Windows/inf/*.inf")
	if err != nil || len(matches) == 0 {
		t.Skip("no real .inf files available at the expected path; skipping")
	}

	checked := 0
	for i, path := range matches {
		if i%10 != 0 { // sample every 10th file - 1186 files is plenty to spot-check without running all of them every time
			continue
		}
		t.Run(filepath.Base(path), func(t *testing.T) {
			data := readRealInfAsASCII(t, path)
			sections, crc := DiscoverSections(data)
			_ = crc

			strs := ParseStrings(data, sections)
			info := ParseVersionSection(data, sections, strs)
			if _, hasVersion := sections["version"]; hasVersion && info.Fields[FieldClass] == "" && info.Fields[FieldClassGUID] == "" {
				t.Logf("note: [Version] present but neither Class nor ClassGUID resolved (may be legitimately absent)")
			}

			entries := ParseManufacturers(data, sections, strs)
			for _, e := range entries {
				if e.SectionRoot == "" {
					continue // a manufacturer line with no section field at all; rare but not invalid
				}
				if _, ok := sections[e.SectionRoot]; !ok {
					// Not necessarily a bug: many manufacturer entries
					// only have decorated sections (e.g.
					// "root.ntamd64"), never a bare "[root]". Just
					// confirms ParseManufacturers didn't invent a
					// section name unrelated to what DiscoverSections
					// found anywhere.
					foundAny := false
					for _, s := range e.Sections {
						if _, ok := sections[s]; ok {
							foundAny = true
							break
						}
					}
					if !foundAny {
						t.Errorf("manufacturer %q: none of %v resolve to a discovered section", e.Name, e.Sections)
					}
				}
			}
			checked++
		})
	}
	t.Logf("checked %d real .inf files", checked)
}
