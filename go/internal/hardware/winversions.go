package hardware

// WindowsVersion identifies one entry in the table of known Windows
// releases. Number encodes major*10+minor (e.g. 61 for both Windows 7
// and Server 2008 R2 - Server distinguishes the two).
type WindowsVersion struct {
	Number int
	Server bool
	Name   string
}

// UnknownOSName is returned by FindWindowsVersionName when no entry
// matches.
const UnknownOSName = "Unknown OS"

// windowsVersions mirrors WinVersions::_versions. See
// https://learn.microsoft.com/windows/win32/sysinfo/operating-system-version
var windowsVersions = []WindowsVersion{
	{50, false, "Windows 2000"},
	{51, false, "Windows XP"},
	{52, false, "Windows XP 64"},
	{52, true, "Windows Server 2003"},
	{52, true, "Windows Server 2003 R2"},
	{60, false, "Windows Vista"},
	{60, true, "Windows Server 2008"},
	{61, true, "Windows Server 2008 R2"},
	{61, false, "Windows 7"},
	{62, true, "Windows Server 2012"},
	{62, false, "Windows 8"},
	{63, true, "Windows Server 2012 R2"},
	{63, false, "Windows 8.1"},
	{64, false, "Windows 10 Tech Preview"},
	{100, true, "Windows Server 2016"},
	{100, true, "Windows Server 2019"},
	{100, true, "Windows Server 2022"},
	{100, false, "Windows 10"},
	{110, false, "Windows 11"},
	{110, true, "Windows Server 2025"},
}

// WindowsVersionCount returns the number of known version table entries.
func WindowsVersionCount() int {
	return len(windowsVersions)
}

// WindowsVersionAt returns the entry at index i, or false if i is out
// of range.
func WindowsVersionAt(i int) (WindowsVersion, bool) {
	if i < 0 || i >= len(windowsVersions) {
		return WindowsVersion{}, false
	}
	return windowsVersions[i], true
}

// FindWindowsVersionIndex returns the table index matching number and
// server, or -1 if there is no such entry.
func FindWindowsVersionIndex(number int, server bool) int {
	for i, v := range windowsVersions {
		if v.Number == number && v.Server == server {
			return i
		}
	}
	return -1
}

// FindWindowsVersionName returns the display name for number and
// server, or UnknownOSName if there is no matching entry.
func FindWindowsVersionName(number int, server bool) string {
	if i := FindWindowsVersionIndex(number, server); i >= 0 {
		return windowsVersions[i].Name
	}
	return UnknownOSName
}
