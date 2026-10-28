#!/usr/bin/env bash
# Builds Hisar for Linux with hisar-browser-host and the hisar CLI, and packs
# desktop/bin/Hisar-<version>-linux-<arch>.{deb,rpm,AppImage}.
# Usage: package-desktop-linux.sh <version>     (for the machine's own architecture; the app needs cgo, so no cross builds)
# Needs Linux with GTK 4 and WebKitGTK 6.0 headers (libgtk-4-dev libwebkitgtk-6.0-dev), a C compiler, Go, Node, curl and
# wails3 (make install-wails), which packs the .deb and .rpm (nfpm, build/linux/nfpm/nfpm.yaml). The AppImage is packed
# by appimagetool and the type-2 runtime, downloaded at the versions pinned below and checked against their SHA-256.
set -euo pipefail

scriptDirectory="$(cd "$(dirname "$0")" && pwd)"
GOTOOLCHAIN=$(bash "$scriptDirectory/go-toolchain.sh")
export GOTOOLCHAIN
cd "$scriptDirectory/../desktop"
version=${1:?usage: package-desktop-linux.sh <version>}
architecture=$(go env GOARCH)
test "$(go env GOOS)" = linux || { echo "build the Linux packages on Linux"; exit 1; }

rm -f bin/Hisar bin/hisar-browser-host bin/hisar
(cd frontend && npm ci --no-audit --no-fund && npm run build)
# The flags of wails3 build, plus the version the update notifier compares.
CGO_ENABLED=1 go build -tags production -trimpath -buildvcs=false \
  -ldflags="-w -s -X github.com/tryhisar/hisar/desktop/updater.stampedVersion=$version" -o bin/Hisar .
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/hisar-browser-host ./cmd/hisar-browser-host
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$version" -o bin/hisar ../cli/cmd/hisar
test "$(bin/hisar --version)" = "hisar $version"
# The omp plugin Settings → Coding agents links omp to, in share/hisar/omp-plugin of every package (desktop/harness).
"$scriptDirectory/stage-omp-plugin.sh" bin/omp-plugin

packageDirectory=$(mktemp -d)
trap 'rm -rf "$packageDirectory"' EXIT
for format in deb rpm; do
  VERSION=$version GOARCH=$architecture wails3 tool package -name Hisar -format "$format" -config build/linux/nfpm/nfpm.yaml -out "$packageDirectory"
  packages=("$packageDirectory"/*."$format")
  test "${#packages[@]}" = 1
  output=bin/Hisar-$version-linux-$architecture.$format
  mv "${packages[0]}" "$output"
  echo "$PWD/$output"
done

# The AppImage holds the three binaries and the omp plugin and uses the system's GTK 4 and WebKitGTK 6.0
# (build/linux/appimage/AppRun).
appImageToolVersion=1.9.1
runtimeVersion=20251108
case $architecture in
  amd64)
    appImageArchitecture=x86_64
    appImageToolSHA256=ed4ce84f0d9caff66f50bcca6ff6f35aae54ce8135408b3fa33abfc3cb384eb0
    runtimeSHA256=2fca8b443c92510f1483a883f60061ad09b46b978b2631c807cd873a47ec260d
    ;;
  arm64)
    appImageArchitecture=aarch64
    appImageToolSHA256=f0837e7448a0c1e4e650a93bb3e85802546e60654ef287576f46c71c126a9158
    runtimeSHA256=00cbdfcf917cc6c0ff6d3347d59e0ca1f7f45a6df1a428a0d6d8a78664d87444
    ;;
  *) echo "no AppImage for $architecture"; exit 1 ;;
esac
toolDirectory=build/linux/appimage/build
mkdir -p "$toolDirectory"
download() {
  local url=$1 sha256=$2 path=$3
  if ! echo "$sha256  $path" | sha256sum -c --status 2>/dev/null; then
    curl -fsSL --retry 3 -o "$path.partial" "$url"
    echo "$sha256  $path.partial" | sha256sum -c --quiet
    mv "$path.partial" "$path"
  fi
  chmod +x "$path"
}
appImageTool=$toolDirectory/appimagetool-$appImageToolVersion-$appImageArchitecture.AppImage
runtime=$toolDirectory/runtime-$runtimeVersion-$appImageArchitecture
download "https://github.com/AppImage/appimagetool/releases/download/$appImageToolVersion/appimagetool-$appImageArchitecture.AppImage" "$appImageToolSHA256" "$appImageTool"
download "https://github.com/AppImage/type2-runtime/releases/download/$runtimeVersion/runtime-$appImageArchitecture" "$runtimeSHA256" "$runtime"

appDirectory=$packageDirectory/Hisar.AppDir
mkdir -p "$appDirectory/usr/bin" "$appDirectory/usr/lib/hisar" "$appDirectory/usr/share/hisar"
install -m 0755 build/linux/appimage/AppRun "$appDirectory/AppRun"
install -m 0755 bin/Hisar bin/hisar "$appDirectory/usr/bin/"
install -m 0755 bin/hisar-browser-host "$appDirectory/usr/lib/hisar/"
cp -R bin/omp-plugin "$appDirectory/usr/share/hisar/omp-plugin"
sed 's|^Exec=/usr/bin/Hisar$|Exec=Hisar|' build/linux/com.tryhisar.hisar.desktop > "$appDirectory/com.tryhisar.hisar.desktop"
cp build/linux/appicon-512.png "$appDirectory/com.tryhisar.hisar.png"
ln -s com.tryhisar.hisar.png "$appDirectory/.DirIcon"
output=bin/Hisar-$version-linux-$architecture.AppImage
rm -f "$output"
ARCH=$appImageArchitecture VERSION=$version "$appImageTool" --appimage-extract-and-run --no-appstream --runtime-file "$runtime" \
  "$appDirectory" "$output" >/dev/null
chmod +x "$output"
echo "$PWD/$output"
