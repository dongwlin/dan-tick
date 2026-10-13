#!/usr/bin/env bash
# Runs inside the pinned packaging image: compile the desktop application against
# the packaged GTK 4 / WebKitGTK 6.0 stack and assemble the AppImage. Called by
# package.sh; not meant to be executed directly on a host.
set -euo pipefail

root_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../../.." && pwd)
cd "$root_dir"

: "${DAN_TICK_WEBKIT_RUNTIME:?DAN_TICK_WEBKIT_RUNTIME must point at the extracted WebKit runtime}"

# Builds the frontend, generates bindings and compiles the Go binary with CGO
# against the libraries installed in this image.
wails3 task linux:build ARCH=amd64

exec bash build/linux/appimage/build.sh \
  "$root_dir/bin/dan-tick" \
  "$root_dir/build/appicon.png" \
  "$root_dir/build/linux/dan-tick.desktop" \
  "$root_dir/build/linux/appimage/build" \
  "$root_dir/bin"
