package hardware

import "testing"

func TestWindowsVersionCount(t *testing.T) {
	if got := WindowsVersionCount(); got != 20 {
		t.Errorf("WindowsVersionCount() = %d, want 20", got)
	}
}

func TestWindowsVersionAtBounds(t *testing.T) {
	if _, ok := WindowsVersionAt(-1); ok {
		t.Error("WindowsVersionAt(-1) should be not-ok")
	}
	if _, ok := WindowsVersionAt(WindowsVersionCount()); ok {
		t.Error("WindowsVersionAt(Count()) should be not-ok")
	}
	v, ok := WindowsVersionAt(0)
	if !ok || v.Name != "Windows 2000" {
		t.Errorf("WindowsVersionAt(0) = %+v, %v", v, ok)
	}
}

func TestFindWindowsVersionDisambiguatesServer(t *testing.T) {
	if got := FindWindowsVersionName(61, false); got != "Windows 7" {
		t.Errorf("FindWindowsVersionName(61, false) = %q", got)
	}
	if got := FindWindowsVersionName(61, true); got != "Windows Server 2008 R2" {
		t.Errorf("FindWindowsVersionName(61, true) = %q", got)
	}
}

func TestFindWindowsVersionUnknown(t *testing.T) {
	if got := FindWindowsVersionIndex(999, false); got != -1 {
		t.Errorf("FindWindowsVersionIndex(999, false) = %d, want -1", got)
	}
	if got := FindWindowsVersionName(999, false); got != UnknownOSName {
		t.Errorf("FindWindowsVersionName(999, false) = %q, want %q", got, UnknownOSName)
	}
}

func TestFindWindowsVersionWindows11(t *testing.T) {
	i := FindWindowsVersionIndex(110, false)
	if i < 0 {
		t.Fatal("expected to find Windows 11 entry")
	}
	v, ok := WindowsVersionAt(i)
	if !ok || v.Name != "Windows 11" {
		t.Errorf("WindowsVersionAt(%d) = %+v", i, v)
	}
}
