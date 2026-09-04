package usbdrive

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// RequiredBytes sums the size of every regular file under paths
// (recursing into directories), matching the original's "Space
// Required" display before a copy starts.
func RequiredBytes(paths []string) (uint64, error) {
	var total uint64
	for _, p := range paths {
		err := filepath.WalkDir(p, func(_ string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			total += uint64(info.Size())
			return nil
		})
		if err != nil {
			return 0, err
		}
	}
	return total, nil
}

// CopyPortable copies each of paths (files or directories, recursed
// into) into destRoot, preserving each path's own base name at the
// destination's top level - e.g. copying "/data/drivers" into
// "E:\" produces "E:\drivers\...". Ported from USBWizard's file-copy
// step: this only ever creates or overwrites files under destRoot, it
// never deletes anything already there or touches the drive's
// filesystem itself (see the package doc comment for why formatting/
// clearing isn't included). Progress lines are written to out.
func CopyPortable(destRoot string, paths []string, out io.Writer) error {
	for _, src := range paths {
		dest := filepath.Join(destRoot, filepath.Base(src))
		if err := copyRecursive(src, dest, out); err != nil {
			return fmt.Errorf("copying %s: %w", src, err)
		}
	}
	return nil
}

func copyRecursive(src, dest string, out io.Writer) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return copyFile(src, dest, info.Mode(), out)
	}

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := copyRecursive(filepath.Join(src, e.Name()), filepath.Join(dest, e.Name()), out); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dest string, mode os.FileMode, out io.Writer) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, in); err != nil {
		return err
	}
	fmt.Fprintf(out, "copied %s\n", dest)
	return nil
}
