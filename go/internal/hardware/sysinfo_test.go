package hardware

import "testing"

func TestIsWide(t *testing.T) {
	if isWide(MonitorSize{}) {
		t.Error("zero-width monitor should not be wide")
	}
	if isWide(MonitorSize{WidthCM: 30, HeightCM: 20}) {
		t.Error("4:3-ish monitor should not be wide")
	}
	if !isWide(MonitorSize{WidthCM: 30, HeightCM: 45}) {
		t.Error("16:9-ish monitor should be wide")
	}
}

func TestIsLaptopChassisTypeShortCircuits(t *testing.T) {
	if IsLaptop(3, nil, BatteryStatus{NoBattery: true}, false) {
		t.Error("chassis type 3 (Desktop) should never be a laptop")
	}
	if !IsLaptop(10, nil, BatteryStatus{NoBattery: true}, false) {
		t.Error("chassis type 10 (Notebook) should always be a laptop")
	}
}

func TestIsLaptopNoBatteryNoACPIDevice(t *testing.T) {
	if IsLaptop(0, []MonitorSize{{WidthCM: 30, HeightCM: 45}}, BatteryStatus{NoBattery: true}, false) {
		t.Error("no battery and no ACPI battery device should not be a laptop")
	}
}

func TestIsLaptopSmallWideMonitorWithBattery(t *testing.T) {
	// A ~13" widescreen panel, true physical size ~16cm x 29cm. Per
	// MonitorSize's inverted WidthCM/HeightCM convention (see its doc
	// comment), that's WidthCM=16 (true height), HeightCM=29 (true
	// width): diag = sqrt(16^2+29^2)/2.54 =~ 13cm, isWide = 29/16 =~
	// 1.8 > 1.35.
	small := MonitorSize{WidthCM: 16, HeightCM: 29}
	if !IsLaptop(0, []MonitorSize{small}, BatteryStatus{NoBattery: false}, false) {
		t.Error("small widescreen monitor with a battery present should be a laptop")
	}
}

func TestIsLaptopLargeMonitorWithBattery(t *testing.T) {
	// A 27" external widescreen monitor, true physical size ~60cm x
	// 34cm. Inverted: WidthCM=34 (true height), HeightCM=60 (true
	// width): diag =~ 27cm (over the 18cm laptop threshold), isWide =
	// 60/34 =~ 1.8 > 1.35.
	large := MonitorSize{WidthCM: 34, HeightCM: 60}
	if IsLaptop(0, []MonitorSize{large}, BatteryStatus{NoBattery: false}, false) {
		t.Error("a large external monitor should not be classified as a laptop")
	}
}

func TestIsLaptopNoMonitorsWithBattery(t *testing.T) {
	if !IsLaptop(0, nil, BatteryStatus{NoBattery: false}, false) {
		t.Error("a battery present with no monitors detected should default to laptop")
	}
}

func TestIsLaptopACPIBatteryDeviceOverridesNoBatteryFlag(t *testing.T) {
	if !IsLaptop(0, nil, BatteryStatus{NoBattery: true}, true) {
		t.Error("an ACPI0003 battery device should count as having a battery")
	}
}

func TestWindowsVersionInfoIsServer(t *testing.T) {
	if (WindowsVersionInfo{ProductType: 1}).IsServer() {
		t.Error("ProductType 1 should not be a server")
	}
	if !(WindowsVersionInfo{ProductType: 3}).IsServer() {
		t.Error("ProductType 3 should be a server")
	}
}
