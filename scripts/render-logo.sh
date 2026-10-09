#!/usr/bin/env bash
# Renders the PNG logo set in assets/logo from the SVG sources, using headless Chrome (macOS).
# Usage: scripts/render-logo.sh
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
LOGO_DIR=$ROOT/assets/logo
PNG_DIR=$LOGO_DIR/png
CHROME=${CHROME:-"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"}
WORK_DIR=$(mktemp -d)
trap 'rm -rf "$WORK_DIR"' EXIT
mkdir -p "$PNG_DIR"

# render_svg <svg file> <output png> <width> <height>
render_svg() {
  local svg=$1 output=$2 width=$3 height=$4
  local page=$WORK_DIR/page.html
  cp "$svg" "$WORK_DIR/source.svg"
  cat >"$page" <<EOF
<!doctype html><html><head><style>html,body{margin:0;padding:0;background:transparent;overflow:hidden}img{display:block;width:${width}px;height:${height}px}</style></head>
<body><img src="source.svg"></body></html>
EOF
  rm -f "$output"
  # Headless Chrome can keep running after writing the screenshot, so stop it once the file exists.
  "$CHROME" --headless=new --disable-gpu --hide-scrollbars --force-device-scale-factor=1 \
    --default-background-color=00000000 --user-data-dir="$WORK_DIR/profile" \
    --window-size="$width,$height" --screenshot="$output" "file://$page" >/dev/null 2>&1 &
  local chrome_pid=$!
  for _ in $(seq 1 40); do
    [[ -s "$output" ]] && break
    sleep 0.25
  done
  sleep 0.5
  kill "$chrome_pid" 2>/dev/null || true
  wait "$chrome_pid" 2>/dev/null || true
  local actual
  actual=$(sips -g pixelWidth -g pixelHeight "$output" | awk '/pixel/ {printf "%s ", $2}')
  [[ "$actual" == "$width $height " ]] || { echo "unexpected size for $output: $actual" >&2; exit 1; }
  echo "$(basename "$output") ${width}x${height}"
}

for size in 1024 512 256 128 64 48 32 16; do
  render_svg "$LOGO_DIR/hisar-mark.svg" "$PNG_DIR/hisar-mark-$size.png" "$size" "$size"
done
for width in 1008 504; do
  height=$((width * 48 / 168))
  render_svg "$LOGO_DIR/hisar-wordmark-on-dark.svg" "$PNG_DIR/hisar-wordmark-on-dark-$width.png" "$width" "$height"
  render_svg "$LOGO_DIR/hisar-wordmark-on-light.svg" "$PNG_DIR/hisar-wordmark-on-light-$width.png" "$width" "$height"
done
render_svg "$LOGO_DIR/hisar-app-icon.svg" "$ROOT/desktop/build/appicon.png" 1024 1024
