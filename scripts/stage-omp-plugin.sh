#!/usr/bin/env bash
# Copies the omp plugin (omp/) that the desktop app ships into <destination>: Hisar.app's Contents/Resources/omp-plugin,
# and share/hisar/omp-plugin in the Linux packages. Settings → Coding agents links omp to that folder. The plugin
# imports only types from omp, so it needs no node_modules; omp starts mcp.json's server with bin/run-hisar, which
# finds the hisar packaged beside the plugin.
# Usage: stage-omp-plugin.sh <destination>
set -euo pipefail

source=$(cd "$(dirname "$0")/../omp" && pwd)
destination=${1:?usage: stage-omp-plugin.sh <destination>}
rm -rf "$destination"
mkdir -p "$destination"
cp -R "$source/package.json" "$source/mcp.json" "$source/bin" "$source/src" "$source/skills" "$destination/"
test -f "$destination/package.json"
test -f "$destination/mcp.json"
test -x "$destination/bin/run-hisar"
