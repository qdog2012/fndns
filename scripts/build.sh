#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="${VERSION:-1.0.9}"
PLATFORM="${PLATFORM:-all}"

command -v go >/dev/null || { echo "需要 Go 1.24+" >&2; exit 1; }
command -v npm >/dev/null || { echo "需要 Node.js 20+ 与 npm" >&2; exit 1; }
command -v python3 >/dev/null || { echo "需要 Python 3" >&2; exit 1; }

cd "$ROOT/webui"
npm ci
npm run build
cd "$ROOT"
python3 scripts/generate_icons.py
go test ./...
mkdir -p bin dist

build_target() {
  local platform="$1"
  local goarch="$2"
  local binary="bin/fndns-linux-${goarch}"
  CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" -o "$binary" ./cmd/fndns
  python3 scripts/package_fpk.py --root "$ROOT" --binary "$ROOT/$binary" \
    --version "$VERSION" --platform "$platform" \
    --output "$ROOT/dist/com.fndns.manager_${VERSION}_${platform}.fpk"
}

if [[ "$PLATFORM" == "all" || "$PLATFORM" == "x86" ]]; then
  build_target x86 amd64
fi
if [[ "$PLATFORM" == "all" || "$PLATFORM" == "arm" ]]; then
  build_target arm arm64
fi

echo "构建完成：$ROOT/dist"
