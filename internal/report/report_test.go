package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"sdio/internal/collection"
	"sdio/internal/hardware"
	"sdio/internal/indexing"
	"sdio/internal/matcher"
	"sdio/internal/scan"
)

func TestPrintReportsFoundAndMissingDevices(t *testing.T) {
	drp := &indexing.Driverpack{Filename: "DP_Test_SDIO01_1.7z"}
	result := scan.Result{
		Devices: []scan.DeviceResult{
			{
				Device: hardware.Device{Description: "Widget Controller"},
				Candidates: []collection.Candidate{{
					Driverpack: drp,
					Result:     matcher.Result{AltSectScore: 2, DecorScore: 1, Status: matcher.StatusBetter},
				}},
			},
			{
				Device: hardware.Device{Description: "Mystery Device"},
				Status: matcher.StatusNFMissing,
			},
		},
	}

	var buf bytes.Buffer
	pending := Print(&buf, result)

	out := buf.String()
	if !strings.Contains(out, "Widget Controller") || !strings.Contains(out, "DP_Test_SDIO01_1.7z") {
		t.Errorf("Print() output missing the found device/driver pack: %s", out)
	}
	if !strings.Contains(out, "Missing") || !strings.Contains(out, "Mystery Device") {
		t.Errorf("Print() output missing the Missing device line: %s", out)
	}
	if !strings.Contains(out, "1 devices matched, 1 missing/no driver found") {
		t.Errorf("Print() summary line wrong: %s", out)
	}

	if len(pending) != 1 || pending[0].Description != "Widget Controller" {
		t.Errorf("Print() pending = %+v, want exactly the Widget Controller candidate", pending)
	}
}

func TestPrintJSONReportsFoundAndMissingDevices(t *testing.T) {
	drp := &indexing.Driverpack{Filename: "DP_Test_SDIO01_1.7z"}
	result := scan.Result{
		Devices: []scan.DeviceResult{
			{
				Device: hardware.Device{Description: "Widget Controller"},
				Candidates: []collection.Candidate{{
					Driverpack: drp,
					Result:     matcher.Result{AltSectScore: 2, DecorScore: 1, Status: matcher.StatusBetter},
				}},
			},
			{
				Device: hardware.Device{Description: "Mystery Device"},
				Status: matcher.StatusNFMissing,
			},
		},
	}

	var buf bytes.Buffer
	pending, err := PrintJSON(&buf, result)
	if err != nil {
		t.Fatalf("PrintJSON() error: %v", err)
	}
	if len(pending) != 1 || pending[0].Description != "Widget Controller" {
		t.Errorf("PrintJSON() pending = %+v, want exactly the Widget Controller candidate", pending)
	}

	var rep JSONReport
	if err := json.Unmarshal(buf.Bytes(), &rep); err != nil {
		t.Fatalf("PrintJSON() output isn't valid JSON: %v\noutput:\n%s", err, buf.String())
	}
	if rep.Matched != 1 || rep.Missing != 1 || len(rep.Devices) != 2 {
		t.Errorf("PrintJSON() report = %+v, want 1 matched, 1 missing, 2 devices", rep)
	}
	if rep.Devices[0].DriverPack != "DP_Test_SDIO01_1.7z" {
		t.Errorf("PrintJSON() matched device = %+v, want driver pack DP_Test_SDIO01_1.7z", rep.Devices[0])
	}
	if rep.Devices[1].Reason == "" {
		t.Errorf("PrintJSON() missing device = %+v, want a non-empty Reason", rep.Devices[1])
	}
}

func TestWriteDeviceListFormatsTextAndJSON(t *testing.T) {
	drp := &indexing.Driverpack{Filename: "DP_Test_SDIO01_1.7z"}
	result := scan.Result{Devices: []scan.DeviceResult{{
		Device:     hardware.Device{Description: "Widget Controller", InstanceID: "DEV1", HardwareIDs: []string{"PCI\\VEN_1234"}},
		Candidates: []collection.Candidate{{Driverpack: drp, Result: matcher.Result{AltSectScore: 2, DecorScore: 1, Status: matcher.StatusBetter}}},
	}}}

	var text bytes.Buffer
	if err := WriteDeviceList(&text, result); err != nil {
		t.Fatalf("WriteDeviceList() error: %v", err)
	}
	if !strings.Contains(text.String(), "status\tdescription\t") || !strings.Contains(text.String(), "Better match\tWidget Controller") {
		t.Errorf("WriteDeviceList() output = %q", text.String())
	}

	var jsonOut bytes.Buffer
	if err := WriteDeviceListJSON(&jsonOut, result); err != nil {
		t.Fatalf("WriteDeviceListJSON() error: %v", err)
	}
	var report JSONReport
	if err := json.Unmarshal(jsonOut.Bytes(), &report); err != nil {
		t.Fatalf("WriteDeviceListJSON() invalid JSON: %v", err)
	}
	if len(report.Devices) != 1 || report.Devices[0].Description != "Widget Controller" {
		t.Errorf("WriteDeviceListJSON() report = %+v", report)
	}
}

// TestPrintExcludesMicrosoftDriverFromPending confirms a matched
// device whose currently installed driver is Microsoft-provided is
// still reported as matched (with a note explaining why), but left
// out of the actionable pending list -install would otherwise install
// unconditionally with no per-device opt-out - the same safety
// reasoning as cmd/sdigo's [MS] tag and select-all exclusion.
func TestPrintExcludesMicrosoftDriverFromPending(t *testing.T) {
	drp := &indexing.Driverpack{Filename: "DP_Test_SDIO01_1.7z"}
	result := scan.Result{
		Devices: []scan.DeviceResult{
			{
				Device:    hardware.Device{Description: "Widget Controller"},
				Installed: &hardware.InstalledDriver{ProviderName: "Microsoft"},
				Candidates: []collection.Candidate{{
					Driverpack: drp,
					Result:     matcher.Result{AltSectScore: 2, DecorScore: 1, Status: matcher.StatusBetter},
				}},
			},
		},
	}

	var buf bytes.Buffer
	pending := Print(&buf, result)
	if len(pending) != 0 {
		t.Errorf("Print() pending = %+v, want empty (Microsoft-provided driver excluded)", pending)
	}
	if out := buf.String(); !strings.Contains(out, "Microsoft-provided driver") {
		t.Errorf("Print() output = %q, want a note explaining the exclusion", out)
	}

	buf.Reset()
	jsonPending, err := PrintJSON(&buf, result)
	if err != nil {
		t.Fatalf("PrintJSON() error: %v", err)
	}
	if len(jsonPending) != 0 {
		t.Errorf("PrintJSON() pending = %+v, want empty (Microsoft-provided driver excluded)", jsonPending)
	}

	var rep JSONReport
	if err := json.Unmarshal(buf.Bytes(), &rep); err != nil {
		t.Fatalf("PrintJSON() output isn't valid JSON: %v", err)
	}
	if rep.Matched != 1 {
		t.Errorf("PrintJSON() Matched = %d, want 1 (still counted, just not auto-installed)", rep.Matched)
	}
	if len(rep.Devices) != 1 || !rep.Devices[0].KeptMicrosoftDriver {
		t.Errorf("PrintJSON() devices = %+v, want KeptMicrosoftDriver=true", rep.Devices)
	}
}

func TestPrintReturnsNoPendingWhenNothingMatched(t *testing.T) {
	result := scan.Result{
		Devices: []scan.DeviceResult{
			{Device: hardware.Device{Description: "Mystery Device"}, Status: matcher.StatusNFMissing},
		},
	}
	var buf bytes.Buffer
	if pending := Print(&buf, result); len(pending) != 0 {
		t.Errorf("Print() pending = %+v, want empty", pending)
	}
}
