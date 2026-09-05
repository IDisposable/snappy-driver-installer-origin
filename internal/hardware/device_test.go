package hardware

import "testing"

func TestGUIDString(t *testing.T) {
	g := GUID{
		Data1: 0x4D36E96E,
		Data2: 0xE325,
		Data3: 0x11CE,
		Data4: [8]byte{0xBF, 0xC1, 0x08, 0x00, 0x2B, 0xE1, 0x03, 0x18},
	}
	want := "4D36E96E-E325-11CE-BFC1-08002BE10318"
	if got := g.String(); got != want {
		t.Errorf("GUID.String() = %q, want %q", got, want)
	}
}

func TestDeviceStatusNotPresent(t *testing.T) {
	if got := deviceStatus(true, dnStarted, 0); got != DeviceNotPresent {
		t.Errorf("deviceStatus(notPresent=true, ...) = %v, want DeviceNotPresent", got)
	}
}

func TestDeviceStatusDisabled(t *testing.T) {
	got := deviceStatus(false, dnHasProblem, deviceCMProbDisabled)
	if got != DeviceDisabled {
		t.Errorf("deviceStatus(HasProblem, CM_PROB_DISABLED) = %v, want DeviceDisabled", got)
	}
}

func TestDeviceStatusHasProblem(t *testing.T) {
	got := deviceStatus(false, dnHasProblem, 99 /* some other problem code */)
	if got != DeviceHasProblem {
		t.Errorf("deviceStatus(HasProblem, other) = %v, want DeviceHasProblem", got)
	}
}

func TestDeviceStatusPrivateProblem(t *testing.T) {
	got := deviceStatus(false, dnPrivateProblem, 0)
	if got != DevicePrivateProblem {
		t.Errorf("deviceStatus(PrivateProblem) = %v, want DevicePrivateProblem", got)
	}
}

func TestDeviceStatusRunning(t *testing.T) {
	got := deviceStatus(false, dnStarted, 0)
	if got != DeviceRunning {
		t.Errorf("deviceStatus(Started) = %v, want DeviceRunning", got)
	}
}

func TestDeviceStatusStopped(t *testing.T) {
	got := deviceStatus(false, 0, 0)
	if got != DeviceStopped {
		t.Errorf("deviceStatus(no flags) = %v, want DeviceStopped", got)
	}
}

func TestDeviceStatusMethod(t *testing.T) {
	d := Device{RawStatusFlags: dnStarted}
	if got := d.Status(); got != DeviceRunning {
		t.Errorf("Device.Status() = %v, want DeviceRunning", got)
	}
}
