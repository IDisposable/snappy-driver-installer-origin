//go:build !windows

package update

import "os"

func replaceFile(src, dest string) error {
	return os.Rename(src, dest)
}
