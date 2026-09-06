//go:build !windows

package install

func Reboot() error {
	return errUnsupported
}
