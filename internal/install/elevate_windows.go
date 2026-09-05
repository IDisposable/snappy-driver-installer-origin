//go:build windows

package install

import (
	"os"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// IsElevated reports whether the current process holds an elevated
// (administrator) token. sdigo's manifest requests no elevation at
// launch (unlike installing a driver, browsing scan results needs no
// admin rights), so this is checked on demand right before an action
// that does need it.
func IsElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// RelaunchElevated starts a new copy of the running executable with a
// UAC elevation prompt, passing args as its command line, and returns
// once Windows has accepted the request to launch it (not once the
// new process exits - the two processes are independent from that
// point on). The caller is expected to exit shortly after this
// returns nil, handing off to the elevated copy.
func RelaunchElevated(args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exePtr, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	verbPtr, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	argPtr, err := windows.UTF16PtrFromString(quoteArgs(args))
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verbPtr, exePtr, argPtr, nil, windows.SW_SHOWNORMAL)
}

// quoteArgs joins args into a Windows command line, quoting any
// argument that needs it so ShellExecute's single lpParameters string
// splits back into the same argv the elevated process's flag parser
// expects.
func quoteArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = syscall.EscapeArg(a)
	}
	return strings.Join(quoted, " ")
}
