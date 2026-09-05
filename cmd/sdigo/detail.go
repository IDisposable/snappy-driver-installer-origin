package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"sdio/internal/collection"
	"sdio/internal/common"
	"sdio/internal/hardware"
	"sdio/internal/matcher"
	"sdio/internal/scan"
)

// updateDetail handles key input on the per-device detail screen.
// Only the keys the footer documents do anything - an unrecognized
// key is ignored rather than closing the screen, so a stray keypress
// can't dismiss it by accident. Scroll keys (arrows/pgup/pgdn/home/
// end) are forwarded to detailViewport, since content routinely runs
// longer than the terminal.
func (m model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	dr := m.currentDevice()
	setSelected := func(v bool) {
		if dr == nil || dr.Best() == nil {
			return
		}
		if v {
			m.selected[dr.Device.InstanceID] = true
		} else {
			delete(m.selected, dr.Device.InstanceID)
		}
		m.table.SetRows(tableRows(m.rows, m.selected, m.showInstalledCol, m.bestMatchWidth, m.versionWidth))
		m.detailViewport.SetContent(m.detailView(*dr))
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case " ":
		if dr != nil && dr.Best() != nil {
			setSelected(!m.selected[dr.Device.InstanceID])
		}
		return m, nil
	case "y":
		setSelected(true)
		m.screen = screenTable
		return m, nil
	case "n":
		setSelected(false)
		m.screen = screenTable
		return m, nil
	case "q", "esc":
		m.screen = screenTable
		return m, nil
	case "up", "down", "k", "j", "pgup", "pgdown", "home", "end", "ctrl+u", "ctrl+d":
		var cmd tea.Cmd
		m.detailViewport, cmd = m.detailViewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

// comparison holds the per-field installed-vs-candidate outcome,
// ported from Manager::draw_hint's cm_date/cm_ver/cm_hwid/cm_score
// locals - answering "whose value for this ONE field is better",
// which is a different, finer-grained question than the overall
// BETTER/WORSE/SAME verdict DeviceResult.Best already resolves: an
// installed inbox driver can carry a numerically higher four-part
// version number (e.g. 10.0.26100.9223 vs. a candidate's
// 10.0.22000.10003) while the candidate still wins overall on date
// and score. 0 means tie or "not comparable" (e.g. no installed
// driver at all); 1 means installed wins that field; 2 means the
// candidate does.
type comparison struct {
	date, version, hwid, score int
}

func compareInstalledVsCandidate(dr scan.DeviceResult, best *collection.Candidate) comparison {
	var c comparison
	if dr.Installed == nil || best == nil {
		return c
	}
	switch r := common.CompareDate(dr.Installed.Version, best.Result.DriverVersion); {
	case r > 0:
		c.date = 1
	case r < 0:
		c.date = 2
	}
	switch r := common.CompareVersion(dr.Installed.Version, best.Result.DriverVersion); {
	case r > 0:
		c.version = 1
	case r < 0:
		c.version = 2
	}
	if dr.InstalledScore != nil {
		// A lower raw score ranks better - see matcher.Result.Cmp's
		// negated CmpUnsigned comparison.
		switch r := matcher.CmpUnsigned(dr.InstalledScore.Score, best.Result.Score); {
		case r < 0:
			c.score = 1
		case r > 0:
			c.score = 2
		}
	}
	c.hwid = hwidComparison(dr.Device.HardwareIDs, dr.Device.CompatibleIDs, dr.Installed.MatchingDeviceID, best.Result.HWID)
	return c
}

// hwidComparison finds whichever of installedID/candidateID matches
// an earlier (more specific) entry in the device's own combined
// hardware-then-compatible ID list, ported from the pp/cm_hwid
// bookkeeping in Manager::draw_hint. Returns 0 if neither matches, or
// both match the very same entry (an exact tie, not "id" - genuinely
// no fresh update: the driver pack targets the identical ID the
// installed driver already used).
func hwidComparison(hardwareIDs, compatibleIDs []string, installedID, candidateID string) int {
	for _, p := range hardwareIDs {
		if pp := hwidMatchBits(p, installedID, candidateID); pp == 1 || pp == 2 {
			return pp
		}
	}
	for _, p := range compatibleIDs {
		if pp := hwidMatchBits(p, installedID, candidateID); pp == 1 || pp == 2 {
			return pp
		}
	}
	return 0
}

func hwidMatchBits(entry, installedID, candidateID string) int {
	pp := 0
	if installedID != "" && strings.EqualFold(installedID, entry) {
		pp |= 1
	}
	if candidateID != "" && strings.EqualFold(candidateID, entry) {
		pp |= 2
	}
	return pp
}

// styleIf applies betterStyle when winner matches side (1=installed,
// 2=candidate), otherwise renders value unstyled.
func styleIf(value string, winner, side int) string {
	if winner == side {
		return betterStyle.Render(value)
	}
	return value
}

// styleIfMatched highlights an ID-list entry that matched either
// side's matching ID (pp!=0, from hwidMatchBits) - ported from
// Manager::draw_hint's per-entry `pp?POPUP_HWID_COLOR:c0`, which
// marks "this is one of the relevant IDs" rather than comparing which
// side wins (that comparative judgment is the "Matched ID" summary
// line below, via styleIf/cmp.hwid).
func styleIfMatched(value string, pp int) string {
	if pp != 0 {
		return betterStyle.Render(value)
	}
	return value
}

// signatureLabel describes a candidate's catalog-validity in the same
// terms Manager::filter's altsectscore-based visibility rule uses:
// 2 is catalog-signed and confirmed valid for the running OS, 1 is
// present but unsigned or unconfirmed, 0 never reaches here (Best()
// requires IsDriverValid, i.e. AltSectScore>0). Styled red when not
// fully valid, mirroring Manager::draw_hint's isvalidcat check -
// there's no equivalent styling for the installed side below since
// this rewrite never ported Driver::isvalidcat's own catalog lookup.
func signatureLabel(altSectScore int) string {
	switch altSectScore {
	case 2:
		return "catalog-signed, valid for this OS"
	case 1:
		return invalidStyle.Render("unsigned or unconfirmed")
	default:
		return invalidStyle.Render("invalid")
	}
}

// verdictSummary states in one sentence why a candidate is (or isn't)
// recommended, ported from the STR_STATUS_BETTER_NEW/_CUR/_OLD
// sentences itembar_t::str_status builds - the Status column only has
// room for a short word (scan.MatchLabel), which can otherwise read
// as a plain negative ("Older") for a driver that's still the
// recommended pick overall.
func verdictSummary(best *collection.Candidate) string {
	if best == nil {
		return "Not recommended: no candidate outranks the installed driver."
	}
	switch {
	case best.Result.Status&matcher.StatusNew != 0:
		return "Recommended: outranks the installed driver, and is dated more recently."
	case best.Result.Status&matcher.StatusOld != 0:
		return "Recommended: outranks the installed driver overall, even though it's dated older - see Score below for why."
	case best.Result.Status&matcher.StatusCurrent != 0:
		return "Recommended: outranks the installed driver (same release date)."
	default:
		return "Recommended: no driver is currently installed for this device."
	}
}

// scoreDifferences enumerates, in the same priority order
// matcher.Score itself combines them (catalog signature highest,
// then feature number, then hardware-ID match precision), which
// specific factors differ between the installed driver and this
// candidate and which side wins each one - the concrete "why" behind
// the two opaque hex Score values, instead of a description of what
// Score is in the abstract. A tied factor is omitted. Returns nil if
// there's nothing to compare (no installed driver, or its own score
// couldn't be computed).
func scoreDifferences(dr scan.DeviceResult, best *collection.Candidate, is64Bit bool) []string {
	if dr.InstalledScore == nil || best == nil {
		return nil
	}
	inst := dr.InstalledScore
	drp := best.Driverpack

	var lines []string
	side := func(installedWins bool) string {
		if installedWins {
			return "installed"
		}
		return "candidate"
	}

	instSig := matcher.SignatureScore(inst.CatalogFileBits, is64Bit, inst.IsNTSection)
	candIsNTSection := strings.Contains(strings.ToLower(drp.InstallPicked(best.HWIDIndex)), ".nt")
	candSig := matcher.SignatureScore(drp.CatalogFileBits(best.HWIDIndex), is64Bit, candIsNTSection)
	if instSig != candSig {
		lines = append(lines, fmt.Sprintf("Catalog signature: %s is properly signed for this system, the other isn't",
			side(instSig < candSig)))
	}

	candFeature := drp.Feature(best.HWIDIndex)
	if inst.Feature != candFeature {
		lines = append(lines, fmt.Sprintf("Driver pack's priority hint: installed=%d, candidate=%d, 255=default (%s wins - lower is preferred)",
			inst.Feature, candFeature, side(inst.Feature < candFeature)))
	}

	cmp := compareInstalledVsCandidate(dr, best)
	if cmp.hwid != 0 {
		lines = append(lines, fmt.Sprintf("Hardware ID match: %s matched a more specific ID", side(cmp.hwid == 1)))
	}
	if cmp.date != 0 {
		lines = append(lines, fmt.Sprintf("Release date: %s is dated more recently (a separate factor from overall rank)", side(cmp.date == 1)))
	}
	if cmp.score != 0 {
		lines = append(lines, fmt.Sprintf("Overall rank: %s wins (%08X vs %08X, lower wins)", side(cmp.score == 1), inst.Score, best.Result.Score))
	}

	return lines
}

// detailHelpLine is rendered as a fixed header above the scrollable
// detail viewport, rather than as part of its content, so it stays
// visible regardless of scroll position.
const detailHelpLine = "Device detail - space: toggle install, y: mark and back, n: unmark and back, q/esc: back, ↑↓: scroll\n"

// detailView renders the full comparison the original's hover
// tooltip shows (installed vs. available driver), for the device
// under the table's cursor.
func (m model) detailView(dr scan.DeviceResult) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Device\n")
	fmt.Fprintf(&b, "  Description    %s\n", dr.Device.Description)
	fmt.Fprintf(&b, "  Manufacturer   %s\n", dr.Device.Manufacturer)
	fmt.Fprintf(&b, "  Instance ID    %s\n", dr.Device.InstanceID)
	fmt.Fprintf(&b, "  Status         %s\n", dr.Device.Status())
	tick := "not selected for install"
	if m.selected[dr.Device.InstanceID] {
		tick = "SELECTED for install"
	}
	fmt.Fprintf(&b, "  Install        %s\n\n", tick)

	best := dr.Best()
	fmt.Fprintf(&b, "%s\n\n", verdictSummary(best))

	cmp := compareInstalledVsCandidate(dr, best)
	installedID, candidateID := "", ""
	if dr.Installed != nil {
		installedID = dr.Installed.MatchingDeviceID
	}
	if best != nil {
		candidateID = best.Result.HWID
	}

	if len(dr.Device.HardwareIDs) > 0 {
		b.WriteString("Installed hardware ID\n")
		for _, id := range dr.Device.HardwareIDs {
			fmt.Fprintf(&b, "  %s\n", styleIfMatched(id, hwidMatchBits(id, installedID, candidateID)))
		}
		b.WriteString("\n")
	}
	if len(dr.Device.CompatibleIDs) > 0 {
		b.WriteString("Installed compatible ID\n")
		for _, id := range dr.Device.CompatibleIDs {
			fmt.Fprintf(&b, "  %s\n", styleIfMatched(id, hwidMatchBits(id, installedID, candidateID)))
		}
		b.WriteString("\n")
	}

	b.WriteString("Installed driver\n")
	if dr.Installed == nil {
		b.WriteString("  (none)\n\n")
	} else {
		inst := dr.Installed
		fmt.Fprintf(&b, "  Provider       %s\n", inst.ProviderName)
		fmt.Fprintf(&b, "  Date           %s\n", styleIf(inst.Version.DateString(), cmp.date, 1))
		fmt.Fprintf(&b, "  Version        %s\n", styleIf(inst.Version.String(), cmp.version, 1))
		fmt.Fprintf(&b, "  Matched ID     %s\n", styleIf(inst.MatchingDeviceID, cmp.hwid, 1))
		fmt.Fprintf(&b, "  Inf file       %s\n", inst.InfPath)
		fmt.Fprintf(&b, "  Section        %s%s\n", inst.InfSection, inst.InfSectionExt)
		if dr.InstalledScore != nil {
			fmt.Fprintf(&b, "  Score          %s\n", styleIf(fmt.Sprintf("%08X", dr.InstalledScore.Score), cmp.score, 1))
		}
		if hardware.IsMicrosoftDriver(inst) {
			fmt.Fprintf(&b, "  %s\n", cautionStyle.Render("Microsoft-provided driver - replacing it is often unnecessary and can be riskier than keeping it"))
		}
		b.WriteString("\n")
	}

	b.WriteString("Available driver (best match)\n")
	if best == nil {
		b.WriteString("  (no actionable candidate)\n")
	} else {
		drp := best.Driverpack
		fmt.Fprintf(&b, "  Driver pack    %s\n", drp.Filename)
		fmt.Fprintf(&b, "  Provider       %s\n", drp.Manufacturer(best.HWIDIndex))
		fmt.Fprintf(&b, "  Date           %s\n", styleIf(best.Result.DriverVersion.DateString(), cmp.date, 2))
		fmt.Fprintf(&b, "  Version        %s\n", styleIf(best.Result.DriverVersion.String(), cmp.version, 2))
		fmt.Fprintf(&b, "  Matched ID     %s\n", styleIf(best.Result.HWID, cmp.hwid, 2))
		fmt.Fprintf(&b, "  Inf file       %s\n", drp.InfPath(best.HWIDIndex))
		section := best.Result.Section
		if best.Result.DecorScore == 0 {
			section = invalidStyle.Render(section)
		}
		fmt.Fprintf(&b, "  Section        %s\n", section)
		fmt.Fprintf(&b, "  Score          %s\n", styleIf(fmt.Sprintf("%08X", best.Result.Score), cmp.score, 2))
		fmt.Fprintf(&b, "  Signature      %s\n", signatureLabel(best.Result.AltSectScore))
		if drp.Pending {
			b.WriteString("  (driver pack data not yet downloaded - needs the configured torrent)\n")
		}

		if diffs := scoreDifferences(dr, best, m.result.System.SysInfo.Is64Bit); len(diffs) > 0 {
			b.WriteString("\nWhy the candidate outranks (or doesn't) the installed driver,\nin order of how much each factor counts toward Score:\n")
			for i, line := range diffs {
				fmt.Fprintf(&b, "  %d. %s\n", i+1, line)
			}
		}
	}
	return b.String()
}
