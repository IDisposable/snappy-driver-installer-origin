#!/usr/bin/env bash
# Builds cmd/hwdump for Windows and runs it directly from WSL, which
# executes it as a real Windows process via PE interop - giving actual
# WMI/SetupAPI/registry answers from the host machine, not a mock.
#
# Usage: go/scripts/test-windows.sh
# Output: JSON on stdout from hwdump; compare fields by hand (or pipe
# to jq) against tools/*/logs/*_log.txt on a real installation, or
# against `powershell.exe Get-CimInstance Win32_BaseBoard` etc. as an
# independent cross-check.

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

if ! grep -qi microsoft /proc/version 2>/dev/null; then
	echo "test-windows.sh: this doesn't look like WSL (no 'microsoft' in /proc/version)." >&2
	echo "PE interop is a WSL feature; this script won't work in a plain Linux/CI environment." >&2
	exit 1
fi

BIN="$(mktemp -u /tmp/hwdump-XXXXXX.exe)"
trap 'rm -f "$BIN"' EXIT

echo "Building cmd/hwdump for windows/amd64..." >&2
GOOS=windows GOARCH=amd64 go build -o "$BIN" ./cmd/hwdump

echo "Running on the real Windows host via WSL interop..." >&2
"$BIN"
