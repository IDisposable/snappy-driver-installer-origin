package scan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/rs/zerolog"

	"sdio/internal/hardware"
	"sdio/internal/logging"
	"sdio/internal/sdwfile"
	"sdio/internal/settings"
)

// testLogger is a discard-everything Logger for tests that need to
// pass one but aren't exercising logging itself.
var testLogger = logging.New(zerolog.Disabled, nil)

// listSnapFiles returns the *.snp basenames in dir, for asserting
// writeSnapshot did (or didn't) create one without depending on its
// exact timestamped name.
func listSnapFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("ReadDir(%s) error: %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".snp" {
			names = append(names, e.Name())
		}
	}
	return names
}

// TestWriteSnapshotCreatesFileInLogDir confirms a real .snp file lands
// in s.LogDir, named "<timestamp>state.snp" matching the original's
// naming convention (see logging.Timestamp's doc comment), and that
// its SDW container decodes back to the same System/Devices it was
// given - the actual bug report this guards against was "no .snp files
// ever appear", not a round-trip subtlety, but round-tripping is the
// only way to confirm the file isn't empty/corrupt.
func TestWriteSnapshotCreatesFileInLogDir(t *testing.T) {
	s := settings.New()
	s.LogDir = t.TempDir()

	p := Prepared{
		System:  System{IsLaptop: true},
		devices: []hardware.Device{{InstanceID: "dev1", Description: "Test Device"}},
	}
	writeSnapshot(s, p, testLogger)

	names := listSnapFiles(t, s.LogDir)
	if len(names) != 1 {
		t.Fatalf("found %d .snp file(s) in %s, want 1: %v", len(names), s.LogDir, names)
	}

	f, err := os.Open(filepath.Join(s.LogDir, names[0]))
	if err != nil {
		t.Fatalf("Open(%s) error: %v", names[0], err)
	}
	defer f.Close()

	version, payload, err := sdwfile.Decode(f, true)
	if err != nil {
		t.Fatalf("Decode() error: %v", err)
	}
	if version != snapshotVersion {
		t.Errorf("version = %d, want %d", version, snapshotVersion)
	}

	var got snapshotPayload
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if !got.System.IsLaptop {
		t.Error("System.IsLaptop = false, want true (round-tripped from Prepared.System)")
	}
	if len(got.Devices) != 1 || got.Devices[0].InstanceID != "dev1" {
		t.Errorf("Devices = %+v, want one device with InstanceID \"dev1\"", got.Devices)
	}
}

// TestWriteSnapshotSkippedWhenFlagNoSnapshotSet confirms -nosnapshot
// actually suppresses file creation, matching State::save's own early
// return on FLAG_NOSNAPSHOT.
func TestWriteSnapshotSkippedWhenFlagNoSnapshotSet(t *testing.T) {
	s := settings.New()
	s.LogDir = t.TempDir()
	s.Flags |= settings.FlagNoSnapshot

	writeSnapshot(s, Prepared{}, testLogger)

	if names := listSnapFiles(t, s.LogDir); len(names) != 0 {
		t.Errorf("found .snp file(s) %v with FlagNoSnapshot set, want none", names)
	}
}

// TestLoadSnapshotRoundTrips confirms loadSnapshot reads back exactly
// what writeSnapshot wrote - the -ls:<file> replay path's only job.
func TestLoadSnapshotRoundTrips(t *testing.T) {
	s := settings.New()
	s.LogDir = t.TempDir()
	want := Prepared{
		System:  System{IsLaptop: true, BaseBoard: hardware.BaseBoard{Manufacturer: "Acme"}},
		devices: []hardware.Device{{InstanceID: "dev1", Description: "Test Device"}},
	}
	writeSnapshot(s, want, testLogger)
	path := filepath.Join(s.LogDir, listSnapFiles(t, s.LogDir)[0])

	system, devices, err := loadSnapshot(path)
	if err != nil {
		t.Fatalf("loadSnapshot() error: %v", err)
	}
	if !reflect.DeepEqual(system, want.System) {
		t.Errorf("System = %+v, want %+v", system, want.System)
	}
	if len(devices) != 1 || devices[0].InstanceID != "dev1" {
		t.Errorf("Devices = %+v, want one device with InstanceID \"dev1\"", devices)
	}
}

// TestLoadSnapshotMissingFileReturnsError confirms a bad -ls: path
// fails loudly instead of silently falling back to a real scan.
func TestLoadSnapshotMissingFileReturnsError(t *testing.T) {
	if _, _, err := loadSnapshot(filepath.Join(t.TempDir(), "missing.snp")); err == nil {
		t.Error("loadSnapshot() error = nil, want a not-found error")
	}
}

// TestLoadSnapshotRejectsWrongVersion confirms a version mismatch (a
// real C++ VER_STATE 0x102 snapshot, or a future incompatible Go
// format) is rejected rather than misdecoded as JSON.
func TestLoadSnapshotRejectsWrongVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wrongversion.snp")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := sdwfile.Encode(f, 0x102, []byte(`{}`), true); err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
	f.Close()

	if _, _, err := loadSnapshot(path); err == nil {
		t.Error("loadSnapshot() error = nil, want a version-mismatch error")
	}
}

// TestPrepareWithStateModeEmulReplaysSnapshot confirms StateModeEmul
// actually takes the snapshot-replay branch instead of falling
// through to real hardware detection - hardware.GetBaseBoard/
// GetSysInfoFast/ScanDevices all return ErrWindowsOnly on this
// (non-Windows) test machine, so Prepare succeeding at all here is
// itself proof the real-hardware calls were never reached.
func TestPrepareWithStateModeEmulReplaysSnapshot(t *testing.T) {
	s := settings.New()
	s.LogDir = t.TempDir()
	s.DrpDir, s.IndexDir = t.TempDir(), t.TempDir()
	saved := Prepared{
		System:  System{IsLaptop: true, BaseBoard: hardware.BaseBoard{Manufacturer: "Acme"}},
		devices: []hardware.Device{{InstanceID: "dev1", Description: "Test Device"}},
	}
	writeSnapshot(s, saved, testLogger)
	s.StateFile = filepath.Join(s.LogDir, listSnapFiles(t, s.LogDir)[0])
	s.StateMode = settings.StateModeEmul

	p, err := Prepare(s, testLogger)
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	if !reflect.DeepEqual(p.System, saved.System) {
		t.Errorf("System = %+v, want %+v", p.System, saved.System)
	}
	if len(p.devices) != 1 || p.devices[0].InstanceID != "dev1" {
		t.Errorf("devices = %+v, want the snapshot's one device", p.devices)
	}
}
