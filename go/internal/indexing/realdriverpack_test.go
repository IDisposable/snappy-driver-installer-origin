package indexing

import (
	"strings"
	"testing"

	"sdio/internal/archive"
	"sdio/internal/matcher"
)

// dtportInf is dtport.inf from a real driver pack (DP_Ports_SDIO01_26083.7z,
// "dt/allx64/DtPort_1.0.0.6/dtport.inf") on a production installation,
// with comment-only lines trimmed for brevity (not byte-identical to
// the source - see TestRealDriverPackFromArchive below for a test
// against the untouched archive bytes). Unlike Windows' own inbox .inf
// files (UTF-16LE, see infsections_test.go), third-party driver-pack
// .inf files are typically plain ANSI text with no BOM - this is the
// "_ansi" case Driverpack::indexinf_ansi's name refers to, and the
// actual shape of file this rewrite's indexing pipeline needs to
// handle in production.
const dtportInf = `[Version]
Signature="$Windows NT$"
DriverPackageType=PlugAndPlay
DriverPackageDisplayName=%DESC%
Class=Ports
ClassGUID={4d36e978-e325-11ce-bfc1-08002be10318}
Provider=%DT%
CatalogFile=dtport.cat
DriverVer = 12/22/2025,1.0.0.6
PnpLockdown=1

[SourceDisksNames]
1=%DriversDisk%,,,

[SourceDisksFiles]
dtport.sys  = 1
WdfCoInstaller01009.dll=1

[SourceDisksFiles.amd64]
dtport.sys  = 1
WdfCoInstaller01009.dll=1

[DestinationDirs]
DtPort.NT.Copy=12
DtPort_Device_CoInstaller_CopyFiles = 11

[ControlFlags]
ExcludeFromSelect=*

[Manufacturer]
%DT%=DtHw,NTamd64

[DtHw]
%VID_37DD&PID_6001.DeviceDesc%=DtPort.NT,DTBUS\COMPORT&VID_37DD&PID_6001

[DtHw.NTamd64]
%VID_37DD&PID_6001.DeviceDesc%=DtPort.NTamd64,DTBUS\COMPORT&VID_37DD&PID_6001


[DtPort.NT.AddService]
DisplayName    = %SvcDesc%
ServiceType    = 1 ; SERVICE_KERNEL_DRIVER
StartType      = 3 ; SERVICE_DEMAND_START
ErrorControl   = 1 ; SERVICE_ERROR_NORMAL
ServiceBinary  = %12%\dtport.sys
LoadOrderGroup = Base

[DtPort.NT.AddReg]
HKR,,EnumPropPages32,,"MsPorts.dll,SerialPortPropPageProvider"

[DtPort.NT.Copy]
dtport.sys

[DtPort.NT]
CopyFiles=DtPort.NT.Copy
AddReg=DtPort.NT.AddReg

[DtPort.NTamd64]
CopyFiles=DtPort.NT.Copy
AddReg=DtPort.NT.AddReg

[DtPort.NT.HW]
AddReg=DtPort.NT.HW.AddReg

[DtPort.NTamd64.HW]
AddReg=DtPort.NT.HW.AddReg

[DtPort.NT.Services]
AddService = DTPORTSER, 0x00000002, DtPort.NT.AddService

[DtPort.NTamd64.Services]
AddService = DTPORTSER, 0x00000002, DtPort.NT.AddService

[DtPort.NT.HW.AddReg]
HKR,,"MinReadTimeout",0x00010001,0
HKR,,"MinWriteTimeout",0x00010001,0
HKR,,"LatencyTimer",0x00010001,16
HKR,,Security,,"D:(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;WD)"

[DtPort.NT.CoInstallers]
AddReg=DtPort_Device_CoInstaller_AddReg
CopyFiles=DtPort_Device_CoInstaller_CopyFiles

[DtPort.NTamd64.CoInstallers]
AddReg=DtPort_Device_CoInstaller_AddReg
CopyFiles=DtPort_Device_CoInstaller_CopyFiles

[DtPort_Device_CoInstaller_AddReg]
HKR,,CoInstallers32,0x00010000, "WdfCoInstaller01009.dll,WdfCoInstaller"

[DtPort_Device_CoInstaller_CopyFiles]
WdfCoInstaller01009.dll

[DtPort.NT.Wdf]
KmdfService= DTPORTSER,Dtport_sect

[DtPort.NTamd64.Wdf]
KmdfService= DTPORTSER,Dtport_sect

[Dtport_sect]
KmdfLibraryVersion=1.9

[Strings]
DT="DT"
DESC="CDM Driver Package - VCP Driver"
DriversDisk="DT USB Drivers Disk"
PortsClassName = "Ports (COM & LPT)"
VID_37DD&PID_6001.DeviceDesc="USB Serial Port"
SvcDesc="DT USB Serial Port Driver"
`

// TestRealDriverPackInfFullPipeline runs every non-deferred piece
// (DiscoverSections, ParseStrings, ParseVersionSection,
// ParseManufacturers, ResolveManufacturerSection) against a real,
// plain-ANSI driver-pack .inf file end-to-end, checking results
// against the file's actual content (verified by manual inspection).
// This is the production case the pipeline needs to handle - unlike
// Windows' own UTF-16 inbox .inf files, exercised elsewhere.
func TestRealDriverPackInfFullPipeline(t *testing.T) {
	data := []byte(dtportInf)

	sections, _ := DiscoverSections(data)
	strs := ParseStrings(data, sections)

	if strs["dt"] != "DT" {
		t.Errorf(`strs["dt"] = %q, want "DT"`, strs["dt"])
	}
	if strs["desc"] != "CDM Driver Package - VCP Driver" {
		t.Errorf(`strs["desc"] = %q`, strs["desc"])
	}

	info := ParseVersionSection(data, sections, strs)
	if info.Fields[FieldClass] != "Ports" {
		t.Errorf("Fields[FieldClass] = %q, want %q", info.Fields[FieldClass], "Ports")
	}
	if info.Fields[FieldClassGUID] != "{4d36e978-e325-11ce-bfc1-08002be10318}" {
		t.Errorf("Fields[FieldClassGUID] = %q", info.Fields[FieldClassGUID])
	}
	if info.Fields[FieldProvider] != "DT" {
		t.Errorf("Fields[FieldProvider] (substituted %%DT%%) = %q, want %q", info.Fields[FieldProvider], "DT")
	}
	if info.Fields[FieldCatalogFile] != "dtport.cat" {
		t.Errorf("Fields[FieldCatalogFile] = %q", info.Fields[FieldCatalogFile])
	}
	if info.Version.Day != 22 || info.Version.Month != 12 || info.Version.Year != 2025 {
		t.Errorf("Version date = %+v, want Day=22 Month=12 Year=2025", info.Version)
	}
	if info.Version.V1 != 1 || info.Version.V2 != 0 || info.Version.V3 != 0 || info.Version.V4 != 6 {
		t.Errorf("Version number = %+v, want 1.0.0.6", info.Version)
	}

	entries := ParseManufacturers(data, sections, strs)
	if len(entries) != 1 {
		t.Fatalf("got %d manufacturer entries, want 1", len(entries))
	}
	mfg := entries[0]
	if mfg.Name != "DT" {
		t.Errorf("manufacturer Name = %q, want substituted %q", mfg.Name, "DT")
	}
	if mfg.SectionRoot != "dthw" {
		t.Errorf("SectionRoot = %q, want %q", mfg.SectionRoot, "dthw")
	}
	wantSections := []string{"dthw", "dthw.ntamd64"}
	if len(mfg.Sections) != len(wantSections) || mfg.Sections[0] != wantSections[0] || mfg.Sections[1] != wantSections[1] {
		t.Fatalf("Sections = %v, want %v", mfg.Sections, wantSections)
	}

	devices := ResolveManufacturerSection(data, sections, "dthw.ntamd64", "ntamd64", strs, matcher.OSDecorations[:])
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	d := devices[0]
	if d.Description != "USB Serial Port" {
		t.Errorf("Description (substituted) = %q, want %q", d.Description, "USB Serial Port")
	}
	// "DtPort.NTamd64" resolves directly via phase (b): the real file
	// has a bare "[DtPort.NTamd64]" section (no ".nt" suffix variant).
	if d.InstallPicked != "dtport.ntamd64" {
		t.Errorf("InstallPicked = %q, want %q", d.InstallPicked, "dtport.ntamd64")
	}
	if d.Feature != defaultFeature {
		t.Errorf("Feature = %#x, want defaultFeature (no FeatureScore key in this file)", d.Feature)
	}
	wantHWID := `DTBUS\COMPORT&VID_37DD&PID_6001`
	if len(d.HWIDs) != 1 || !strings.EqualFold(d.HWIDs[0], wantHWID) {
		t.Errorf("HWIDs = %v, want [%q]", d.HWIDs, wantHWID)
	}
}

// TestRealDriverPackFromArchive runs the same pipeline as
// TestRealDriverPackInfFullPipeline, but against the untouched bytes
// extracted live from a real driver-pack .7z archive via
// internal/archive, rather than the trimmed dtportInf constant above.
// This is the actual production path (archive -> extract -> parse)
// end-to-end, and confirms dtportInf's trimming didn't accidentally
// change anything the pipeline cares about.
func TestRealDriverPackFromArchive(t *testing.T) {
	const path = "/mnt/d/OneDrive/Desktop/Reinstall/DriverInstaller/drivers/DP_Ports_SDIO01_26083.7z"
	const infName = "dt/allx64/DtPort_1.0.0.6/dtport.inf"

	r, err := archive.Open(path)
	if err != nil {
		t.Skipf("real driver pack not available at %s: %v", path, err)
	}
	defer r.Close()

	data, err := r.Extract(infName)
	if err != nil {
		t.Fatalf("Extract(%s) error: %v", infName, err)
	}

	sections, _ := DiscoverSections(data)
	strs := ParseStrings(data, sections)
	info := ParseVersionSection(data, sections, strs)

	if info.Fields[FieldClass] != "Ports" {
		t.Errorf("Fields[FieldClass] = %q, want %q", info.Fields[FieldClass], "Ports")
	}
	if info.Version.Day != 22 || info.Version.Month != 12 || info.Version.Year != 2025 {
		t.Errorf("Version date = %+v, want Day=22 Month=12 Year=2025", info.Version)
	}
	if info.Version.V1 != 1 || info.Version.V4 != 6 {
		t.Errorf("Version number = %+v, want 1.0.0.6", info.Version)
	}

	entries := ParseManufacturers(data, sections, strs)
	if len(entries) != 1 || entries[0].SectionRoot != "dthw" {
		t.Fatalf("manufacturer entries = %+v, want one with SectionRoot \"dthw\"", entries)
	}

	devices := ResolveManufacturerSection(data, sections, "dthw.ntamd64", "ntamd64", strs, matcher.OSDecorations[:])
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	wantHWID := `DTBUS\COMPORT&VID_37DD&PID_6001`
	if len(devices[0].HWIDs) != 1 || !strings.EqualFold(devices[0].HWIDs[0], wantHWID) {
		t.Errorf("HWIDs = %v, want [%q]", devices[0].HWIDs, wantHWID)
	}
}
