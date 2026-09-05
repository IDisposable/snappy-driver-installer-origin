package indexing

import (
	"strings"
	"testing"
)

// TestWriteHumanReadableSyntheticFixture confirms the text dump
// includes every real field a reader would want to see, using
// BuildIndex's own synthetic-fixture shape rather than duplicating
// syntheticInf here.
func TestWriteHumanReadableSyntheticFixture(t *testing.T) {
	infFiles := map[string][]byte{
		"synth/synth.inf": []byte(`[Version]
Signature="$Windows NT$"
Provider=%MFG%
DriverVer=01/02/2024,3.4.5.6

[Manufacturer]
%MFG%=SynthMfg

[SynthMfg]
%DEV1.DeviceDesc%=Synth.Install,SYNTH\VID_0001&PID_0002

[Synth.Install]
AddReg=Synth.AddReg

[Strings]
MFG="Synth Corp"
DEV1.DeviceDesc="Synth Widget"
`),
	}
	idx := BuildIndex(infFiles, nil, nil)
	drp := &Driverpack{Filename: "DP_Synth_1.7z", Index: idx}

	var buf strings.Builder
	if err := WriteHumanReadable(drp, &buf); err != nil {
		t.Fatalf("WriteHumanReadable() error: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"DP_Synth_1.7z", "synth.inf", "Synth Corp",
		"Synth Widget", `SYNTH\VID_0001&PID_0002`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("WriteHumanReadable() output missing %q:\n%s", want, out)
		}
	}
	lower := strings.ToLower(out)
	for _, want := range []string{"synthmfg", "synth.install"} {
		if !strings.Contains(lower, want) {
			t.Errorf("WriteHumanReadable() output missing %q:\n%s", want, out)
		}
	}
}
