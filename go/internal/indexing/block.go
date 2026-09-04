package indexing

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// readBlock reads one loadable_vector<T>-shaped block: a 4-byte
// byte-count, a 4-byte element count, then that many fixed-size T
// records - ported from vector_load in common.h. T must be a struct of
// only fixed-size numeric fields (int32/uint32/[N]int32/...), since
// encoding/binary reads it field by field.
func readBlock[T any](r *bytes.Reader) ([]T, error) {
	var usedBytes, count int32
	if err := binary.Read(r, binary.LittleEndian, &usedBytes); err != nil {
		return nil, fmt.Errorf("reading block byte count: %w", err)
	}
	if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
		return nil, fmt.Errorf("reading block element count: %w", err)
	}

	var zero T
	if elemSize := int32(binary.Size(zero)); count == 0 && usedBytes > 0 && elemSize > 0 {
		// vector_load's "if(!num)num=sz" fallback.
		count = usedBytes / elemSize
	}

	items := make([]T, count)
	for i := range items {
		if err := binary.Read(r, binary.LittleEndian, &items[i]); err != nil {
			return nil, fmt.Errorf("reading block element %d: %w", i, err)
		}
	}
	return items, nil
}

// writeBlock writes items in the same loadable_vector<T> form
// readBlock reads.
func writeBlock[T any](w *bytes.Buffer, items []T) error {
	var zero T
	usedBytes := int32(binary.Size(zero)) * int32(len(items))

	if err := binary.Write(w, binary.LittleEndian, usedBytes); err != nil {
		return fmt.Errorf("writing block byte count: %w", err)
	}
	if err := binary.Write(w, binary.LittleEndian, int32(len(items))); err != nil {
		return fmt.Errorf("writing block element count: %w", err)
	}
	for i := range items {
		if err := binary.Write(w, binary.LittleEndian, items[i]); err != nil {
			return fmt.Errorf("writing block element %d: %w", i, err)
		}
	}
	return nil
}

// readBytesBlock reads a loadable_vector<char>-shaped block (used for
// Txt) directly as a byte slice, rather than through readBlock's
// per-element reflection - Txt blobs can be megabytes, and reading
// them one byte at a time via binary.Read would be slow.
func readBytesBlock(r *bytes.Reader) ([]byte, error) {
	var usedBytes, count int32
	if err := binary.Read(r, binary.LittleEndian, &usedBytes); err != nil {
		return nil, fmt.Errorf("reading byte-block byte count: %w", err)
	}
	if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
		return nil, fmt.Errorf("reading byte-block element count: %w", err)
	}
	n := count
	if n == 0 {
		n = usedBytes
	}

	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("reading byte-block data: %w", err)
	}
	return buf, nil
}

func writeBytesBlock(w *bytes.Buffer, data []byte) error {
	if err := binary.Write(w, binary.LittleEndian, int32(len(data))); err != nil {
		return fmt.Errorf("writing byte-block byte count: %w", err)
	}
	if err := binary.Write(w, binary.LittleEndian, int32(len(data))); err != nil {
		return fmt.Errorf("writing byte-block element count: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("writing byte-block data: %w", err)
	}
	return nil
}
