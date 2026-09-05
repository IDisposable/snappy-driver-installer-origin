package common

import "fmt"

const (
	kb = 1024.0
	mb = kb * 1024.0
	gb = mb * 1024.0
	tb = gb * 1024.0
)

// BytesToStr formats a byte count as a human-readable binary size string
// (e.g. "1.5 GB").
func BytesToStr(bytes uint64) string {
	f := float64(bytes)
	switch {
	case f > tb:
		return fmt.Sprintf("%.1f TB", f/tb)
	case f > gb:
		return fmt.Sprintf("%.1f GB", f/gb)
	case f > mb:
		return fmt.Sprintf("%.1f MB", f/mb)
	case f > kb:
		return fmt.Sprintf("%.1f KB", f/kb)
	default:
		return fmt.Sprintf("%.0f B", f)
	}
}
