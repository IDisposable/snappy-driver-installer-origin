#!/usr/bin/env bash
# Builds cmd/sdigo for Windows and runs it directly from WSL, which
# executes it as a real Windows process via PE interop.
#
# Usage: ./scripts/run-windows.sh [args...]
#   args   forwarded to sdigo as-is
#
# Examples:
#   ./scripts/run-windows.sh
#   ./scripts/run-windows.sh -nogui -drp-dir='D:\drivers' -index-dir='D:\indexes'
#   ./scripts/run-windows.sh hwdump
#   ./scripts/run-windows.sh torrenttest -torrent='D:\SDIO_Update.torrent' -data-dir=D:\tmp -list
#
# cmd/sdigo is a full-screen interactive TUI by default (-nogui for a
# plain report): run this script directly in a real WSL terminal (not
# through a non-interactive wrapper/pipe) so it gets a real TTY for
# keyboard input.

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

if ! grep -qi microsoft /proc/version 2>/dev/null; then
	echo "run-windows.sh: this doesn't look like WSL (no 'microsoft' in /proc/version)." >&2
	echo "PE interop is a WSL feature; this script won't work in a plain Linux/CI environment." >&2
	exit 1
fi

BIN="$(mktemp -u /tmp/sdigo-XXXXXX.exe)"
trap 'rm -f "$BIN"' EXIT

echo "Building cmd/sdigo for windows/amd64..." >&2
GOOS=windows GOARCH=amd64 go build -o "$BIN" ./cmd/sdigo

echo "Running on the real Windows host via WSL interop..." >&2
"$BIN" "$@"
