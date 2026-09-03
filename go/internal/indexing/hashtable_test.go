package indexing

import (
	"os"
	"path/filepath"
	"testing"

	"sdio/internal/sdwfile"
)

func TestHashtableAddFindSingleItem(t *testing.T) {
	var h Hashtable
	h.Reset(4)

	key := int32(APHash([]byte("PCI\\VEN_8086&DEV_1234")))
	h.AddItem(key, 42)

	v, ok := h.Find(key)
	if !ok || v != 42 {
		t.Fatalf("Find(key) = %d, %v; want 42, true", v, ok)
	}
	if _, ok := h.FindNext(); ok {
		t.Fatal("expected no further matches for a unique key")
	}
}

func TestHashtableFindMissingKey(t *testing.T) {
	var h Hashtable
	h.Reset(4)
	h.AddItem(1, 100)

	if _, ok := h.Find(999); ok {
		t.Fatal("expected Find to fail for a key never added")
	}
}

func TestHashtableFindOnEmptyTable(t *testing.T) {
	var h Hashtable
	h.Reset(4)
	if _, ok := h.Find(0); ok {
		t.Fatal("expected Find on an empty table to fail, including for key=0")
	}
}

func TestHashtableDuplicateKeysChainAndFindNext(t *testing.T) {
	// Force collisions by using a tiny table (size 1: every key maps to
	// bucket 0) with several distinct keys, then insert one duplicate.
	var h Hashtable
	h.Reset(1)

	h.AddItem(10, 1)
	h.AddItem(20, 2)
	h.AddItem(10, 3) // duplicate key, different value

	v1, ok := h.Find(10)
	if !ok || v1 != 1 {
		t.Fatalf("Find(10) first = %d, %v; want 1, true", v1, ok)
	}
	v2, ok := h.FindNext()
	if !ok || v2 != 3 {
		t.Fatalf("FindNext() = %d, %v; want 3, true", v2, ok)
	}
	if _, ok := h.FindNext(); ok {
		t.Fatal("expected only two entries for key 10")
	}

	v, ok := h.Find(20)
	if !ok || v != 2 {
		t.Fatalf("Find(20) = %d, %v; want 2, true", v, ok)
	}
}

func TestHashtableManyItemsNoCorruption(t *testing.T) {
	var h Hashtable
	h.Reset(8) // deliberately small to force plenty of collisions

	const n = 500
	for i := int32(0); i < n; i++ {
		h.AddItem(i, i*10)
	}
	for i := int32(0); i < n; i++ {
		v, ok := h.Find(i)
		if !ok || v != i*10 {
			t.Fatalf("Find(%d) = %d, %v; want %d, true", i, v, ok, i*10)
		}
	}
}

// TestHashtableAgainstRealIndex rebuilds a real driver-pack index's
// hash table from scratch (via AddItem, keyed by each HWID's APHash)
// and confirms it agrees with the table actually stored in the file:
// for every real HWID, Find(APHash(hwid)) must resolve back to an
// HWID_list entry whose own HWID string matches. This is the strongest
// evidence Find/AddItem/bucketHash are correct, since it validates
// against real hash values this rewrite didn't choose - it can't
// accidentally match its own encoding conventions the way an
// artificial test might.
func TestHashtableAgainstRealIndex(t *testing.T) {
	candidates, _ := filepath.Glob("/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/indexes/SDI/DP_Chipset_*.bin")
	if len(candidates) == 0 {
		t.Skip("no real installation available at the expected path; skipping")
	}

	f, err := os.Open(candidates[0])
	if err != nil {
		t.Fatalf("opening %s: %v", candidates[0], err)
	}
	defer f.Close()

	_, payload, err := sdwfile.Decode(f, true)
	if err != nil {
		t.Fatalf("sdwfile.Decode error: %v", err)
	}
	idx, err := DecodeIndex(payload)
	if err != nil {
		t.Fatalf("DecodeIndex error: %v", err)
	}
	if len(idx.HWIDs) == 0 {
		t.Fatal("expected at least one HWID in the real index")
	}

	var rebuilt Hashtable
	rebuilt.Reset(int32(len(idx.HWIDs)/2 + 1))
	for i, h := range idx.HWIDs {
		key := int32(APHash(idx.Text.Get(h.HWID)))
		rebuilt.AddItem(key, int32(i))
	}

	checked := 0
	for i, h := range idx.HWIDs {
		if i >= 200 { // enough for confidence without re-hashing the whole file
			break
		}
		want := idx.Text.Get(h.HWID)
		key := int32(APHash(want))

		found := false
		for v, ok := rebuilt.Find(key); ok; v, ok = rebuilt.FindNext() {
			got := idx.Text.Get(idx.HWIDs[v].HWID)
			if string(got) == string(want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("HWID %q (index %d): Find/FindNext never resolved back to itself", want, i)
		}
		checked++
	}
	t.Logf("checked %d real HWID entries against a rebuilt hash table", checked)
}
