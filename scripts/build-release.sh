#!/usr/bin/env bash
# Build cross-platform release archives.
# Usage: scripts/build-release.sh [version] [output-dir]
# Version defaults to `git describe --tags --always --dirty`.
set -euo pipefail
cd "$(dirname "$0")/.."

version="${1:-$(git describe --tags --always --dirty)}"
out="${2:-dist}"
mkdir -p "$out"
outdir="$(cd "$out" && pwd)"
rm -f "$outdir/checksums.txt"

targets=(
  linux/amd64 linux/arm64
  darwin/amd64 darwin/arm64
  windows/amd64 windows/arm64
)

for target in "${targets[@]}"; do
  os="${target%/*}"
  arch="${target#*/}"
  ext=""
  archive="tar.gz"
  if [ "$os" = windows ]; then
    ext=".exe"
    archive="zip"
  fi
  base="opencodable_${version}_${os}_${arch}"
  stage="$(mktemp -d)"
  echo "building ${base}"
  GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 \
    go build -trimpath -ldflags "-s -w -X main.version=${version}" \
    -o "$stage/opencodable${ext}" ./cmd/opencodable
  cp LICENSE README.md "$stage/"
  if [ "$os" = windows ]; then
    (cd "$stage" && zip -q -r "$outdir/${base}.zip" .)
  else
    tar -C "$stage" -czf "$outdir/${base}.tar.gz" .
  fi
  rm -rf "$stage"
done

(cd "$outdir" && sha256sum opencodable_* > checksums.txt)
echo "built ${version} in ${outdir}"
ls -lh "$outdir"
