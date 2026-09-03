package indexing

import (
	"bytes"
	"fmt"
	"unicode/utf16"
)

// osAttrMarker is the UTF-16LE encoding of "OSAtt", the start of the
// "OSAttrList" attribute name found in a Windows catalog (.cat) file's
// signed content, matched here as raw bytes against the file's ASN.1
// DER encoding rather than by parsing PKCS#7/ASN.1 structure.
var osAttrMarker = []byte{'O', 0, 'S', 0, 'A', 0, 't', 0, 't', 0}

// FindOSAttr scans a .cat file's raw bytes for its embedded OS
// compatibility attribute, ported from findosattr in indexing.cpp. It
// is a byte-pattern scan, not a real ASN.1 parse: every occurrence of
// the marker is followed by a length byte and then a UTF-16LE string
// such as "2:6.1,2:10.0" ("<ProductType>:<major>.<minor>" pairs), but
// two byte-alignment variants exist depending on the signing tool -
// confirmed against a real driver's .cat file, where the string
// consistently starts one byte later than the naive offset. The
// original peeks at that later byte and checks whether it looks like
// the start of a version string ('1' or '2') to decide which
// alignment applies. The longest string found across all occurrences
// in the file wins, matching a catalog that signs more than one file.
func FindOSAttr(data []byte) string {
	best := ""
	for i := 0; i+11 < len(data); i++ {
		if data[i] != 'O' || !bytes.Equal(data[i:i+10], osAttrMarker) {
			continue
		}
		ofs := 18
		if i+19 < len(data) && (data[i+19] == '1' || data[i+19] == '2') {
			ofs = 19
		}
		s := readUTF16LEZ(data, i+ofs)
		if len(s) > len(best) {
			best = s
		}
	}
	return best
}

// readUTF16LEZ decodes a null-terminated UTF-16LE string starting at
// start, stopping at the end of data if no terminator is found.
func readUTF16LEZ(data []byte, start int) string {
	var units []uint16
	for p := start; p+1 < len(data); p += 2 {
		u := uint16(data[p]) | uint16(data[p+1])<<8
		if u == 0 {
			break
		}
		units = append(units, u)
	}
	return string(utf16.Decode(units))
}

// IsValidCat reports whether a catalog file's OS-attribute string (as
// returned by FindOSAttr) covers the given Windows major/minor
// version, ported from the identical logic in Driver::isvalidcat
// (enum.cpp) and Hwidmatch::isvalidcat (matcher.cpp). Windows 11
// reports major version 11 but signs catalogs as "2:10.x", matching
// the original's major==11 -> 10 normalization.
func IsValidCat(catAttr string, major, minor int) bool {
	if catAttr == "" {
		return false
	}
	if major == 11 {
		major = 10
	}
	return bytes.Contains([]byte(catAttr), []byte(fmt.Sprintf("2:%d.%d", major, minor)))
}
