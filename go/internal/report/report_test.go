package report

import (
	"bytes"
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
	if !strings.Contains(out, "MISSING") || !strings.Contains(out, "Mystery Device") {
		t.Errorf("Print() output missing the MISSING device line: %s", out)
	}
	if !strings.Contains(out, "1 devices matched, 1 missing/no driver found") {
		t.Errorf("Print() summary line wrong: %s", out)
	}

	if len(pending) != 1 || pending[0].Description != "Widget Controller" {
		t.Errorf("Print() pending = %+v, want exactly the Widget Controller candidate", pending)
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
