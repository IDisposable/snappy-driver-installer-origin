package main

import (
	"testing"

	"sdio/internal/collection"
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

// TestBuildRowsHonorsFilters confirms the table only shows what
// Visible() allows, matching the "new and better match" default the
// user asked for while remaining fully driven by settings.Filters.
func TestBuildRowsHonorsFilters(t *testing.T) {
	drp := &indexing.Driverpack{Filename: "DP_Test_SDIO01_1.bin"}
	better := scan.DeviceResult{Candidates: []collection.Candidate{{
		Driverpack: drp,
		Result:     matcher.Result{AltSectScore: 2, DecorScore: 1, Status: matcher.StatusBetter},
	}}}
	current := scan.DeviceResult{Candidates: []collection.Candidate{{
		Driverpack: drp,
		Result:     matcher.Result{AltSectScore: 2, DecorScore: 1, Status: matcher.StatusCurrent},
	}}}
	devices := []scan.DeviceResult{better, current}

	rows := buildRows(devices, settings.DefaultFilters)
	if len(rows) != 1 {
		t.Fatalf("buildRows(DefaultFilters) = %d rows, want 1 (only the StatusBetter device)", len(rows))
	}

	rows = buildRows(devices, settings.DefaultFilters|settings.FilterCurrent)
	if len(rows) != 2 {
		t.Fatalf("buildRows(DefaultFilters|FilterCurrent) = %d rows, want 2", len(rows))
	}
}
