package matcher

import "testing"

func TestCmpUnsigned(t *testing.T) {
	cases := []struct {
		a, b uint32
		want int
	}{
		{1, 2, -1},
		{2, 1, 1},
		{5, 5, 0},
		{0xFFFFFFFF, 0, 1},
	}
	for _, c := range cases {
		if got := CmpUnsigned(c.a, c.b); got != c.want {
			t.Errorf("CmpUnsigned(%d, %d) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestIsValidVer(t *testing.T) {
	cases := []struct {
		v1, major, minor int
		want             bool
	}{
		{5, 5, 1, true},
		{5, 6, 1, false},
		{6, 5, 1, false},
		{6, 6, 1, true},
		{6, 10, 0, true},
		{106, 6, 0, true},
		{106, 6, 1, false},
		{106, 10, 0, false},
		{0, 5, 1, true}, // default: no restriction
	}
	for _, c := range cases {
		if got := IsValidVer(c.v1, c.major, c.minor); got != c.want {
			t.Errorf("IsValidVer(%d, %d, %d) = %v, want %v", c.v1, c.major, c.minor, got, c.want)
		}
	}
}

func TestIsBlacklisted(t *testing.T) {
	if !IsBlacklisted("PCI\\VEN_168C&DEV_002B&SUBSYS_30A117AA", "Realtek.NTamd64",
		RealtekBlacklistHWID, RealtekBlacklistSection) {
		t.Error("expected the Realtek blacklist entry to match")
	}
	if IsBlacklisted("PCI\\VEN_168C&DEV_002B&SUBSYS_30A117AA", "SomeOtherVendor.NTamd64",
		RealtekBlacklistHWID, RealtekBlacklistSection) {
		t.Error("expected no match: hwid matches but section doesn't")
	}
	if IsBlacklisted("PCI\\VEN_10EC&DEV_8168", "Realtek.NTamd64",
		RealtekBlacklistHWID, RealtekBlacklistSection) {
		t.Error("expected no match: section matches but hwid doesn't")
	}
}

func TestIsValidUSB3Hub(t *testing.T) {
	if !IsValidUSB3Hub(`IUSB3\ROOT_HUB30&VID_8086&PID_1E31&REV_0004`, IntelUSB3Gen2HubIDs) {
		t.Error("expected a gen2 hub ID (with extra &REV_ suffix) to match")
	}
	if IsValidUSB3Hub(`IUSB3\ROOT_HUB30&VID_8086&PID_9999`, IntelUSB3Gen2HubIDs) {
		t.Error("expected an unlisted PID not to match")
	}
	if !IsValidUSB3Hub(`iusb3\root_hub30&vid_8086&pid_8c31`, IntelUSB3Gen4HubIDs) {
		t.Error("expected a case-insensitive match")
	}
}

func TestIntelPathUsesSDIPrefix(t *testing.T) {
	if IntelPathUsesSDIPrefix(0) {
		t.Error("packVersion=0 (not found) should use the plain prefix")
	}
	if IntelPathUsesSDIPrefix(16073) {
		t.Error("16073 should use the plain prefix (boundary is exclusive)")
	}
	if !IntelPathUsesSDIPrefix(16074) {
		t.Error("16074 should use the _sdi_ prefix")
	}
}

func TestCalcNotebookValidNonLaptopSection(t *testing.T) {
	if !CalcNotebookValid(`c:\drivers\Ports\dtport.inf`, false, "") {
		t.Error("a non-laptop-restricted section should always be valid")
	}
}

func TestCalcNotebookValidRequiresLaptop(t *testing.T) {
	if CalcNotebookValid(`c:\drivers\touchpad_mouse\synaptics.inf`, false, "Dell") {
		t.Error("a laptop-only section must be rejected on non-laptop hardware")
	}
}

func TestCalcNotebookValidRequiresMarker(t *testing.T) {
	if CalcNotebookValid(`c:\drivers\Elan_nb\touchpad.inf`, true, "") {
		t.Error("a laptop-only section with no OEM marker must be rejected")
	}
}

func TestCalcNotebookValidMarkerMatch(t *testing.T) {
	if !CalcNotebookValid(`c:\drivers\Dell_nb\touchpad.inf`, true, "Dell") {
		t.Error("expected a matching OEM marker in the path to be accepted")
	}
	if CalcNotebookValid(`c:\drivers\HP_nb\touchpad.inf`, true, "Dell") {
		t.Error("expected a non-matching OEM marker to be rejected")
	}
}

func TestResultCmpOrdersByAltSectScoreFirst(t *testing.T) {
	better := Result{AltSectScore: 2, Score: 1}
	worse := Result{AltSectScore: 1, Score: 100}
	if better.Cmp(worse) <= 0 {
		t.Error("higher AltSectScore should win regardless of Score")
	}
}

func TestResultCmpFallsBackToScoreDescending(t *testing.T) {
	higherScore := Result{AltSectScore: 1, Score: 100}
	lowerScore := Result{AltSectScore: 1, Score: 1}
	if higherScore.Cmp(lowerScore) >= 0 {
		t.Error("with equal AltSectScore, a higher Score should sort first (Cmp < 0)")
	}
}

func TestResultCmpIgnoresDupBitInStatusTiebreak(t *testing.T) {
	a := Result{Status: StatusCurrent | StatusDup}
	b := Result{Status: StatusCurrent}
	if a.Cmp(b) != 0 {
		t.Errorf("Cmp() = %d, want 0 (StatusDup must be masked out of the tiebreak)", a.Cmp(b))
	}
}

func TestResultIsDup(t *testing.T) {
	a := Result{InfCRC: 42, HWID: "PCI\\VEN_1", Section: "Foo.NTamd64"}
	b := Result{InfCRC: 42, HWID: "PCI\\VEN_1", Section: "Foo.NTamd64"}
	if !a.IsDup(b) {
		t.Error("expected identical InfCRC/HWID/Section to be a dup")
	}
	c := Result{InfCRC: 43, HWID: "PCI\\VEN_1", Section: "Foo.NTamd64"}
	if a.IsDup(c) {
		t.Error("expected differing InfCRC not to be a dup")
	}
}

func TestResultIsDriverValid(t *testing.T) {
	if !(Result{AltSectScore: 1, DecorScore: 1}).IsDriverValid() {
		t.Error("expected positive AltSectScore and DecorScore to be valid")
	}
	if (Result{AltSectScore: 0, DecorScore: 1}).IsDriverValid() {
		t.Error("expected AltSectScore==0 to be invalid")
	}
	if (Result{AltSectScore: 1, DecorScore: 0}).IsDriverValid() {
		t.Error("expected DecorScore==0 to be invalid")
	}
}
