package main

import (
	"github.com/charmbracelet/lipgloss"
)

// betterStyle/invalidStyle render the detail screen's "greenlight"
// comparison uses betterStyle for the winning side.
// COLOR text colors - the original highlights whichever side of a
// per-field comparison wins in green, and flags a bad signature or
// OS/arch mismatch in red. cautionStyle has no original equivalent -
// it flags the Microsoft-inbox-driver note below.
var (
	betterStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2"))
	invalidStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1"))
	cautionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3"))

	// cautionStyleOverhead is how many extra bytes cautionStyle.Render
	// adds around its content (the ANSI escape codes) - 0 outside a
	// real terminal, where lipgloss renders plain text unstyled.
	// deviceRow needs this to keep a styled cell's raw length within
	// its column's width - see that function's doc comment.
	cautionStyleOverhead = len(cautionStyle.Render(""))
)
