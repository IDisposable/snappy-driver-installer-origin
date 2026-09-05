package hardware

import "testing"

func TestMatchDeviceIDHardwareMatch(t *testing.T) {
	hw := []string{`PCI\VEN_8086&DEV_1000`, `PCI\VEN_8086&DEV_1001`}
	pos, isHW := MatchDeviceID(hw, nil, `PCI\VEN_8086&DEV_1001`)
	if pos != 1 || !isHW {
		t.Errorf("MatchDeviceID = %d, %v; want 1, true", pos, isHW)
	}
}

func TestMatchDeviceIDCaseInsensitive(t *testing.T) {
	hw := []string{`PCI\VEN_8086&DEV_1000`}
	pos, isHW := MatchDeviceID(hw, nil, `pci\ven_8086&dev_1000`)
	if pos != 0 || !isHW {
		t.Errorf("MatchDeviceID (case-insensitive) = %d, %v; want 0, true", pos, isHW)
	}
}

func TestIsMicrosoftDriver(t *testing.T) {
	cases := []struct {
		inst *InstalledDriver
		want bool
	}{
		{nil, false},
		{&InstalledDriver{ProviderName: "Microsoft"}, true},
		{&InstalledDriver{ProviderName: "  microsoft  "}, true},
		{&InstalledDriver{ProviderName: "Realtek"}, false},
		{&InstalledDriver{ProviderName: ""}, false},
	}
	for _, c := range cases {
		if got := IsMicrosoftDriver(c.inst); got != c.want {
			t.Errorf("IsMicrosoftDriver(%+v) = %v, want %v", c.inst, got, c.want)
		}
	}
}

func TestMatchDeviceIDFallsBackToCompatibleIDs(t *testing.T) {
	hw := []string{`PCI\VEN_8086&DEV_1000`}
	compat := []string{`PCI\CC_020000`, `PCI\CC_0200`}
	pos, isHW := MatchDeviceID(hw, compat, `PCI\CC_0200`)
	if pos != 1 || isHW {
		t.Errorf("MatchDeviceID (compatible fallback) = %d, %v; want 1, false", pos, isHW)
	}
}

func TestMatchDeviceIDNotFoundWithNoCompatibleIDs(t *testing.T) {
	hw := []string{`PCI\VEN_8086&DEV_1000`}
	pos, isHW := MatchDeviceID(hw, nil, `PCI\VEN_FFFF&DEV_FFFF`)
	// No compatible IDs to fall back to: isHardwareID stays true
	// (untouched), matching the original leaving *ishw at its initial
	// value of 1 when there's nothing to retry against.
	if pos != -1 || !isHW {
		t.Errorf("MatchDeviceID (not found, no fallback) = %d, %v; want -1, true", pos, isHW)
	}
}

func TestMatchDeviceIDNotFoundEvenWithCompatibleIDs(t *testing.T) {
	hw := []string{`PCI\VEN_8086&DEV_1000`}
	compat := []string{`PCI\CC_0200`}
	pos, isHW := MatchDeviceID(hw, compat, `PCI\VEN_FFFF&DEV_FFFF`)
	// The compatible-ID retry did happen (isHardwareID flips to false)
	// even though it also didn't find a match, matching the original:
	// *ishw is set to 0 unconditionally once the retry is attempted.
	if pos != -1 || isHW {
		t.Errorf("MatchDeviceID (not found, exhausted fallback) = %d, %v; want -1, false", pos, isHW)
	}
}
