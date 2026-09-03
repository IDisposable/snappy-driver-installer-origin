package indexing

import "unicode/utf16"

// Txt is a driver-pack index's interned string blob, ported from Txt
// in common.h/common.cpp. Strings are stored null-terminated, either
// as narrow (ANSI/UTF-8-ish 8-bit) or as UTF-16LE, depending on which
// original method (strcpy vs strcpyw) wrote them; which encoding
// applies to which field is a property of the higher-level record that
// holds the offset; determined when indexing.cpp's .inf-parsing/
// field-population logic is ported.
type Txt struct {
	Data []byte
}

// Get returns the null-terminated narrow-string bytes starting at
// offset, ported from Txt::get. Returns nil if offset is out of range.
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

// GetString is Get decoded as a string (the original's ANSI codepage
// vs. UTF-8 handling isn't replicated - see the package doc comment on
// why decoding narrow strings needs more context than this type has).
func (t Txt) GetString(offset ofst) string {
	return string(t.Get(offset))
}

// GetW returns the null-terminated UTF-16LE string starting at offset,
// decoded to a Go string, ported from Txt::getw.
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
