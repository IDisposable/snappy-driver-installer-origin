package indexing

import "testing"

// sect is the *install* section actually used (InfSection+InfSectionExt
// from the registry), not the manufacturer's device-list section: the
// .inf's [DtHw.NTamd64] line reads
// "...DeviceDesc%=DtPort.NTamd64,DTBUS\..." - the device list is
// "DtHw.NTamd64" but the install target it names is "DtPort.NTamd64".

func TestScanInstalledInfDecoratedSection(t *testing.T) {
	data := []byte(dtportInf)
	info := ScanInstalledInf(data, "DtPort.NTamd64", `DTBUS\COMPORT&VID_37DD&PID_6001`, nil)

	if !info.Found {
		t.Fatal("expected a match for the decorated install section")
	}
	if info.InfPos != 0 {
		t.Errorf("InfPos = %d, want 0 (only HWID on the line)", info.InfPos)
	}
	if info.Feature != defaultFeature {
		t.Errorf("Feature = %#x, want %#x (no featurescore key in dtportInf)", info.Feature, defaultFeature)
	}
	if info.CatalogFileBits != 1<<FieldCatalogFile {
		t.Errorf("CatalogFileBits = %#x, want %#x (only CatalogFile= is set)", info.CatalogFileBits, 1<<FieldCatalogFile)
	}
	if !info.IsNTSection {
		t.Error("IsNTSection = false, want true for \"DtPort.NTamd64\"")
	}
}

func TestScanInstalledInfBareSection(t *testing.T) {
	data := []byte(dtportInf)
	info := ScanInstalledInf(data, "DtPort.NT", `DTBUS\COMPORT&VID_37DD&PID_6001`, nil)

	if !info.Found {
		t.Fatal("expected a match for the bare (non-decorated) install section")
	}
}

func TestScanInstalledInfWrongSectionNotFound(t *testing.T) {
	data := []byte(dtportInf)
	info := ScanInstalledInf(data, "SomeOtherSection", `DTBUS\COMPORT&VID_37DD&PID_6001`, nil)

	if info.Found {
		t.Error("expected no match for a section this HWID isn't declared under")
	}
	if info.Feature != defaultFeature {
		t.Errorf("Feature = %#x, want the default sentinel on a non-match", info.Feature)
	}
	// CatalogFileBits is computed independently of the HWID/section
	// search (matches fillinfo not resetting catalogfile on failure).
	if info.CatalogFileBits != 1<<FieldCatalogFile {
		t.Errorf("CatalogFileBits = %#x, want %#x even when the HWID/section search fails", info.CatalogFileBits, 1<<FieldCatalogFile)
	}
}

func TestScanInstalledInfWrongHWIDNotFound(t *testing.T) {
	data := []byte(dtportInf)
	info := ScanInstalledInf(data, "DtHw.NTamd64", `NOMATCH\VID_0000&PID_0000`, nil)

	if info.Found {
		t.Error("expected no match for an unrelated hardware ID")
	}
}

func TestScanInstalledInfMalformedData(t *testing.T) {
	info := ScanInstalledInf([]byte("not an inf file at all"), "Section", "HWID", nil)
	if info.Found {
		t.Error("expected no match against garbage input")
	}
}
