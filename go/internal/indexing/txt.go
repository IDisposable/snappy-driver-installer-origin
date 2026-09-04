package indexing

import "unicode/utf16"

// Txt is a driver-pack index's interned string blob. Strings are
// stored null-terminated, either as narrow (ANSI/UTF-8-ish 8-bit, read
// by GetString) or as UTF-16LE (read by GetW) - which encoding applies
// is a property of the higher-level record that holds the offset, not
// of Txt itself.
type Txt struct {
	Data []byte
}

// Get returns the null-terminated narrow-string bytes starting at
// offset. Returns nil if offset is out of range.
func (t Txt) Get(offset ofst) []byte {
	i := int(offset)
	if i < 0 || i >= len(t.Data) {
		return nil
	}
	end := i
	for end < len(t.Data) && t.Data[end] != 0 {
		end++
	}
	return t.Data[i:end]
}

// GetString is Get decoded as a string, without any ANSI-codepage
// translation - verified against real driver-pack data as correct for
// every field this rewrite currently reads (hardware IDs, paths,
// manufacturer/description text); a field that turns out to need real
// codepage handling would need its own decoding, not a Txt change.
func (t Txt) GetString(offset ofst) string {
	return string(t.Get(offset))
}

// GetW returns the null-terminated UTF-16LE string starting at offset,
// decoded to a Go string.
func (t Txt) GetW(offset ofst) string {
	i := int(offset)
	var u16 []uint16
	for i+1 < len(t.Data) {
		c := uint16(t.Data[i]) | uint16(t.Data[i+1])<<8
		if c == 0 {
			break
		}
		u16 = append(u16, c)
		i += 2
	}
	return string(utf16.Decode(u16))
}
