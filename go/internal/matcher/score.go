package matcher

// Catalog-file field bit positions, matching indexing.FieldCatalogFile.. in
// internal/indexing (kept as local constants rather than importing
// that package, since internal/indexing already imports
// internal/matcher for OSDecorations - importing back would cycle).
// Ported from the CatalogFile.. range used in calc_signature/
// Hwidmatch::calc_catalogfile in enum.cpp/matcher.cpp.
const (
	CatalogFileBit        = 1 << 3 // indexing.FieldCatalogFile
	CatalogFileNTBit      = 1 << 4 // indexing.FieldCatalogFileNT
	CatalogFileNTx86Bit   = 1 << 5 // indexing.FieldCatalogFileNTx86
	CatalogFileNTIA64Bit  = 1 << 6 // indexing.FieldCatalogFileNTIA64
	CatalogFileNTAMD64Bit = 1 << 7 // indexing.FieldCatalogFileNTAMD64
)

// SignatureScore computes the catalog-signature component of a
// driver's overall Score, ported from calc_signature in enum.cpp.
// is64Bit is the running system's architecture; isNTSection reports
// whether the driver's chosen install section is ".nt"-decorated
// (checked via strings.Contains on InfSection/InfSectionExt in the
// original - that check itself isn't ported here, since it needs the
// not-yet-ported Driver type; callers compute isNTSection themselves).
func SignatureScore(catalogFileBits int, is64Bit bool, isNTSection bool) int {
	if is64Bit {
		if catalogFileBits&(CatalogFileBit|CatalogFileNTBit|CatalogFileNTAMD64Bit|CatalogFileNTIA64Bit) != 0 {
			return 0
		}
	} else {
		if catalogFileBits&(CatalogFileBit|CatalogFileNTBit|CatalogFileNTx86Bit) != 0 {
			return 0
		}
	}
	if isNTSection {
		return 0x8000
	}
	return 0xC000
}

// Score computes a driver candidate's overall match score, ported from
// calc_score in enum.cpp. feature is the "featurescore" value from the
// resolved install section (see indexing.ResolvedDevice.Feature, or
// indexing.defaultFeature if none); identifierScore comes from
// IdentifierScore; major is the running Windows major version (10 and
// later use a richer score layout folding in the signature and
// feature bits; older versions use a simpler one).
func Score(catalogFileBits, feature, identifierScore, major int, is64Bit, isNTSection bool) uint32 {
	sig := SignatureScore(catalogFileBits, is64Bit, isNTSection)
	if major >= 6 {
		return uint32(sig<<16) + uint32(feature<<16) + uint32(identifierScore)
	}
	return uint32(sig + identifierScore)
}

// IdentifierScore ranks how directly a device's hardware/compatible ID
// matched an .inf file's hardware/compatible ID list, ported from
// calc_identifierscore in enum.cpp. Lower is better: an exact
// hardware-ID-to-hardware-ID match at position 0 scores 0x0000, up to
// a compatible-ID-to-compatible-ID match scoring 0x3000 or higher.
// devPos is the matched ID's position in the .inf file's HWID list;
// deviceIsHardwareID reports whether the match came from the device's
// own hardware ID (true) or one of its compatible IDs (false); infPos
// is the matched position within whichever of the device's own ID
// lists was used.
func IdentifierScore(devPos int, deviceIsHardwareID bool, infPos int) int {
	switch {
	case deviceIsHardwareID && infPos == 0:
		return devPos
	case deviceIsHardwareID:
		return 0x1000 + devPos + 0x100*infPos
	case infPos == 0:
		return 0x2000 + devPos
	default:
		return 0x3000 + devPos + 0x100*infPos
	}
}
