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

targets=(
  linux/amd64 linux/arm64
  darwin/amd64 darwin/arm64
  windows/amd64 windows/arm64
)

built=()
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
    archive="${base}.zip"
    (cd "$stage" && zip -q -r "$outdir/$archive" .)
  else
    archive="${base}.tar.gz"
    tar -C "$stage" -czf "$outdir/$archive" .
  fi
  rm -rf "$stage"
  built+=("$archive")
done

# Checksum exactly what this run built, never stale archives left in dist/.
(cd "$outdir" && sha256sum "${built[@]}" > checksums.txt)
echo "built ${version} in ${outdir}"
ls -lh "$outdir"
