package main

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// splashDuration is how long screenSplash stays up before yielding to
// screenScanning on its own - purely cosmetic, so kept short. A user
// in a hurry can also dismiss it early with any keypress; a scan that
// finishes before either happens skips it entirely (scanDoneMsg sets
// m.screen directly, regardless of what's currently showing).
const splashDuration = 1200 * time.Millisecond

// splashDoneMsg fires once splashDuration elapses.
type splashDoneMsg struct{}

func tickSplashCmd() tea.Cmd {
	return tea.Tick(splashDuration, func(t time.Time) tea.Msg {
		return splashDoneMsg{}
	})
}

// splashBanner is the ASCII-art title screenSplash shows for
// splashDuration on startup - "GO FORTH" in block letters, this
// rewrite's own name (see the About screen and README.md), not a
// reproduction of the original VCL app's own splash/logo. Every line
// uses fixed-width source lines by construction, so the block-letter
// banner stays aligned in a monospace terminal.
const splashBanner = ` ██████╗  ██████╗      ███████╗ ██████╗ ██████╗ ████████╗██╗  ██╗
██╔════╝ ██╔═══██╗     ██╔════╝██╔═══██╗██╔══██╗╚══██╔══╝██║  ██║
██║  ███╗██║   ██║     █████╗  ██║   ██║██████╔╝   ██║   ███████║
██║   ██║██║   ██║     ██╔══╝  ██║   ██║██╔══██╗   ██║   ██╔══██║
╚██████╔╝╚██████╔╝     ██║     ╚██████╔╝██║  ██║   ██║   ██║  ██║
 ╚═════╝  ╚═════╝      ╚═╝      ╚═════╝ ╚═╝  ╚═╝   ╚═╝   ╚═╝  ╚═╝`

// splashStyle centers splashBanner and its subtitle in the terminal
// and gives the banner itself a bit of color - the only screen in the
// TUI that's purely decorative, so it's the one place a splash of
// color doesn't fight with the table/status styling used everywhere
// else.
var splashStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)

// splashView renders screenSplash: splashBanner plus a subtitle,
// centered in the terminal - see splashDuration/tickSplashCmd for how
// long it stays up.
func (m model) splashView() string {
	body := splashStyle.Render(splashBanner) + "\n\n" +
		"Snappy Driver Installer - reimplemented in Go\n\n" +
		"press any key to skip..."
	if m.width <= 0 || m.height <= 0 {
		return body
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, body)
}
