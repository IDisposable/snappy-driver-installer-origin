package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"

	"sdio/internal/collection"
	"sdio/internal/common"
	"sdio/internal/hardware"
	"sdio/internal/indexing"
	"sdio/internal/matcher"
	"sdio/internal/scan"
	"sdio/internal/sdwfile"
	"sdio/internal/settings"
	"sdio/internal/update"
	"sdio/internal/usbdrive"
)

// realDtPortCandidate builds a real candidate.Candidate for the
// reference installation's dtport pack, the same real fixture
// internal/installflow's tests use, for exercising scoreDifferences
// against actual driver-pack data (Feature/CatalogFileBits/
// InstallPicked) rather than a hand-built stand-in.
func realDtPortCandidate(t *testing.T) collection.Candidate {
	t.Helper()
	const indexPath = "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/indexes/SDI/DP_Ports_SDIO01_26083.bin"
	const packDir = "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/drivers"
	const packFilename = "DP_Ports_SDIO01_26083.7z"

	f, err := os.Open(indexPath)
	if err != nil {
		t.Skipf("real index file not available at %s: %v", indexPath, err)
	}
	defer f.Close()
	_, payload, err := sdwfile.Decode(f, true)
	if err != nil {
		t.Fatalf("Decode() error: %v", err)
	}
	idx, err := indexing.DecodeIndex(payload)
	if err != nil {
		t.Fatalf("DecodeIndex() error: %v", err)
	}
	drp := &indexing.Driverpack{Path: packDir, Filename: packFilename, Index: idx}

	wantHWID := `DTBUS\COMPORT&VID_37DD&PID_6001`
	for i := range idx.HWIDs {
		if strings.EqualFold(drp.HWID(i), wantHWID) {
			return collection.Candidate{
				Driverpack: drp, HWIDIndex: i,
				Result: matcher.Result{HWID: wantHWID, DriverVersion: common.Version{V1: 1, V2: 0, V3: 0, V4: 6}},
			}
		}
	}
	t.Fatalf("HWID %q not found in %s", wantHWID, indexPath)
	return collection.Candidate{}
}

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
	item := optionItem{isFlag: true, flagBit: settings.FlagAutoClose}
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
// selected argument - the Sel column cmd/sdigo's space-bar toggle
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

func TestHwidComparisonPrefersEarlierMoreSpecificMatch(t *testing.T) {
	hw := []string{"USB\\VID_0BDA&PID_5634&MI_00", "USB\\VID_0BDA&PID_5634"}
	compat := []string{"USB\\Class_0e&SubClass_03", "USB\\Class_0e"}

	// candidate matches the most specific hardware ID; installed only
	// matches a later, less specific compatible ID.
	if got := hwidComparison(hw, compat, "USB\\CLASS_0E", "USB\\VID_0BDA&PID_5634&MI_00"); got != 2 {
		t.Errorf("hwidComparison() = %d, want 2 (candidate matched the more specific entry)", got)
	}
	// installed matches the most specific ID this time.
	if got := hwidComparison(hw, compat, "USB\\VID_0BDA&PID_5634&MI_00", "USB\\CLASS_0E"); got != 1 {
		t.Errorf("hwidComparison() = %d, want 1 (installed matched the more specific entry)", got)
	}
	// both match the very same entry: a tie, not a win for either side.
	if got := hwidComparison(hw, compat, "USB\\VID_0BDA&PID_5634&MI_00", "USB\\VID_0BDA&PID_5634&MI_00"); got != 0 {
		t.Errorf("hwidComparison() = %d, want 0 (exact tie)", got)
	}
	// neither ID appears in the device's own list at all.
	if got := hwidComparison(hw, compat, "USB\\SOMETHING_ELSE", "USB\\SOMETHING_ELSE_TOO"); got != 0 {
		t.Errorf("hwidComparison() = %d, want 0 (neither matches)", got)
	}
}

func TestCompareInstalledVsCandidate(t *testing.T) {
	newer := common.Version{Day: 13, Month: 1, Year: 2024, V1: 10, V2: 0, V3: 22000, V4: 10003}
	older := common.Version{Day: 21, Month: 6, Year: 2006, V1: 10, V2: 0, V3: 26100, V4: 9223}

	drp := &indexing.Driverpack{Filename: "DP_Test.7z"}
	best := &collection.Candidate{Driverpack: drp, Result: matcher.Result{DriverVersion: newer, Score: 2, HWID: "USB\\A"}}
	dr := scan.DeviceResult{
		Device:         hardware.Device{HardwareIDs: []string{"USB\\A", "USB\\B"}},
		Installed:      &hardware.InstalledDriver{Version: older, MatchingDeviceID: "USB\\B"},
		InstalledScore: &collection.InstalledScore{Score: 1},
	}

	cmp := compareInstalledVsCandidate(dr, best)
	if cmp.date != 2 {
		t.Errorf("date = %d, want 2 (candidate's date is newer)", cmp.date)
	}
	if cmp.version != 1 {
		t.Errorf("version = %d, want 1 (installed's raw version number is numerically higher)", cmp.version)
	}
	if cmp.score != 1 {
		t.Errorf("score = %d, want 1 (installed's lower raw score ranks better)", cmp.score)
	}
	if cmp.hwid != 2 {
		t.Errorf("hwid = %d, want 2 (candidate matched the earlier, more specific hardware ID)", cmp.hwid)
	}

	if got := compareInstalledVsCandidate(scan.DeviceResult{}, best); got != (comparison{}) {
		t.Errorf("compareInstalledVsCandidate() with no installed driver = %+v, want zero value", got)
	}
	if got := compareInstalledVsCandidate(dr, nil); got != (comparison{}) {
		t.Errorf("compareInstalledVsCandidate() with no candidate = %+v, want zero value", got)
	}
}

// TestVerdictSummaryAlwaysStatesRecommended confirms every case where
// the table would show a non-MISSING status label produces a sentence
// starting with "Recommended" - the table's short label alone (esp.
// OLDER) can otherwise read as a plain negative for a driver that is,
// in fact, still the recommended pick.
func TestVerdictSummaryAlwaysStatesRecommended(t *testing.T) {
	if got := verdictSummary(nil); strings.HasPrefix(got, "Recommended") {
		t.Errorf("verdictSummary(nil) = %q, should not claim a recommendation exists", got)
	}
	for _, status := range []int{
		matcher.StatusBetter | matcher.StatusNew,
		matcher.StatusBetter | matcher.StatusOld,
		matcher.StatusBetter | matcher.StatusCurrent,
		matcher.StatusBetter,
	} {
		best := &collection.Candidate{Result: matcher.Result{Status: status}}
		if got := verdictSummary(best); !strings.HasPrefix(got, "Recommended") {
			t.Errorf("verdictSummary(status=%#x) = %q, want it to start with \"Recommended\"", status, got)
		}
	}
}

// TestScoreDifferencesEnumeratesEachFactor uses the real dtport
// driver pack (Feature=255, CatalogFileBits=8/signed, InstallPicked
// "dtport.nt") against a deliberately worse-in-every-respect
// installed driver, confirming every factor scoreDifferences knows
// about is reported with the candidate as the winner.
func TestScoreDifferencesEnumeratesEachFactor(t *testing.T) {
	best := realDtPortCandidate(t)
	best.Result.DriverVersion = common.Version{Year: 2024, Month: 1, Day: 13, V1: 1, V2: 0, V3: 0, V4: 6}
	best.Result.Score = 10

	dr := scan.DeviceResult{
		Device: hardware.Device{HardwareIDs: []string{best.Result.HWID}},
		Installed: &hardware.InstalledDriver{
			MatchingDeviceID: `NOT\IN\THE\LIST`,
			Version:          common.Version{Year: 2020, Month: 1, Day: 1, V1: 0, V2: 9, V3: 0, V4: 0},
		},
		InstalledScore: &collection.InstalledScore{
			Score: 20, CatalogFileBits: 0, Feature: 100, IsNTSection: true,
		},
	}

	diffs := scoreDifferences(dr, &best, true)
	joined := strings.Join(diffs, "\n")
	for _, want := range []string{
		"Catalog signature: candidate is properly signed",
		"Driver pack's priority hint: installed=100, candidate=255, 255=default (installed wins",
		"Hardware ID match: candidate matched a more specific ID",
		"Release date: candidate is dated more recently",
		"Overall rank: candidate wins (00000014 vs 0000000A",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("scoreDifferences() missing %q; got:\n%s", want, joined)
		}
	}
}

// TestScoreDifferencesOmitsTiedFactors confirms a device with no
// installed driver at all (nothing to compare against) returns no
// lines, rather than a list of false differences.
func TestScoreDifferencesOmitsTiedFactors(t *testing.T) {
	best := realDtPortCandidate(t)
	dr := scan.DeviceResult{Device: hardware.Device{HardwareIDs: []string{best.Result.HWID}}}

	if diffs := scoreDifferences(dr, &best, true); diffs != nil {
		t.Errorf("scoreDifferences() with no installed driver = %v, want nil", diffs)
	}
}

func TestIsMicrosoftDriver(t *testing.T) {
	cases := []struct {
		inst *hardware.InstalledDriver
		want bool
	}{
		{nil, false},
		{&hardware.InstalledDriver{ProviderName: "Microsoft"}, true},
		{&hardware.InstalledDriver{ProviderName: "  microsoft  "}, true},
		{&hardware.InstalledDriver{ProviderName: "Realtek"}, false},
		{&hardware.InstalledDriver{ProviderName: ""}, false},
	}
	for _, c := range cases {
		if got := isMicrosoftDriver(c.inst); got != c.want {
			t.Errorf("isMicrosoftDriver(%+v) = %v, want %v", c.inst, got, c.want)
		}
	}
}

// TestDeviceRowFlagsMicrosoftDriver confirms the [MS] flag is
// prepended (not appended) to the device description, so a long
// description's ellipsis truncation can't hide it, and that it only
// appears for actionable (Best()!=nil) rows.
func TestDeviceRowFlagsMicrosoftDriver(t *testing.T) {
	drp := &indexing.Driverpack{Filename: "DP_Test_SDIO01_1.7z"}
	msInstalled := scan.DeviceResult{
		Device:    hardware.Device{Description: "Widget"},
		Installed: &hardware.InstalledDriver{ProviderName: "Microsoft"},
		Candidates: []collection.Candidate{{
			Driverpack: drp,
			Result:     matcher.Result{AltSectScore: 2, DecorScore: 1, Status: matcher.StatusBetter},
		}},
	}
	row := deviceRow(msInstalled, false, false)
	if got := row[2]; got != "[MS] Widget" {
		t.Errorf("device cell = %q, want %q", got, "[MS] Widget")
	}

	nonMS := msInstalled
	nonMS.Installed = &hardware.InstalledDriver{ProviderName: "Realtek"}
	if got := deviceRow(nonMS, false, false)[2]; got != "Widget" {
		t.Errorf("device cell = %q, want %q (no MS flag for a non-Microsoft installed driver)", got, "Widget")
	}
}

// TestResizeWhileDetailScreenOpenDoesNotPanic reproduces a real crash:
// bubbles/table.SetColumns re-renders immediately against whatever
// rows are already loaded, and if showInstalled flips (changing the
// column count) those rows are the wrong shape, indexing off the end
// of the new columns. Shrinking the window while the detail screen is
// open exercises exactly that path via refreshTable.
func TestResizeWhileDetailScreenOpenDoesNotPanic(t *testing.T) {
	best := realDtPortCandidate(t)
	best.Result.AltSectScore = 2
	best.Result.DecorScore = 1
	best.Result.Status = matcher.StatusBetter

	result := scan.Result{Devices: []scan.DeviceResult{{
		Device:     hardware.Device{Description: "Widget", InstanceID: "d1", HardwareIDs: []string{best.Result.HWID}},
		Candidates: []collection.Candidate{best},
	}}}
	s := settings.New()
	m := newModel(result, s, nil)

	mm, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	m = mm.(model)

	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(model)
	if m.screen != screenDetail {
		t.Fatalf("expected screenDetail, got %v", m.screen)
	}
	_ = m.View()

	for _, size := range []tea.WindowSizeMsg{
		{Width: 40, Height: 10},
		{Width: 10, Height: 3},
		{Width: 1, Height: 1},
		{Width: 200, Height: 50},
	} {
		mm, _ = m.Update(size)
		m = mm.(model)
		_ = m.View() // must not panic at any size
	}
}

// TestDetailViewportScrolls confirms the detail screen's content
// actually scrolls when it overflows a short terminal, rather than
// silently clipping with no way to see the rest.
func TestDetailViewportScrolls(t *testing.T) {
	best := realDtPortCandidate(t)
	best.Result.AltSectScore = 2
	best.Result.DecorScore = 1
	best.Result.Status = matcher.StatusBetter

	result := scan.Result{Devices: []scan.DeviceResult{{
		Device:         hardware.Device{Description: "Widget", InstanceID: "d1", HardwareIDs: []string{best.Result.HWID}},
		Candidates:     []collection.Candidate{best},
		Installed:      &hardware.InstalledDriver{ProviderName: "Realtek", MatchingDeviceID: best.Result.HWID},
		InstalledScore: &collection.InstalledScore{Score: 99, Feature: 1},
	}}}
	s := settings.New()
	m := newModel(result, s, nil)

	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 8}) // short enough that content overflows
	m = mm.(model)
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(model)

	if !m.detailViewport.AtTop() {
		t.Fatal("expected the viewport to open scrolled to the top")
	}
	if m.detailViewport.TotalLineCount() <= m.detailViewport.Height {
		t.Fatalf("test fixture bug: content (%d lines) must exceed the viewport height (%d) to test scrolling",
			m.detailViewport.TotalLineCount(), m.detailViewport.Height)
	}

	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = mm.(model)
	if m.detailViewport.YOffset == 0 {
		t.Error("YOffset = 0 after pressing down, want it to have scrolled")
	}
}

// TestAboutScreenNavigation confirms ? opens the About screen from
// the table and q/esc/? all return to it - the same "only documented
// keys act" contract as the other popups.
func TestAboutScreenNavigation(t *testing.T) {
	m := newModel(scan.Result{}, settings.New(), nil)

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = mm.(model)
	if m.screen != screenAbout {
		t.Fatalf("screen = %v after '?', want screenAbout", m.screen)
	}
	if !strings.Contains(m.View(), "Snappy Driver Installer: Go Forth") {
		t.Errorf("About view missing product name: %s", m.View())
	}

	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'x'}},
		{Type: tea.KeySpace},
		{Type: tea.KeyEnter},
	} {
		mm, _ = m.Update(key)
		m = mm.(model)
		if m.screen != screenAbout {
			t.Fatalf("unrecognized key %q closed the About screen", key)
		}
	}

	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(model)
	if m.screen != screenTable {
		t.Fatalf("screen = %v after esc, want screenTable", m.screen)
	}
}

// TestNewModelOpensWelcomeOnFirstRun confirms scan.Result.FirstRun
// controls whether the TUI opens straight to the Welcome screen,
// without needing an interactive keypress to get there.
func TestNewModelOpensWelcomeOnFirstRun(t *testing.T) {
	if m := newModel(scan.Result{FirstRun: true}, settings.New(), nil); m.screen != screenWelcome {
		t.Errorf("screen = %v with FirstRun=true, want screenWelcome", m.screen)
	}
	if m := newModel(scan.Result{FirstRun: false}, settings.New(), nil); m.screen != screenTable {
		t.Errorf("screen = %v with FirstRun=false, want screenTable", m.screen)
	}
}

// TestWelcomeRequiresTorrentFile confirms selecting a download option
// with no -torrent-file configured reports the problem instead of
// silently doing nothing or crashing trying to reach a torrent that
// was never configured.
func TestWelcomeRequiresTorrentFile(t *testing.T) {
	s := settings.New()
	m := newModel(scan.Result{}, s, nil)
	m.screen = screenWelcome

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(model)
	if m.screen != screenInstallLog {
		t.Fatalf("screen = %v, want screenInstallLog (the no-torrent-file message)", m.screen)
	}
	if len(m.opLog) == 0 || !strings.Contains(m.opLog[0], "torrent-file") {
		t.Errorf("opLog = %v, want a message mentioning -torrent-file", m.opLog)
	}
}

// TestWelcomeAllDriverPacksNeedsConfirmation confirms the "Download
// All Driver Packs" item goes through a confirm screen rather than
// starting a large download on a single keypress.
func TestWelcomeAllDriverPacksNeedsConfirmation(t *testing.T) {
	s := settings.New()
	s.TorrentFile = "dummy.torrent" // just needs to be non-empty for this check
	m := newModel(scan.Result{}, s, nil)
	m.screen = screenWelcome
	m.welcomeIndex = len(welcomeItems) - 1 // "Download All Driver Packs"

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(model)
	if m.screen != screenWelcomeConfirmAll {
		t.Fatalf("screen = %v, want screenWelcomeConfirmAll", m.screen)
	}

	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(model)
	if m.screen != screenWelcome {
		t.Fatalf("screen = %v after esc, want screenWelcome (cancel, not download)", m.screen)
	}
}

// TestUSBDriveKeyReportsUnsupportedPlatform exploits the same trick
// TestInstallOneNormalModeReachesInstallDriver (internal/installflow)
// uses: usbdrive.ListRemovable returns a real "unsupported platform"
// error on this non-Windows dev machine, so pressing "u" reaching
// that error (instead of silently doing nothing) is a real,
// mock-free proof the key is wired to the real function.
func TestUSBDriveKeyReportsUnsupportedPlatform(t *testing.T) {
	m := newModel(scan.Result{}, settings.New(), nil)

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	m = mm.(model)
	if m.screen != screenInstallLog {
		t.Fatalf("screen = %v, want screenInstallLog (the unsupported-platform message)", m.screen)
	}
	if len(m.opLog) == 0 || !strings.Contains(m.opLog[0], "removable drives") {
		t.Errorf("opLog = %v, want a message about listing removable drives", m.opLog)
	}
}

// TestUpdateUSBDriveNavigationAndConfirm confirms up/down move the
// cursor within bounds and enter opens the confirm screen without
// starting a copy, using synthetic drives (no real hardware needed).
func TestUpdateUSBDriveNavigationAndConfirm(t *testing.T) {
	m := newModel(scan.Result{}, settings.New(), nil)
	m.screen = screenUSBDrive
	m.usbDrives = []usbdrive.Drive{
		{Root: `E:\`, TotalBytes: 1000, FreeBytes: 900},
		{Root: `F:\`, TotalBytes: 2000, FreeBytes: 500},
	}

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = mm.(model)
	if m.usbDriveIndex != 1 {
		t.Fatalf("usbDriveIndex = %d after down, want 1", m.usbDriveIndex)
	}
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = mm.(model)
	if m.usbDriveIndex != 1 {
		t.Fatalf("usbDriveIndex = %d after a second down, want 1 (clamped at the end)", m.usbDriveIndex)
	}

	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(model)
	if m.screen != screenUSBDriveConfirm {
		t.Fatalf("screen = %v after enter, want screenUSBDriveConfirm", m.screen)
	}
	if !strings.Contains(m.usbDriveConfirmView(), `F:\`) {
		t.Errorf("confirm view doesn't mention the selected drive F:\\: %s", m.usbDriveConfirmView())
	}

	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(model)
	if m.screen != screenUSBDrive {
		t.Fatalf("screen = %v after esc, want screenUSBDrive (cancel, not copy)", m.screen)
	}
}

// TestUpdateConfirmInstallRelaunchesElevatedWhenNotElevated confirms
// confirming install without an elevated token hands off to sdiGo's
// relaunch instead of calling runInstallCmd directly.
// install.IsElevated() is always false on this non-Windows dev
// machine (see internal/install's !windows stub) - the same mock-free
// trick used elsewhere in this file to prove a path is genuinely
// reached rather than assumed.
func TestUpdateConfirmInstallRelaunchesElevatedWhenNotElevated(t *testing.T) {
	m := newModel(scan.Result{}, settings.New(), nil)
	m.screen = screenConfirmInstall
	m.selected = map[string]bool{"DEV1": true, "DEV2": false}

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = mm.(model)

	if len(m.relaunchInstanceIDs) != 1 || m.relaunchInstanceIDs[0] != "DEV1" {
		t.Fatalf("relaunchInstanceIDs = %v, want exactly [DEV1]", m.relaunchInstanceIDs)
	}
	if m.screen == screenInstalling {
		t.Error("screen advanced to screenInstalling, want it to stay put while sdiGo relaunches elevated")
	}
}

// TestNewModelResumeSelectedOpensConfirmInstall confirms a selection
// carried across the elevation relaunch restores straight to the
// confirm-install screen, so the "y" that triggered the relaunch
// isn't silently dropped.
func TestNewModelResumeSelectedOpensConfirmInstall(t *testing.T) {
	m := newModel(scan.Result{}, settings.New(), map[string]bool{"DEV1": true})
	if m.screen != screenConfirmInstall {
		t.Fatalf("screen = %v, want screenConfirmInstall when resumeSelected is non-empty", m.screen)
	}
	if !m.selected["DEV1"] {
		t.Errorf("selected = %v, want DEV1 restored", m.selected)
	}
}

// TestResumeFileRoundTrip confirms writeResumeFile/readResumeFile
// agree on the on-disk format used to carry a selection across the
// elevation relaunch.
func TestResumeFileRoundTrip(t *testing.T) {
	path := writeResumeFile([]string{"DEV1", "DEV2"})
	if path == "" {
		t.Fatal("writeResumeFile() returned an empty path")
	}
	defer os.Remove(path)

	got := readResumeFile(path)
	if !got["DEV1"] || !got["DEV2"] || len(got) != 2 {
		t.Errorf("readResumeFile() = %v, want exactly DEV1 and DEV2", got)
	}
}

// TestReadResumeFileMissingFileReturnsNil confirms a missing/bad
// -elevated-resume path degrades to "nothing to restore" instead of
// sdiGo failing outright over an internal-only flag.
func TestReadResumeFileMissingFileReturnsNil(t *testing.T) {
	if got := readResumeFile(filepath.Join(t.TempDir(), "does-not-exist.txt")); got != nil {
		t.Errorf("readResumeFile() = %v, want nil for a missing file", got)
	}
}

// TestActiveFileLinesShowsEachInProgressFile confirms a multi-file
// download reports its own per-file breakdown, not just the overall
// aggregate - the overall percent alone can sit unchanged for a long
// time while individual files actually finish and start.
func TestActiveFileLinesShowsEachInProgressFile(t *testing.T) {
	got := activeFileLines([]update.FileProgress{
		{Path: "a/DP_Done.7z", Completed: 100, Total: 100},
		{Path: "a/DP_Half.7z", Completed: 50, Total: 100},
		{Path: "a/DP_Started.7z", Completed: 10, Total: 100},
	})

	if !strings.Contains(got, "1/3 files complete") {
		t.Errorf("activeFileLines() = %q, want a 1/3 complete count", got)
	}
	if !strings.Contains(got, "DP_Half.7z") || !strings.Contains(got, "DP_Started.7z") {
		t.Errorf("activeFileLines() = %q, want both in-progress files listed", got)
	}
	if strings.Contains(got, "DP_Done.7z") {
		t.Errorf("activeFileLines() = %q, a completed file shouldn't be listed as active", got)
	}
}

// TestActiveFileLinesSingleFileIsEmpty confirms a single-file download
// (the install path, which selects one pack at a time) doesn't
// duplicate the overall percent as a redundant one-file breakdown.
func TestActiveFileLinesSingleFileIsEmpty(t *testing.T) {
	got := activeFileLines([]update.FileProgress{{Path: "a/DP_Only.7z", Completed: 10, Total: 100}})
	if got != "" {
		t.Errorf("activeFileLines() = %q, want empty for a single file", got)
	}
}

// TestActiveFileLinesCapsShownFiles confirms a large batch (e.g.
// "Download All Driver Packs", 100+ files) summarizes the overflow
// instead of listing every in-progress file.
func TestActiveFileLinesCapsShownFiles(t *testing.T) {
	files := make([]update.FileProgress, maxActiveDownloadLines+3)
	for i := range files {
		files[i] = update.FileProgress{Path: fmt.Sprintf("a/DP_%d.7z", i), Completed: 1, Total: 100}
	}

	got := activeFileLines(files)
	if !strings.Contains(got, "3 more in progress") {
		t.Errorf("activeFileLines() = %q, want the overflow summarized as 3 more", got)
	}
}

// TestDownloadStatusViewShowsPerFileBreakdown confirms the Installing/
// Downloading screen renders activeFileLines' output alongside the
// overall percent, not just the overall percent alone.
func TestDownloadStatusViewShowsPerFileBreakdown(t *testing.T) {
	m := newModel(scan.Result{}, settings.New(), nil)
	m.dlProgress = &progressTracker{}
	m.dlProgress.report(update.Progress{
		Completed: 150, Total: 200,
		Files: []update.FileProgress{
			{Path: "a/DP_Done.7z", Completed: 100, Total: 100},
			{Path: "a/DP_Half.7z", Completed: 50, Total: 100},
		},
	})

	got := m.downloadStatusView("Downloading")
	if !strings.Contains(got, "75%") {
		t.Errorf("downloadStatusView() = %q, want the overall 75%% shown", got)
	}
	if !strings.Contains(got, "1/2 files complete") || !strings.Contains(got, "DP_Half.7z") {
		t.Errorf("downloadStatusView() = %q, want the per-file breakdown included", got)
	}
}
