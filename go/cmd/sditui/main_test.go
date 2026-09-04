package main

import (
	"testing"

	"github.com/charmbracelet/bubbles/table"

	"sdio/internal/collection"
	"sdio/internal/common"
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

// TestLayoutColumnsAddsInstalledColumnWhenWide confirms the sixth
// "Installed" column only appears once the terminal is wide enough to
// fit it alongside the fixed-width columns, and that column widths
// never go negative/zero on a narrow terminal.
func TestLayoutColumnsAddsInstalledColumnWhenWide(t *testing.T) {
	narrow, showNarrow := layoutColumns(80, nil)
	if showNarrow {
		t.Error("layoutColumns(80, nil): showInstalled = true, want false")
	}
	if len(narrow) != 5 {
		t.Fatalf("layoutColumns(80, nil) = %d columns, want 5", len(narrow))
	}
	for _, c := range narrow {
		if c.Width <= 0 {
			t.Errorf("column %q has non-positive width %d", c.Title, c.Width)
		}
	}

	wide, showWide := layoutColumns(200, nil)
	if !showWide {
		t.Error("layoutColumns(200, nil): showInstalled = false, want true")
	}
	if len(wide) != 6 {
		t.Fatalf("layoutColumns(200, nil) = %d columns, want 6", len(wide))
	}
}

// TestLayoutColumnsKeepsDeviceAndBestMatchFixed confirms Device and
// Best match don't grow with the terminal - a real device description
// or driver-pack filename is never longer than a fixed, modest bound,
// so stretching these columns on a wide terminal only wastes space on
// trailing blank padding (the user's exact complaint about the first
// resize-aware layout).
func TestLayoutColumnsKeepsDeviceAndBestMatchFixed(t *testing.T) {
	narrow, _ := layoutColumns(90, nil)
	wide, _ := layoutColumns(250, nil)
	for _, title := range []string{"Device", "Best match"} {
		n := columnWidth(t, narrow, title)
		w := columnWidth(t, wide, title)
		if n != w {
			t.Errorf("%s width changed with terminal width: %d (90 cols) vs %d (250 cols), want fixed", title, n, w)
		}
	}
}

// TestLayoutColumnsSizesVersionColumnsToContent confirms Version/
// Installed are sized to fit the actual longest value present rather
// than a guessed constant - too narrow clips (the original complaint
// about "1.8.0.1"-sized columns), too wide wastes space.
func TestLayoutColumnsSizesVersionColumnsToContent(t *testing.T) {
	candidateVersion := common.Version{V1: 10, V2: 0, V3: 22000, V4: 10003}
	installedVersion := common.Version{V1: 10, V2: 0, V3: 26100, V4: 4405}
	drp := &indexing.Driverpack{Filename: "DP_Test_SDIO01_1.7z"}
	devices := []scan.DeviceResult{{
		Device: hardware.Device{InstanceID: "d1"},
		Candidates: []collection.Candidate{{
			Driverpack: drp,
			Result:     matcher.Result{AltSectScore: 2, DecorScore: 1, Status: matcher.StatusBetter, DriverVersion: candidateVersion},
		}},
		Installed: &hardware.InstalledDriver{Version: installedVersion},
	}}

	cols, showInstalled := layoutColumns(1000, devices)
	if !showInstalled {
		t.Fatal("layoutColumns(1000, ...): showInstalled = false, want true")
	}
	if got, want := columnWidth(t, cols, "Version"), len(candidateVersion.String()); got != want {
		t.Errorf("Version width = %d, want %d (len(%q))", got, want, candidateVersion.String())
	}
	if got, want := columnWidth(t, cols, "Installed"), len(installedVersion.String()); got != want {
		t.Errorf("Installed width = %d, want %d (len(%q))", got, want, installedVersion.String())
	}
}

func columnWidth(t *testing.T, cols []table.Column, title string) int {
	t.Helper()
	for _, c := range cols {
		if c.Title == title {
			return c.Width
		}
	}
	t.Fatalf("no column titled %q", title)
	return 0
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
