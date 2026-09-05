package update

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPromotePendingIndexRenamesToRealName confirms the underscore-
// prefixed placeholder becomes the real DP_*.bin name, content
// preserved.
func TestPromotePendingIndexRenamesToRealName(t *testing.T) {
	dir := t.TempDir()
	pending := filepath.Join(dir, "_P_Ports_SDIO01_26083.bin")
	if err := os.WriteFile(pending, []byte("real index data"), 0o644); err != nil {
		t.Fatalf("writing pending fixture: %v", err)
	}

	if err := PromotePendingIndex(dir, "DP_Ports_SDIO01_26083.7z"); err != nil {
		t.Fatalf("PromotePendingIndex() error: %v", err)
	}

	real := filepath.Join(dir, "DP_Ports_SDIO01_26083.bin")
	data, err := os.ReadFile(real)
	if err != nil {
		t.Fatalf("reading promoted file: %v", err)
	}
	if string(data) != "real index data" {
		t.Errorf("promoted file content = %q, want %q", data, "real index data")
	}
	if _, err := os.Stat(pending); err == nil {
		t.Error("pending placeholder still exists after promotion")
	}
}

// TestPromotePendingIndexNoOpWhenNothingPending confirms a pack with
// no pending placeholder (the normal case - most packs never had one)
// is silently a no-op, not an error.
func TestPromotePendingIndexNoOpWhenNothingPending(t *testing.T) {
	dir := t.TempDir()
	if err := PromotePendingIndex(dir, "DP_Ports_SDIO01_26083.7z"); err != nil {
		t.Errorf("PromotePendingIndex() error = %v, want nil when nothing is pending", err)
	}
}

// TestPromotePendingIndexOverwritesExistingRealFile confirms promotion
// still succeeds when a real index of the same name already exists at
// the destination (a stale leftover, or - reported live - a case that
// tripped a bare os.Rename with "Access is denied" on Windows for
// several packs in the same batch). SaveFile's copy-then-remove
// fallback replaces it rather than erroring out.
func TestPromotePendingIndexOverwritesExistingRealFile(t *testing.T) {
	dir := t.TempDir()
	pending := filepath.Join(dir, "_P_Ports_SDIO01_26083.bin")
	real := filepath.Join(dir, "DP_Ports_SDIO01_26083.bin")
	if err := os.WriteFile(pending, []byte("new real data"), 0o644); err != nil {
		t.Fatalf("writing pending fixture: %v", err)
	}
	if err := os.WriteFile(real, []byte("stale leftover"), 0o644); err != nil {
		t.Fatalf("writing pre-existing real fixture: %v", err)
	}

	if err := PromotePendingIndex(dir, "DP_Ports_SDIO01_26083.7z"); err != nil {
		t.Fatalf("PromotePendingIndex() error: %v", err)
	}

	data, err := os.ReadFile(real)
	if err != nil {
		t.Fatalf("reading promoted file: %v", err)
	}
	if string(data) != "new real data" {
		t.Errorf("promoted file content = %q, want the pending index's content %q", data, "new real data")
	}
}
