#!/usr/bin/env bash
# Assemble the self-contained AppImage.
#
# This script runs inside the pinned packaging image (build/linux/runtime/Dockerfile,
# target "packaging"), which provides one matching GTK 4 / WebKitGTK 6.0 stack and
# the patched WebKit runtime. Everything the application needs at run time is
# collected from that environment into the AppDir; only glibc, the C++ runtime and
# the GPU drivers stay on the host.
#
# Usage: build.sh <binary> <icon> <desktop file> <build directory> <output directory>
#
# Environment:
#   DAN_TICK_WEBKIT_RUNTIME  extracted runtime tree (required), holds usr/lib/...
#   DAN_TICK_VERSION         display version recorded in the packaged runtime info
#   DAN_TICK_BUILD_COMMIT    commit recorded in the packaged runtime info
set -euo pipefail

usage() {
  echo "Usage: $0 <binary> <icon> <desktop file> <build directory> <output directory>" >&2
  exit 2
}

log() { printf '==> %s\n' "$*" >&2; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

[[ $# == 5 ]] || usage

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
binary=$(realpath "$1")
icon=$(realpath "$2")
desktop=$(realpath "$3")
build_dir=$(realpath -m "$4")
output_dir=$(realpath -m "$5")

runtime=${DAN_TICK_WEBKIT_RUNTIME:-}
[[ -n $runtime ]] || die 'DAN_TICK_WEBKIT_RUNTIME must point at the extracted WebKit runtime.'
runtime=$(realpath "$runtime")
[[ -d $runtime/usr/lib ]] || die "the WebKit runtime at $runtime has no usr/lib directory."

# Collecting libraries only makes sense in the environment they were built in.
# The packaging image writes its runtime id to this marker file.
[[ -f /etc/dan-tick-packaging-image ]] || [[ ${DAN_TICK_ALLOW_UNPINNED_HOST:-} == 1 ]] \
  || die 'run this script inside the pinned packaging image (see build/linux/runtime/Dockerfile), or set DAN_TICK_ALLOW_UNPINNED_HOST=1 for a deliberate local experiment.'

multiarch=usr/lib/x86_64-linux-gnu
app_name=dan-tick
app_dir="$build_dir/$app_name-x86_64.AppDir"
lib_dir="$app_dir/$multiarch"
runtime_dir="$lib_dir/webkitgtk-6.0"

# Libraries that must stay on the host. The C library and the dynamic loader have
# to match the running kernel and the host's NSS modules; libstdc++ and libgcc
# are provided by every supported distribution, and bundling the C++ runtime next
# to host graphics drivers is a well known source of breakage. Everything else,
# including libGL/libEGL/libgbm/libdrm/libvulkan and the TLS stack, is bundled so
# that no GTK, WebKit or GIO package is required on the target system.
excluded_libraries=(
  ld-linux-x86-64.so.2
  libanl.so.1 libBrokenLocale.so.1 libc.so.6 libdl.so.2 libm.so.6 libnsl.so.1
  libpthread.so.0 libresolv.so.2 librt.so.1 libthread_db.so.1 libutil.so.1
  libgcc_s.so.1 libstdc++.so.6
)

is_excluded() {
  local name=$1 excluded
  for excluded in "${excluded_libraries[@]}"; do
    [[ $name == "$excluded" ]] && return 0
  done
  return 1
}

is_elf() {
  [[ -f $1 && ! -L $1 ]] && [[ $(head -c 4 -- "$1" | od --address-radix=n --format=x1 | tr -d ' \n') == 7f454c46 ]]
}

# Print "<name>\t<absolute path>" for every shared library an ELF file needs.
# Resolving with the bundled library directory on the search path makes already
# collected libraries win over the copies still installed in the image.
dependencies_of() {
  local elf=$1 line name path
  while IFS= read -r line; do
    case $line in
      *'=> not found'*)
        name=${line%%=>*}
        printf '%s\t%s\n' "$(printf '%s' "$name" | tr -d '\t')" 'MISSING'
        ;;
      *'=> '*)
        name=${line%%=>*}
        path=${line#*=> }
        path=${path%% (*}
        printf '%s\t%s\n' "$(printf '%s' "$name" | tr -d '\t')" "$path"
        ;;
    esac
  done < <(LD_LIBRARY_PATH="$lib_dir" ldd "$elf" 2>&1 || true)
}

soname_of() {
  local soname
  soname=$(patchelf --print-soname "$1" 2>/dev/null || true)
  printf '%s' "${soname:-$(basename -- "$1")}"
}

# ---------------------------------------------------------------------------
# AppDir skeleton: application, WebKit runtime, sandbox helpers, resources
# ---------------------------------------------------------------------------
log "preparing $app_dir"
rm -rf -- "$app_dir"
mkdir -p "$app_dir/usr/bin" "$lib_dir"

install -m0755 "$binary" "$app_dir/usr/bin/$app_name"
install -m0755 "$script_dir/AppRun" "$app_dir/AppRun"
install -m0644 "$icon" "$app_dir/$app_name.png"
install -m0644 "$desktop" "$app_dir/$app_name.desktop"
ln -s "$app_name.png" "$app_dir/.DirIcon"

log "adding the patched WebKit runtime"
cp --archive "$runtime/usr/." "$app_dir/usr/"
[[ -d $runtime_dir ]] || die "the WebKit runtime has no $multiarch/webkitgtk-6.0 directory."
[[ -f $runtime_dir/injected-bundle/libwebkitgtkinjectedbundle.so ]] \
  || die 'the WebKit runtime has no injected bundle.'
# The auxiliary processes are executed by the WebKit library, so their execute
# permission is part of the contract (the generator this replaces shipped 0644).
chmod 0755 "$runtime_dir"/WebKit*Process

log "adding the sandbox helpers"
install -m0755 "$(command -v bwrap)" "$runtime_dir/bwrap"
install -m0755 "$(command -v xdg-dbus-proxy)" "$runtime_dir/xdg-dbus-proxy"

log "adding GIO modules, image loaders and GLib schemas"
mkdir -p "$lib_dir/gio/modules" "$lib_dir/gdk-pixbuf-2.0/2.10.0/loaders" "$app_dir/usr/share/glib-2.0/schemas"
cp --archive /usr/lib/x86_64-linux-gnu/gio/modules/. "$lib_dir/gio/modules/"
cp --archive /usr/lib/x86_64-linux-gnu/gdk-pixbuf-2.0/2.10.0/loaders/. "$lib_dir/gdk-pixbuf-2.0/2.10.0/loaders/"
rm -f -- "$lib_dir/gio/modules/giomodule.cache" \
  "$lib_dir/gdk-pixbuf-2.0/2.10.0/loaders.cache"
cp --archive /usr/share/glib-2.0/schemas/gschemas.compiled "$app_dir/usr/share/glib-2.0/schemas/"
if [[ -d /usr/share/icons ]]; then
  # Theme index and the Adwaita icon theme GTK 4 falls back to; host themes stay
  # usable because XDG_DATA_DIRS keeps the system directories.
  mkdir -p "$app_dir/usr/share/icons/hicolor"
  cp --archive /usr/share/icons/hicolor/index.theme "$app_dir/usr/share/icons/hicolor/" 2>/dev/null || true
  [[ -d /usr/share/icons/Adwaita ]] && cp --archive /usr/share/icons/Adwaita "$app_dir/usr/share/icons/"
fi

# ---------------------------------------------------------------------------
# Dependency closure
# ---------------------------------------------------------------------------
declare -A processed=() collected=()
queue=()
enqueue() { queue+=("$1"); }

enqueue "$app_dir/usr/bin/$app_name"
while IFS= read -r -d '' elf; do enqueue "$elf"; done \
  < <(find "$lib_dir" "$app_dir/usr/bin" -type f -print0)

log "resolving the dependency closure"
while ((${#queue[@]})); do
  elf=${queue[0]}
  queue=("${queue[@]:1}")
  [[ -z ${processed[$elf]:-} ]] || continue
  processed[$elf]=1
  is_elf "$elf" || continue

  while IFS=$'\t' read -r name path; do
    [[ -n $name ]] || continue
    if [[ $path == MISSING ]]; then
      die "$name is missing while resolving $elf: the packaging image is incomplete."
    fi
    if [[ $path == "$app_dir"/* ]]; then
      [[ -n ${processed[$path]:-} ]] || enqueue "$path"
      continue
    fi
    is_excluded "$name" && continue
    [[ -f $path ]] || die "$name resolved to $path, which is not a file."

    soname=$(soname_of "$path")
    target="$lib_dir/$soname"
    if [[ ! -f $target ]]; then
      install -m0644 "$path" "$target"
    fi
    collected[$soname]=$path
    enqueue "$target"
  done < <(dependencies_of "$elf")
done

log "bundled $(find "$lib_dir" -maxdepth 1 -type f | wc -l) shared libraries"

# ---------------------------------------------------------------------------
# Relative load paths, caches and permissions
# ---------------------------------------------------------------------------
log "rewriting load paths relative to \$ORIGIN"
while IFS= read -r -d '' elf; do
  is_elf "$elf" || continue
  relative=$(realpath --relative-to="$(dirname -- "$elf")" "$lib_dir")
  if [[ $relative == . ]]; then
    patchelf --set-rpath '$ORIGIN' "$elf"
  else
    patchelf --set-rpath "\$ORIGIN/$relative" "$elf"
  fi
done < <(find "$app_dir" -type f -print0)

log "generating loader caches"
# gdk-pixbuf only accepts absolute loader paths, so the cache keeps a placeholder
# that AppRun substitutes with the real mount point of the AppImage.
loaders_cache="$lib_dir/gdk-pixbuf-2.0/2.10.0/loaders.cache"
sed -E "s|^\"[^\"]*/(libpixbufloader-[^\"/]+\.so)\"$|\"@APPDIR@/$multiarch/gdk-pixbuf-2.0/2.10.0/loaders/\1\"|" \
  /usr/lib/x86_64-linux-gnu/gdk-pixbuf-2.0/2.10.0/loaders.cache >"$loaders_cache"
grep -q '@APPDIR@' "$loaders_cache" || die 'the image loader cache could not be relocated.'
gio-querymodules "$lib_dir/gio/modules"
chmod 0644 "$loaders_cache" "$lib_dir/gio/modules/giomodule.cache"

# ---------------------------------------------------------------------------
# Package level verification
# ---------------------------------------------------------------------------
log "verifying the packaged runtime"
verify_elf=$app_dir/usr/bin/$app_name
for candidate in "$verify_elf" "$runtime_dir"/WebKit*Process \
  "$runtime_dir/injected-bundle/libwebkitgtkinjectedbundle.so" \
  "$runtime_dir/bwrap" "$runtime_dir/xdg-dbus-proxy"; do
  [[ -f $candidate ]] || die "$candidate is missing from the package."
done
# The auxiliary processes and the sandbox helpers are executed, so they must keep
# their execute permission; the injected bundle is loaded as a library.
for candidate in "$verify_elf" "$runtime_dir"/WebKit*Process \
  "$runtime_dir/bwrap" "$runtime_dir/xdg-dbus-proxy"; do
  [[ -x $candidate ]] || die "$candidate is not executable."
done
for candidate in "$verify_elf" "$runtime_dir"/WebKit*Process \
  "$runtime_dir/injected-bundle/libwebkitgtkinjectedbundle.so" \
  "$runtime_dir/bwrap" "$runtime_dir/xdg-dbus-proxy"; do
  if LD_LIBRARY_PATH="$lib_dir" ldd "$candidate" 2>&1 | grep -q 'not found'; then
    LD_LIBRARY_PATH="$lib_dir" ldd "$candidate" >&2 || true
    die "$candidate has unresolved shared libraries."
  fi
done

# The application must load GTK, WebKit and JavaScriptCore from the package, not
# from the machine that built it.
for requirement in libgtk-4.so.1 libwebkitgtk-6.0.so.4 libjavascriptcoregtk-6.0.so.1; do
  resolved=$(LD_LIBRARY_PATH="$lib_dir" ldd "$verify_elf" | awk -v name="$requirement" '$1 == name { print $3 }')
  [[ $resolved == "$app_dir"/* ]] || die "$requirement resolves to '$resolved' instead of the packaged copy."
  printf '    %-30s %s\n' "$requirement" "${resolved#"$app_dir"/}" >&2
done

if find "$app_dir" -type f -print0 | xargs -0 -r patchelf --print-rpath 2>/dev/null | grep -qE '(^|:)/'; then
  die 'at least one file still has an absolute load path.'
fi

log "recording the packaged runtime manifest"
manifest="$app_dir/usr/share/$app_name"
mkdir -p "$manifest"
{
  printf 'runtime-id: %s\n' "$(cat /etc/dan-tick-packaging-image)"
  printf 'version: %s\n' "${DAN_TICK_VERSION:-dev}"
  printf 'commit: %s\n' "${DAN_TICK_BUILD_COMMIT:-unknown}"
  printf 'built-in: %s\n\n' "$(sed -n 's/^PRETTY_NAME="\(.*\)"$/\1/p' /etc/os-release)"
  printf '# sha256  size  mode  path\n'
  (cd "$app_dir" && find . -type f -print0 | sort -z | xargs -0 sha256sum | sed 's| \./| |') \
    | while read -r sum path; do
        printf '%s  %s  %s  %s\n' "$sum" "$(stat -c %s "$app_dir/$path")" "$(stat -c %a "$app_dir/$path")" "$path"
      done
} >"$manifest/runtime-manifest.txt"

log "packaging the AppImage"
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
mkdir -p "$build_dir" "$output_dir"
tool="$build_dir/appimagetool-1.9.1-x86_64.AppImage"
appimage_runtime="$build_dir/runtime-20251108-x86_64"
fetch_tool "$tool" \
  https://github.com/AppImage/appimagetool/releases/download/1.9.1/appimagetool-x86_64.AppImage \
  ed4ce84f0d9caff66f50bcca6ff6f35aae54ce8135408b3fa33abfc3cb384eb0
fetch_tool "$appimage_runtime" \
  https://github.com/AppImage/type2-runtime/releases/download/20251108/runtime-x86_64 \
  2fca8b443c92510f1483a883f60061ad09b46b978b2631c807cd873a47ec260d
chmod +x "$tool"

ARCH=x86_64 "$tool" --appimage-extract-and-run --runtime-file "$appimage_runtime" \
  "$app_dir" "$output_dir/$app_name-x86_64.AppImage"
chmod +x "$output_dir/$app_name-x86_64.AppImage"

log "built $output_dir/$app_name-x86_64.AppImage ($(du -h "$output_dir/$app_name-x86_64.AppImage" | cut -f1))"
