package indexing

import (
	"bytes"
	"fmt"
)

// Index is one driver's compiled index decoded from an indexes/**/*.bin
// payload. It is the storage layer for collection matching.
type Index struct {
	InfFiles      []InfFile
	Manufacturers []Manufacturer
	Descs         []Desc
	HWIDs         []HWID
	Text          Txt
	Hashes        Hashtable
}

// DecodeIndex parses a decompressed .bin index payload into structured
// driver-pack index data. The read order is fixed by the on-disk
// format and must not change: inf files, manufacturers, descriptions,
// hardware IDs, the text blob, then the hash table.
func DecodeIndex(payload []byte) (*Index, error) {
	r := bytes.NewReader(payload)

	infFiles, err := readBlock[InfFile](r)
	if err != nil {
		return nil, fmt.Errorf("reading inf files: %w", err)
	}
	manufacturers, err := readBlock[Manufacturer](r)
	if err != nil {
		return nil, fmt.Errorf("reading manufacturers: %w", err)
	}
	descs, err := readBlock[Desc](r)
	if err != nil {
		return nil, fmt.Errorf("reading descriptions: %w", err)
	}
	hwids, err := readBlock[HWID](r)
	if err != nil {
		return nil, fmt.Errorf("reading hardware IDs: %w", err)
	}
	text, err := readBytesBlock(r)
	if err != nil {
		return nil, fmt.Errorf("reading text blob: %w", err)
	}
	hashes, err := decodeHashtable(r)
	if err != nil {
		return nil, fmt.Errorf("reading hash table: %w", err)
	}

	if remaining := r.Len(); remaining != 0 {
		return nil, fmt.Errorf("%d unexpected trailing bytes after hash table", remaining)
	}

	return &Index{
		InfFiles:      infFiles,
		Manufacturers: manufacturers,
		Descs:         descs,
		HWIDs:         hwids,
		Text:          Txt{Data: text},
		Hashes:        hashes,
	}, nil
}

// EncodeIndex serializes idx back to the same payload format
// DecodeIndex reads.
func EncodeIndex(idx *Index) ([]byte, error) {
	var buf bytes.Buffer

	if err := writeBlock(&buf, idx.InfFiles); err != nil {
		return nil, fmt.Errorf("writing inf files: %w", err)
	}
	if err := writeBlock(&buf, idx.Manufacturers); err != nil {
		return nil, fmt.Errorf("writing manufacturers: %w", err)
	}
	if err := writeBlock(&buf, idx.Descs); err != nil {
		return nil, fmt.Errorf("writing descriptions: %w", err)
	}
	if err := writeBlock(&buf, idx.HWIDs); err != nil {
		return nil, fmt.Errorf("writing hardware IDs: %w", err)
	}
	if err := writeBytesBlock(&buf, idx.Text.Data); err != nil {
		return nil, fmt.Errorf("writing text blob: %w", err)
	}
	if err := encodeHashtable(&buf, idx.Hashes); err != nil {
		return nil, fmt.Errorf("writing hash table: %w", err)
	}

	return buf.Bytes(), nil
}
