package scan

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"sdio/internal/hardware"
	"sdio/internal/logging"
	"sdio/internal/sdwfile"
	"sdio/internal/settings"
)

// snapshotVersion tags the payload format written to logs/*.snp - a
// fresh Go-native JSON encoding, not the original's raw state_m_t/
// Device/Driver C struct dump. Deliberately distinct from the
// original's VER_STATE (0x102, main.h): the SDW container itself is
// the only part of the snapshot format required to match
// (docs/COMPATIBILITY.md), so nothing should ever try to read this
// version number as if it were the original's.
const snapshotVersion int32 = 1

// snapshotPayload is the JSON document a snapshot's SDW container
// wraps - enough of what Prepare detected to be a useful record of
// "what did this machine look like at scan time", without
// replicating the original's internal Driver/Device C structs.
type snapshotPayload struct {
	System  System
	Devices []hardware.Device
}

// writeSnapshot saves a timestamped state snapshot to
// s.LogDir/<timestamp>state.snp, mirroring model.cpp's
// Bundle::bundle_lowpriority - runs once per real hardware scan (see
// Prepare), not on every rematch. Skipped entirely under
// FlagNoSnapshot, matching State::save's own early return. A failure
// here is logged but never fails the scan: a snapshot is a diagnostic
// convenience, not something anything else in this rewrite depends on.
func writeSnapshot(s *settings.Settings, p Prepared, logger *logging.Logger) {
	if s.Flags&settings.FlagNoSnapshot != 0 {
		return
	}
	payload, err := json.Marshal(snapshotPayload{System: p.System, Devices: p.devices})
	if err != nil {
		logger.Error().Err(err).Msg("encoding state snapshot failed")
		return
	}
	if err := os.MkdirAll(s.LogDir, 0o755); err != nil {
		logger.Error().Err(err).Str("dir", s.LogDir).Msg("creating log directory for state snapshot failed")
		return
	}
	name := filepath.Join(s.LogDir, logging.Timestamp()+"state.snp")
	f, err := os.Create(name)
	if err != nil {
		logger.Error().Err(err).Str("file", name).Msg("creating state snapshot failed")
		return
	}
	if err := sdwfile.Encode(f, snapshotVersion, payload, true); err != nil {
		logger.Error().Err(err).Str("file", name).Msg("writing state snapshot failed")
		f.Close()
		os.Remove(name) // don't leave a truncated .snp behind for a later -ls: to choke on
		return
	}
	f.Close()
	logger.Info().Str("file", name).Msg("saved state snapshot")
}

// loadSnapshot reads back a snapshot writeSnapshot wrote, for
// StateModeEmul (-ls:<file>) replay - the Go equivalent of
// State::load, decoding the JSON payload described on snapshotPayload
// rather than the original's raw struct dump.
func loadSnapshot(path string) (System, []hardware.Device, error) {
	f, err := os.Open(path)
	if err != nil {
		return System{}, nil, err
	}
	defer f.Close()

	version, payload, err := sdwfile.Decode(f, true)
	if err != nil {
		return System{}, nil, fmt.Errorf("decoding SDW container: %w", err)
	}
	if version != snapshotVersion {
		return System{}, nil, fmt.Errorf("unsupported snapshot version %d (want %d)", version, snapshotVersion)
	}

	var snap snapshotPayload
	if err := json.Unmarshal(payload, &snap); err != nil {
		return System{}, nil, fmt.Errorf("decoding snapshot payload: %w", err)
	}
	return snap.System, snap.Devices, nil
}
