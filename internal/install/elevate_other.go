//go:build !windows

package install

// IsElevated is unsupported on non-Windows platforms; it always
// reports false, consistent with Driver/CreateRestorePoint also being
// unavailable here.
func IsElevated() bool {
	return false
}

// RelaunchElevated is unsupported on non-Windows platforms.
func RelaunchElevated(args []string) error {
	return errUnsupported
}
