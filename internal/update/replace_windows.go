//go:build windows

package update

import "golang.org/x/sys/windows"

// replaceFile uses MoveFileEx directly because os.Rename already replaces
// the destination, but it does not expose MOVEFILE_WRITE_THROUGH. The flag
// asks Windows to flush the replacement before this function returns.
func replaceFile(src, dest string) error {
	srcPtr, err := windows.UTF16PtrFromString(src)
	if err != nil {
		return err
	}
	destPtr, err := windows.UTF16PtrFromString(dest)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(srcPtr, destPtr, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
