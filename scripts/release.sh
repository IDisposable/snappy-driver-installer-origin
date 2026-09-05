#!/usr/bin/env bash
# Builds the release sdigo.exe: runs the test suite, generates the
# Windows app manifest/version resource (go-winres - see
# docs/RELEASE.md for why this isn't committed), then cross-compiles.
# Used by both a local release build and .github/workflows/release.yml,
# so there is exactly one place that defines what a release build does.
#
# Usage: ./scripts/release.sh [version]
#   version   product/file version go-winres embeds (e.g. 0.2.0).
#             Defaults to "git-tag", which go-winres resolves from the
#             nearest git tag - fine in CI (checked out at the release
#             tag) or a local checkout that already has one.

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

version="${1:-git-tag}"

echo "Running tests..." >&2
go test ./...

winres="$(command -v go-winres || true)"
if [ -z "$winres" ]; then
	gobin="$(go env GOPATH)/bin"
	winres="$gobin/go-winres"
	if [ ! -x "$winres" ]; then
		echo "Installing go-winres..." >&2
		go install github.com/tc-hib/go-winres@latest
	fi
fi

echo "Generating Windows app manifest/version resource..." >&2
(cd cmd/sdigo && "$winres" make --arch amd64 --out rsrc --product-version "$version" --file-version "$version")

echo "Building sdigo.exe for windows/amd64..." >&2
GOOS=windows GOARCH=amd64 go build -o sdigo.exe ./cmd/sdigo

echo "Built sdigo.exe" >&2
