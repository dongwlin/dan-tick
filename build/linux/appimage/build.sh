#!/usr/bin/env bash
# Package the application with the distribution's GTK4/WebKitGTK runtime.
set -euo pipefail

if [[ $# != 5 ]]; then
  echo "Usage: $0 <binary> <icon> <desktop file> <build directory> <output directory>" >&2
  exit 2
fi
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
binary=$(realpath "$1")
icon=$(realpath "$2")
desktop=$(realpath "$3")
build_dir=$(realpath -m "$4")
output_dir=$(realpath -m "$5")

# Product support is linux/amd64. Packaging must use the matching host tools.
[[ $(uname -m) == x86_64 ]] || { echo 'AppImage packaging requires an x86_64 Linux host.' >&2; exit 1; }
LC_ALL=C readelf -h "$binary" | grep -q 'Machine:.*Advanced Micro Devices X86-64' || {
  echo 'The application binary must be an x86_64 ELF.' >&2
  exit 1
}

app_dir="$build_dir/dan-tick-x86_64.AppDir"
mkdir -p "$build_dir" "$output_dir"
# Always create a fresh AppDir: a previous linuxdeploy build may contain GUI
# libraries and Ubuntu-specific WebKit helpers that shadow the host runtime.
rm -rf -- "$app_dir"
install -Dm755 "$binary" "$app_dir/usr/bin/dan-tick"
install -m755 "$script_dir/AppRun" "$app_dir/AppRun"
install -m644 "$icon" "$app_dir/dan-tick.png"
install -m644 "$desktop" "$app_dir/dan-tick.desktop"
ln -s dan-tick.png "$app_dir/.DirIcon"

# Fixed upstream releases, verified even when reusing the local tool cache.
fetch_tool() {
  local target=$1 url=$2 digest=$3
  if [[ ! -f $target ]]; then
    curl --fail --location --retry 3 --connect-timeout 20 "$url" --output "$target.download"
    mv -- "$target.download" "$target"
  fi
  printf '%s  %s\n' "$digest" "$target" | sha256sum --check --status || {
    echo "AppImage tool checksum mismatch: $target" >&2
    exit 1
  }
}
tool="$build_dir/appimagetool-1.9.1-x86_64.AppImage"
runtime="$build_dir/runtime-20251108-x86_64"
fetch_tool "$tool" \
  https://github.com/AppImage/appimagetool/releases/download/1.9.1/appimagetool-x86_64.AppImage \
  ed4ce84f0d9caff66f50bcca6ff6f35aae54ce8135408b3fa33abfc3cb384eb0
fetch_tool "$runtime" \
  https://github.com/AppImage/type2-runtime/releases/download/20251108/runtime-x86_64 \
  2fca8b443c92510f1483a883f60061ad09b46b978b2631c807cd873a47ec260d
chmod +x "$tool"

# The static type-2 runtime does not require libfuse2. Extract-and-run also
# allows packaging in containers without a /dev/fuse device.
ARCH=x86_64 "$tool" --appimage-extract-and-run --runtime-file "$runtime" \
  "$app_dir" "$output_dir/dan-tick-x86_64.AppImage"
chmod +x "$output_dir/dan-tick-x86_64.AppImage"
