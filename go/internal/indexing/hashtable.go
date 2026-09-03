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
// Find/FindNext (the chained lookup used by matcher.cpp) aren't ported
// yet - only the storage format, needed to read an index file's
// HWID -> record lookup table.
type Hashtable struct {
	Size  int32
	Items []HashItem
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
