// Package install performs the actual driver installation, ported
// from the non-GUI parts of install.cpp/system.cpp's restore-point
// helpers. Two significant pieces of the original are deliberately not
// ported:
//
//   - The "install64.exe" bundled-helper/WOW64 workaround: the
//     original shipped a 32-bit main executable (for old-Windows
//     compatibility) that could not call UpdateDriverForPlugAndPlay
//     Devices correctly for 64-bit driver packages, so it extracted
//     and ran a bundled 64-bit helper binary for that case. This
//     rewrite builds a native amd64 binary, so the problem doesn't
//     exist.
//   - The "Autoclicker": a background thread that simulated mouse
//     clicks to dismiss a driver-signing dialog that could steal
//     focus during the 32-bit install path. Not needed by the native
//     64-bit API call either.
package install

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Result is the outcome of a driver-install attempt.
type Result struct {
	Installed   bool
	NeedsReboot bool
}

// RestorePointDescription is the fixed description SDIO gives the
// restore point it creates before installing drivers, ported from the
// literal string in Manager::thread_install.
const RestorePointDescription = "Installed drivers"

// RemoveExtraInfs deletes every .inf file (any case) in infPath's
// directory except the one infPath itself names, ported from
// removeextrainfs in install.cpp - cleanup after extracting a driver
// package that shipped more than one .inf variant. The original used
// a "*.inF" FindFirstFile pattern; on Windows' case-insensitive
// filesystem that matches every case variant of ".inf", which this
// replicates via an explicit case-insensitive extension check rather
// than a glob pattern (Go's filepath.Glob matches case-sensitively
// regardless of OS).
func RemoveExtraInfs(infPath string) error {
	dir := filepath.Dir(infPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", dir, err)
	}

	lowerInfPath := strings.ToLower(infPath)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.EqualFold(filepath.Ext(name), ".inf") {
			continue
		}
		if strings.Contains(lowerInfPath, strings.ToLower(name)) {
			continue // this is (or matches) the inf actually being installed
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return fmt.Errorf("removing %s: %w", name, err)
		}
	}
	return nil
}
