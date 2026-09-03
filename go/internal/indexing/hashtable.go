package indexing

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Hashtable is an open-chaining hash table mapping int32 keys to int32
// values, ported from Hashtable in common.h/common.cpp. Size is the
// bucket count; Items holds both the buckets (indices 0..Size-1) and
// any overflow chain entries appended after them.
//
// Keys are typically themselves a hash of some other value (e.g. a
// hardware ID string's APHash) - Find/AddItem re-hash the raw bytes of
// the int32 key a second time to pick a bucket, so that similar string
// hashes don't cluster into the same bucket. findNextIdx/findKey hold
// FindNext's cursor between calls, matching the original's
// findnext_v/findstr instance fields.
type Hashtable struct {
	Size  int32
	Items []HashItem

	findNextIdx int32
	findKey     int32
}

// Reset initializes h with the given bucket count (at least 1) and
// clears all items, ported from Hashtable::reset.
func (h *Hashtable) Reset(size int32) {
	if size == 0 {
		size = 1
	}
	h.Size = size
	h.Items = make([]HashItem, size)
}

// bucketHash re-hashes an int32 key's raw little-endian bytes, ported
// from the gethashcode((char*)&key, sizeof(int)) calls in
// Hashtable::additem/find.
func bucketHash(key int32) uint32 {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(key))
	return APHash(b[:])
}

// AddItem inserts a key/value pair, appending a new item to the
// bucket's collision chain if the bucket is already occupied, ported
// from Hashtable::additem.
func (h *Hashtable) AddItem(key, value int32) {
	curi := int32(bucketHash(key) % uint32(h.Size))
	previ := int32(-1)

	if h.Items[curi].Next != 0 {
		for {
			previ = curi
			next := h.Items[curi].Next
			if next <= 0 {
				break
			}
			curi = next
		}
	}

	if h.Items[curi].Next == -1 {
		h.Items = append(h.Items, HashItem{})
		curi = int32(len(h.Items) - 1)
	}

	h.Items[curi].Key = key
	h.Items[curi].Value = value
	h.Items[curi].Next = -1
	if previ >= 0 {
		h.Items[previ].Next = curi
	}
}

// Find looks up key (typically a string's APHash), returning the
// first matching value. Call FindNext afterward to retrieve further
// values inserted under the same key (duplicate keys are legal - e.g.
// multiple driver-pack entries sharing one hardware ID). Ported from
// Hashtable::find.
func (h *Hashtable) Find(key int32) (value int32, found bool) {
	if h.Size == 0 {
		return 0, false
	}

	bucket := int32(bucketHash(key) % uint32(h.Size))
	if h.Items[bucket].Next == 0 {
		return 0, false // untouched bucket
	}

	idx := bucket
	for {
		cur := h.Items[idx]
		if key == cur.Key {
			h.findNextIdx = cur.Next
			h.findKey = key
			return cur.Value, true
		}
		if cur.Next <= 0 {
			break
		}
		idx = cur.Next
	}
	return 0, false
}

// FindNext continues a lookup started by Find, returning further
// values inserted under the same key. Ported from Hashtable::findnext.
func (h *Hashtable) FindNext() (value int32, found bool) {
	idx := h.findNextIdx
	if idx <= 0 {
		return 0, false
	}
	for {
		cur := h.Items[idx]
		if cur.Key == h.findKey {
			h.findNextIdx = cur.Next
			return cur.Value, true
		}
		if cur.Next <= 0 {
			break
		}
		idx = cur.Next
	}
	return 0, false
}

func decodeHashtable(r *bytes.Reader) (Hashtable, error) {
	var size int32
	if err := binary.Read(r, binary.LittleEndian, &size); err != nil {
		return Hashtable{}, fmt.Errorf("reading hashtable size: %w", err)
	}
	items, err := readBlock[HashItem](r)
	if err != nil {
		return Hashtable{}, fmt.Errorf("reading hashtable items: %w", err)
	}
	return Hashtable{Size: size, Items: items}, nil
}

func encodeHashtable(w *bytes.Buffer, h Hashtable) error {
	if err := binary.Write(w, binary.LittleEndian, h.Size); err != nil {
		return fmt.Errorf("writing hashtable size: %w", err)
	}
	return writeBlock(w, h.Items)
}

// APHash is the hash function Hashtable uses (Partow's AP hash),
// ported from Hashtable::APHash in common.cpp. The original reads
// through a (platform-default, likely signed) `char *`; b is sign-
// extended as an int8 to match that, which only matters for non-ASCII
// bytes (hardware ID strings are ASCII in practice).
func APHash(s []byte) uint32 {
	hash := uint32(0xAAAAAAAA)
	for i, raw := range s {
		c := uint32(int32(int8(raw)))
		if i&1 == 0 {
			hash ^= (hash << 7) ^ (c * (hash >> 3))
		} else {
			hash ^= ^((hash << 11) + (c ^ (hash >> 5)))
		}
	}
	return hash
}
