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
