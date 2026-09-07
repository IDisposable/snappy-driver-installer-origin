// Package install performs driver installation and restore-point operations
// through native Windows APIs.
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

// RestorePointDescription is the user-facing restore-point description.
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

	normalizedInfPath, err := filepath.Abs(infPath)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", infPath, err)
	}
	normalizedInfPath = filepath.Clean(normalizedInfPath)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.EqualFold(filepath.Ext(name), ".inf") {
			continue
		}
		candidate, err := filepath.Abs(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("resolving %s: %w", name, err)
		}
		if strings.EqualFold(filepath.Clean(candidate), normalizedInfPath) {
			continue // this is (or matches) the inf actually being installed
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return fmt.Errorf("removing %s: %w", name, err)
		}
	}
	return nil
}
