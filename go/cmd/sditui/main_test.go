package main

import (
	"testing"

	"sdio/internal/collection"
	"sdio/internal/hardware"
	"sdio/internal/indexing"
	"sdio/internal/matcher"
	"sdio/internal/scan"
	"sdio/internal/settings"
)

// TestBuildOptionItemsCoversEveryFlagAndFilter guards against the
// options screen silently dropping a config option if
// settings.FlagOptions/FilterOptions ever grow a new entry - the
// user's explicit ask was for ALL of the original GUI's config
// options, not a hand-picked subset.
func TestBuildOptionItemsCoversEveryFlagAndFilter(t *testing.T) {
	items := buildOptionItems()
	want := len(settings.FlagOptions()) + len(settings.FilterOptions())
	if len(items) != want {
		t.Fatalf("buildOptionItems() = %d items, want %d (flags+filters)", len(items), want)
	}
	flags, filters := 0, 0
	for _, it := range items {
		if it.isFlag {
			flags++
		} else {
			filters++
		}
	}
	if flags != len(settings.FlagOptions()) {
		t.Errorf("flag items = %d, want %d", flags, len(settings.FlagOptions()))
	}
	if filters != len(settings.FilterOptions()) {
		t.Errorf("filter items = %d, want %d", filters, len(settings.FilterOptions()))
	}
}

func TestOptionItemToggleFlag(t *testing.T) {
	s := settings.New()
	item := optionItem{isFlag: true, flagBit: settings.FlagAutoInstall}
	if item.checked(s) {
		t.Fatal("checked() = true before toggle, want false")
	}
	item.toggle(s)
	if !item.checked(s) {
		t.Fatal("checked() = false after toggle, want true")
	}
	item.toggle(s)
	if item.checked(s) {
		t.Fatal("checked() = true after second toggle, want false")
	}
}

func TestOptionItemToggleFilter(t *testing.T) {
	s := settings.New()
	s.Filters = settings.DefaultFilters
	item := optionItem{isFlag: false, filterBit: settings.FilterInvalid}
	if item.checked(s) {
		t.Fatal("checked() = true before toggle, want false (FilterInvalid isn't in DefaultFilters)")
	}
	item.toggle(s)
	if !item.checked(s) {
		t.Fatal("checked() = false after toggle, want true")
	}
	if s.Filters&settings.FilterBetter == 0 {
		t.Error("toggling one filter bit must not disturb the others")
	}
}

// TestVisibleDevicesHonorsFilters confirms the table only shows what
// Visible() allows, matching the "new and better match" default the
// user asked for while remaining fully driven by settings.Filters.
func TestVisibleDevicesHonorsFilters(t *testing.T) {
	drp := &indexing.Driverpack{Filename: "DP_Test_SDIO01_1.bin"}
	better := scan.DeviceResult{Device: hardware.Device{InstanceID: "better"}, Candidates: []collection.Candidate{{
		Driverpack: drp,
		Result:     matcher.Result{AltSectScore: 2, DecorScore: 1, Status: matcher.StatusBetter},
	}}}
	current := scan.DeviceResult{Device: hardware.Device{InstanceID: "current"}, Candidates: []collection.Candidate{{
		Driverpack: drp,
		Result:     matcher.Result{AltSectScore: 2, DecorScore: 1, Status: matcher.StatusCurrent},
	}}}
	devices := []scan.DeviceResult{better, current}

	visible := visibleDevices(devices, settings.DefaultFilters)
	if len(visible) != 1 {
		t.Fatalf("visibleDevices(DefaultFilters) = %d, want 1 (only the StatusBetter device)", len(visible))
	}

	visible = visibleDevices(devices, settings.DefaultFilters|settings.FilterCurrent)
	if len(visible) != 2 {
		t.Fatalf("visibleDevices(DefaultFilters|FilterCurrent) = %d, want 2", len(visible))
	}
}

// TestLayoutColumnsAddsInstalledColumnWhenWide confirms the fifth
// "Installed" column only appears once the terminal is wide enough to
// give Device/Best match reasonable room, and that column widths never
// go negative/zero on a narrow terminal.
func TestLayoutColumnsAddsInstalledColumnWhenWide(t *testing.T) {
	narrow, showNarrow := layoutColumns(80)
	if showNarrow {
		t.Error("layoutColumns(80): showInstalled = true, want false")
	}
	if len(narrow) != 5 {
		t.Fatalf("layoutColumns(80) = %d columns, want 5", len(narrow))
	}
	for _, c := range narrow {
		if c.Width <= 0 {
			t.Errorf("column %q has non-positive width %d", c.Title, c.Width)
		}
	}

	wide, showWide := layoutColumns(160)
	if !showWide {
		t.Error("layoutColumns(160): showInstalled = false, want true")
	}
	if len(wide) != 6 {
		t.Fatalf("layoutColumns(160) = %d columns, want 6", len(wide))
	}
}

// TestDeviceRowIncludesInstalledColumnOnlyWhenRequested confirms
// deviceRow's cell count always matches what layoutColumns produced,
// since bubbles/table matches row cells to columns positionally - a
// mismatch here would silently misalign or drop displayed data.
func TestDeviceRowIncludesInstalledColumnOnlyWhenRequested(t *testing.T) {
	dr := scan.DeviceResult{Device: hardware.Device{InstanceID: "d1", Description: "Test Device"}}

	narrow := deviceRow(dr, false, false)
	if len(narrow) != 5 {
		t.Fatalf("deviceRow(showInstalled=false) = %d cells, want 5", len(narrow))
	}

	wide := deviceRow(dr, false, true)
	if len(wide) != 6 {
		t.Fatalf("deviceRow(showInstalled=true) = %d cells, want 6", len(wide))
	}
	if wide[5] != "not installed" {
		t.Errorf("deviceRow installed cell = %q, want %q", wide[5], "not installed")
	}
}

// TestDeviceRowSelectionMarker confirms only an actionable (has a
// Best candidate) row can show a tick, and that the tick reflects the
// selected argument - the Sel column cmd/sditui's space-bar toggle
// depends on rendering correctly.
func TestDeviceRowSelectionMarker(t *testing.T) {
	drp := &indexing.Driverpack{Filename: "DP_Test_SDIO01_1.bin"}
	actionable := scan.DeviceResult{Candidates: []collection.Candidate{{
		Driverpack: drp,
		Result:     matcher.Result{AltSectScore: 2, DecorScore: 1, Status: matcher.StatusBetter},
	}}}
	missing := scan.DeviceResult{}

	if got := deviceRow(actionable, false, false)[0]; got != "[ ]" {
		t.Errorf("unselected actionable row Sel cell = %q, want %q", got, "[ ]")
	}
	if got := deviceRow(actionable, true, false)[0]; got != "[x]" {
		t.Errorf("selected actionable row Sel cell = %q, want %q", got, "[x]")
	}
	if got := deviceRow(missing, true, false)[0]; got != "   " {
		t.Errorf("MISSING row Sel cell = %q, want blank (nothing to install)", got)
	}
}
