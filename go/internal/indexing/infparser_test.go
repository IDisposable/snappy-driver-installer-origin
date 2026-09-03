package indexing

import "testing"

// Fixtures below are transcribed verbatim from a real Windows .inf file
// (C:\Windows\inf\wdma_usb.inf, read via the WSL-mounted host
// filesystem, decoded from its UTF-16LE encoding - this rewrite's own
// encoding detection/conversion isn't ported yet, so tests feed
// InfParser plain ASCII directly, matching what indexinf_ansi would
// have handed it after conversion).

func newTestParser(text string, stringList map[string]string) *InfParser {
	data := []byte(text)
	return NewInfParser(data, 0, len(data), stringList)
}

func TestParseItemAndFieldSimpleKeyValue(t *testing.T) {
	p := newTestParser("Class=MEDIA\n", nil)
	key, ok := p.ParseItem()
	if !ok || key != "Class" {
		t.Fatalf("ParseItem() = %q, %v; want %q, true", key, ok, "Class")
	}
	val, ok := p.ParseField()
	if !ok || val != "MEDIA" {
		t.Fatalf("ParseField() = %q, %v; want %q, true", val, ok, "MEDIA")
	}
	if _, ok := p.ParseField(); ok {
		t.Fatal("expected no second field")
	}
}

func TestParseItemAndFieldQuotedString(t *testing.T) {
	p := newTestParser(`Signature="$Windows NT$"`+"\n", nil)
	key, ok := p.ParseItem()
	if !ok || key != "Signature" {
		t.Fatalf("ParseItem() = %q, %v", key, ok)
	}
	val, ok := p.ParseField()
	if !ok || val != "$Windows NT$" {
		t.Fatalf("ParseField() = %q, %v; want %q, true", val, ok, "$Windows NT$")
	}
}

func TestParseFieldMultipleCommaSeparated(t *testing.T) {
	p := newTestParser("DriverVer = 08/14/2026,10.0.26100.9223\n", nil)
	if key, ok := p.ParseItem(); !ok || key != "DriverVer" {
		t.Fatalf("ParseItem() = %q, %v", key, ok)
	}
	date, ok := p.ParseField()
	if !ok || date != "08/14/2026" {
		t.Fatalf("ParseField() [date] = %q, %v", date, ok)
	}
	ver, ok := p.ParseField()
	if !ok || ver != "10.0.26100.9223" {
		t.Fatalf("ParseField() [version] = %q, %v", ver, ok)
	}
	if _, ok := p.ParseField(); ok {
		t.Fatal("expected no third field")
	}
}

func TestParseDateAndVersionFromDriverVer(t *testing.T) {
	v := ParseDate("08/14/2026")
	if v.Day != 14 || v.Month != 8 || v.Year != 2026 {
		t.Errorf("ParseDate(\"08/14/2026\") = %+v, want Day=14 Month=8 Year=2026", v)
	}

	ver := ParseVersionNumber("10.0.26100.9223")
	if ver.V1 != 10 || ver.V2 != 0 || ver.V3 != 26100 || ver.V4 != 9223 {
		t.Errorf("ParseVersionNumber(\"10.0.26100.9223\") = %+v, want 10.0.26100.9223", ver)
	}
}

func TestSubstitutionFastPath(t *testing.T) {
	p := newTestParser("Provider=%MSFT%\n", map[string]string{"msft": "Microsoft"})
	if key, ok := p.ParseItem(); !ok || key != "Provider" {
		t.Fatalf("ParseItem() = %q, %v", key, ok)
	}
	val, ok := p.ParseField()
	if !ok || val != "Microsoft" {
		t.Fatalf("ParseField() = %q, %v; want %q, true", val, ok, "Microsoft")
	}
}

func TestSubstitutionInKey(t *testing.T) {
	// [Manufacturer] line: "%MfgName%=Microsoft, ntamd64"
	p := newTestParser("%MfgName%=Microsoft, ntamd64\n", map[string]string{"mfgname": "(Generic USB Audio)"})
	key, ok := p.ParseItem()
	if !ok || key != "(Generic USB Audio)" {
		t.Fatalf("ParseItem() = %q, %v; want substituted manufacturer name", key, ok)
	}
	f1, ok := p.ParseField()
	if !ok || f1 != "Microsoft" {
		t.Fatalf("ParseField() [1] = %q, %v", f1, ok)
	}
	f2, ok := p.ParseField()
	if !ok || f2 != "ntamd64" {
		t.Fatalf("ParseField() [2] = %q, %v, want no leading space", f2, ok)
	}
}

func TestDeviceDescLineWithHWID(t *testing.T) {
	// A model section line: description key, install section, one HWID.
	line := `%USB\VID_04D2&PID_FF05.DeviceDesc%=USBAudio.NonCompliantAltec,USB\VID_04D2&PID_FF05` + "\n"
	stringList := map[string]string{`usb\vid_04d2&pid_ff05.devicedesc`: "Altec Lansing USB Audio"}
	p := newTestParser(line, stringList)

	desc, ok := p.ParseItem()
	if !ok || desc != "Altec Lansing USB Audio" {
		t.Fatalf("ParseItem() = %q, %v", desc, ok)
	}
	installSection, ok := p.ParseField()
	if !ok || installSection != "USBAudio.NonCompliantAltec" {
		t.Fatalf("ParseField() [install section] = %q, %v", installSection, ok)
	}
	hwid, ok := p.ParseField()
	if !ok || hwid != `USB\VID_04D2&PID_FF05` {
		t.Fatalf("ParseField() [hwid] = %q, %v", hwid, ok)
	}
}

func TestParseItemSkipsBlankAndCommentLines(t *testing.T) {
	text := "\n; a comment\n\nClass=MEDIA\n"
	p := newTestParser(text, nil)
	key, ok := p.ParseItem()
	if !ok || key != "Class" {
		t.Fatalf("ParseItem() = %q, %v; want to skip blank/comment lines to reach %q", key, ok, "Class")
	}
}

func TestParseItemReturnsFalseAtEndOfBlock(t *testing.T) {
	p := newTestParser("NoEqualsSignHere\n", nil)
	if _, ok := p.ParseItem(); ok {
		t.Fatal("expected ParseItem() to fail on a line with no '='")
	}
}

// TestParseWholeSection feeds an entire real manufacturer section
// (transcribed verbatim, six device lines, from wdma_usb.inf's
// [Altec.Section.ntamd64]) through the item/field loop a real caller
// would use, checking every line is extracted correctly - not just
// hand-picked single-line cases.
func TestParseWholeSection(t *testing.T) {
	section := `%USB\VID_04D2&PID_FF05.DeviceDesc%=USBAudio.NonCompliantAltec,USB\VID_04D2&PID_FF05
%USB\VID_04D2&PID_FF05.DeviceDesc%=USBAudio.NonCompliantAltec,USB\VID_04D2&PID_0305
%USB\VID_04D2&PID_FF47&MI_00.DeviceDesc%=USBAudio.Altec,USB\VID_04D2&PID_FF47&MI_00
%USB\VID_04D2&PID_FF49&MI_00.DeviceDesc%=USBAudio.Altec,USB\VID_04D2&PID_FF49&MI_00
%USB\VID_04D2&PID_0070&MI_00.DeviceDesc%=USBAudio.Altec,USB\VID_04D2&PID_0070&MI_00
%USB\VID_04D2&PID_2060&MI_00.DeviceDesc%=USBAudio.AltecPhone,USB\VID_04D2&PID_2060&MI_00
`
	wantHWIDs := []string{
		`USB\VID_04D2&PID_FF05`,
		`USB\VID_04D2&PID_0305`,
		`USB\VID_04D2&PID_FF47&MI_00`,
		`USB\VID_04D2&PID_FF49&MI_00`,
		`USB\VID_04D2&PID_0070&MI_00`,
		`USB\VID_04D2&PID_2060&MI_00`,
	}
	wantInstallSections := []string{
		"USBAudio.NonCompliantAltec",
		"USBAudio.NonCompliantAltec",
		"USBAudio.Altec",
		"USBAudio.Altec",
		"USBAudio.Altec",
		"USBAudio.AltecPhone",
	}

	p := newTestParser(section, nil) // no %DeviceDesc% substitutions needed for this check
	var gotHWIDs, gotInstalls []string
	for {
		if _, ok := p.ParseItem(); !ok {
			break
		}
		install, ok := p.ParseField()
		if !ok {
			t.Fatal("expected an install section field")
		}
		gotInstalls = append(gotInstalls, install)

		hwid, ok := p.ParseField()
		if !ok {
			t.Fatal("expected a hardware ID field")
		}
		gotHWIDs = append(gotHWIDs, hwid)
	}

	if len(gotHWIDs) != len(wantHWIDs) {
		t.Fatalf("got %d device lines, want %d", len(gotHWIDs), len(wantHWIDs))
	}
	for i := range wantHWIDs {
		if gotHWIDs[i] != wantHWIDs[i] {
			t.Errorf("hwid[%d] = %q, want %q", i, gotHWIDs[i], wantHWIDs[i])
		}
		if gotInstalls[i] != wantInstallSections[i] {
			t.Errorf("install[%d] = %q, want %q", i, gotInstalls[i], wantInstallSections[i])
		}
	}
}

func TestSubstitutionUnmatchedPercentLeftAsIs(t *testing.T) {
	p := newTestParser("Key=100%done\n", nil)
	if _, ok := p.ParseItem(); !ok {
		t.Fatal("ParseItem() failed")
	}
	val, ok := p.ParseField()
	if !ok || val != "100%done" {
		t.Fatalf("ParseField() = %q, %v; want unmatched %% left as-is", val, ok)
	}
}
