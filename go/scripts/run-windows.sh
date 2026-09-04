#!/usr/bin/env bash
# Builds one of this rewrite's commands for Windows and runs it
# directly from WSL, which executes it as a real Windows process via
# PE interop - the same technique test-windows.sh uses for cmd/hwdump,
# generalized to run the actual application (or any cmd/* target)
# with real arguments, not just dump diagnostics.
#
# Usage: go/scripts/run-windows.sh [target] [-- ] [args...]
#   target   one of: sdi (default), sditui, hwdump, torrenttest
#   args     forwarded to the built executable as-is
#
# Examples:
#   go/scripts/run-windows.sh
#   go/scripts/run-windows.sh sdi -drp-dir='D:\drivers' -index-dir='D:\indexes'
#   go/scripts/run-windows.sh sditui -torrent-file='D:\SDIO_Update.torrent'
#
# cmd/sditui is a full-screen interactive TUI: run this script directly
# in a real WSL terminal (not through a non-interactive wrapper/pipe)
# so it gets a real TTY for keyboard input.

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

if ! grep -qi microsoft /proc/version 2>/dev/null; then
	echo "run-windows.sh: this doesn't look like WSL (no 'microsoft' in /proc/version)." >&2
	echo "PE interop is a WSL feature; this script won't work in a plain Linux/CI environment." >&2
	exit 1
fi

target="sdi"
if [[ $# -gt 0 && $1 != -* ]]; then
	target="$1"
	shift
fi
if [[ $# -gt 0 && $1 == "--" ]]; then
	shift
fi

case "$target" in
sdi | sditui | hwdump | torrenttest) ;;
*)
	echo "run-windows.sh: unknown target '$target' (want sdi, sditui, hwdump, or torrenttest)" >&2
	exit 1
	;;
esac

BIN="$(mktemp -u "/tmp/${target}-XXXXXX.exe")"
trap 'rm -f "$BIN"' EXIT

echo "Building cmd/$target for windows/amd64..." >&2
GOOS=windows GOARCH=amd64 go build -o "$BIN" "./cmd/$target"

echo "Running on the real Windows host via WSL interop..." >&2
"$BIN" "$@"
