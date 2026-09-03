// Package indexing reads (and will eventually write) SDIO's compiled
// driver-pack index cache: the decompressed payload produced by
// internal/sdwfile.Decode. Ported from the data model in indexing.h/
// indexing.cpp (Driverpack, Collection, and the data_*_t record types),
// not the .inf-parsing/indexing pipeline itself (Parser, genindex,
// driverpack_indexinf_async, etc.), which comes later.
//
// All record types here use explicit int32/uint32 fields, never Go's
// platform-sized int, because their field order and 4-byte-per-field
// layout must match the original C++ structs exactly for
// encoding/binary to parse them correctly.
package indexing

// ofst is a byte offset into a Txt string blob, matching the `ofst`
// typedef (unsigned) in indexing.h/common.h.
type ofst = uint32

// NumVerNames mirrors NUM_VER_NAMES in indexing.h: the number of named
// .inf version-block fields tracked per file.
const NumVerNames = 11

// Indices into InfFile.Fields/Cats, ported from the anonymous enum in
// indexing.h.
const (
	FieldClassGUID = iota
	FieldClass
	FieldProvider
	FieldCatalogFile
	FieldCatalogFileNT
	FieldCatalogFileNTx86
	FieldCatalogFileNTIA64
	FieldCatalogFileNTAMD64
	FieldDriverVer
	FieldDriverPackageDisplayName
	FieldDriverPackageType
)

// rawVersion mirrors the Version class (common.h) as stored on disk:
// seven 4-byte fields, day/month/year plus a four-part version number.
// V1 < 0 means the version number is unset; Year < 1000 means the date
// is unset (see common.Version, which uses the same convention for the
// in-memory equivalent built from this).
type rawVersion struct {
	Day, Month, Year int32
	V1, V2, V3, V4   int32
}

// InfFile mirrors data_inffile_t (132 bytes on disk): one entry per
// .inf file indexed from a driver pack.
type InfFile struct {
	InfPath     ofst
	InfFilename ofst
	Fields      [NumVerNames]ofst
	Cats        [NumVerNames]ofst
	Version     rawVersion
	InfSize     int32
	InfCRC      int32
}

// Manufacturer mirrors data_manufacturer_t (16 bytes on disk): one
// entry per [Manufacturer] section block found in an .inf file.
type Manufacturer struct {
	InffileIndex uint32
	Manufacturer ofst
	Sections     ofst
	SectionsN    int32
}

// Desc mirrors data_desc_t (24 bytes on disk): one entry per device
// description line under a manufacturer section.
type Desc struct {
	ManufacturerIndex uint32
	SectPos           int32
	Desc              ofst
	Install           ofst
	InstallPicked     ofst
	Feature           uint32
}

// HWID mirrors data_HWID_t (12 bytes on disk): one entry per hardware
// ID a description line matches.
type HWID struct {
	DescIndex uint32
	InfPos    int32
	HWID      ofst
}

// HashItem mirrors Hashitem (common.h, 16 bytes on disk): one bucket
// entry in a Hashtable's open-chaining hash table.
type HashItem struct {
	Key      int32
	Value    int32
	Next     int32
	ValueLen int32
}
