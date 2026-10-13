#!/usr/bin/env bash
# Launcher behavior without a display, network, or actual account data.
#
# The packaged AppRun must start the application with the resources inside the
# AppImage, keep the WebKit sandbox enabled, and fail with a clear message when
# the package or the host environment is incomplete.
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
fixture=$(mktemp -d /tmp/dan-tick-apprun-test.XXXXXX)
trap 'rm -rf -- "$fixture"' EXIT

lib_dir_rel=usr/lib/x86_64-linux-gnu
app_dir="$fixture/path with spaces.AppDir"
app_lib="$app_dir/$lib_dir_rel"
mkdir -p "$app_dir/usr/bin" "$app_lib/webkitgtk-6.0/injected-bundle" \
  "$app_lib/gdk-pixbuf-2.0/2.10.0" "$app_lib/gio/modules" \
  "$app_dir/usr/share/glib-2.0/schemas" "$fixture/tools" "$fixture/unrelated" "$fixture/cache"
install -m755 "$script_dir/AppRun" "$app_dir/AppRun"

cat >"$app_dir/usr/bin/dan-tick" <<'BINARY'
#!/usr/bin/env bash
{
  printf '%s\n' "$PWD"
  printf 'GIO_EXTRA_MODULES=%s\n' "${GIO_EXTRA_MODULES:-}"
  printf 'GDK_PIXBUF_MODULE_FILE=%s\n' "${GDK_PIXBUF_MODULE_FILE:-}"
  printf 'XDG_DATA_DIRS=%s\n' "${XDG_DATA_DIRS:-}"
  printf 'LD_LIBRARY_PATH=%s\n' "${LD_LIBRARY_PATH:-}"
  printf 'WEBKIT_EXEC_PATH=%s\n' "${WEBKIT_EXEC_PATH:-}"
  printf 'GDK_BACKEND=%s\n' "${GDK_BACKEND:-}"
  printf 'WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=%s\n' "${WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS:-}"
  printf 'arguments=%s\n' "$*"
} >"$LAUNCH_RESULT"
BINARY
chmod +x "$app_dir/usr/bin/dan-tick"

for helper in WebKitWebProcess WebKitNetworkProcess WebKitGPUProcess bwrap xdg-dbus-proxy; do
  printf '#!/bin/sh\nexit 0\n' >"$app_lib/webkitgtk-6.0/$helper"
  chmod +x "$app_lib/webkitgtk-6.0/$helper"
done
: >"$app_lib/webkitgtk-6.0/injected-bundle/libwebkitgtkinjectedbundle.so"
: >"$app_lib/gio/modules/libgiognutls.so"
printf '"@APPDIR@/%s/gdk-pixbuf-2.0/2.10.0/loaders/libpixbufloader-bmp.so"\n' "$lib_dir_rel" \
  >"$app_lib/gdk-pixbuf-2.0/2.10.0/loaders.cache"

cat >"$fixture/tools/ldd" <<'LDD'
#!/usr/bin/env bash
[[ $1 == -r && $2 == */usr/bin/dan-tick ]] || exit 2
printf '%s\n' "${LDD_OUTPUT:-libgtk-4.so.1 => /package/libgtk-4.so.1}"
exit "${LDD_EXIT:-0}"
LDD
cat >"$fixture/tools/zenity" <<'ZENITY'
#!/usr/bin/env bash
printf '%s\n' "$@" >"$DIALOG_RESULT"
ZENITY
chmod +x "$fixture/tools/ldd" "$fixture/tools/zenity"

export PATH="$fixture/tools:$PATH" LAUNCH_RESULT="$fixture/launched" DIALOG_RESULT="$fixture/dialog"
export XDG_CACHE_HOME="$fixture/cache" GDK_BACKEND=x11 LD_LIBRARY_PATH=/custom/library
unset WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS
unset GIO_EXTRA_MODULES GDK_PIXBUF_MODULE_FILE XDG_DATA_DIRS

# Both launch styles must preserve argv and the caller's working directory, and
# must expose the packaged resources instead of the host's.
ln -s "$app_dir/AppRun" "$fixture/entry link"
cd "$fixture/unrelated"
"$fixture/entry link" 'argument with spaces' '--flag=value'
{
  printf '%s\n' "$PWD"
  printf 'GIO_EXTRA_MODULES=%s\n' "$app_lib/gio/modules"
  printf 'GDK_PIXBUF_MODULE_FILE=%s\n' "$fixture/cache/dan-tick/loaders.cache"
  printf 'XDG_DATA_DIRS=%s\n' "$app_dir/usr/share:/usr/local/share:/usr/share"
  printf 'LD_LIBRARY_PATH=%s\n' /custom/library
  printf '%s\n' 'WEBKIT_EXEC_PATH=' 'GDK_BACKEND=x11' 'WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS='
  printf '%s\n' 'arguments=argument with spaces --flag=value'
} >"$fixture/expected"
cmp "$fixture/expected" "$LAUNCH_RESULT"
rm "$LAUNCH_RESULT"

# The image loader cache is rewritten for the current mount point, because
# gdk-pixbuf only accepts absolute loader paths.
grep -q "^\"$app_dir/$lib_dir_rel/gdk-pixbuf-2.0/2.10.0/loaders/libpixbufloader-bmp.so\"$" \
  "$fixture/cache/dan-tick/loaders.cache"

expect_failure() {
  local expected=$1
  if "$app_dir/AppRun" >"$fixture/stdout" 2>"$fixture/stderr"; then
    echo 'An incomplete package unexpectedly launched.' >&2
    exit 1
  fi
  [[ ! -e $LAUNCH_RESULT ]]
  grep -q "$expected" "$fixture/stderr"
}

# A missing auxiliary process, a missing host library and an incomplete package
# must all fail with a message instead of starting a broken window.
export DISPLAY=:99
mv "$app_lib/webkitgtk-6.0/WebKitWebProcess" "$fixture/web-process"
expect_failure '安装包不完整'
grep -q 'dan-tick 启动失败' "$DIALOG_RESULT"
mv "$fixture/web-process" "$app_lib/webkitgtk-6.0/WebKitWebProcess"

export LDD_OUTPUT='libgtk-4.so.1 => not found'
expect_failure 'libgtk-4.so.1 => not found'

export LDD_OUTPUT="version 'GLIBC_2.39' not found" LDD_EXIT=1
expect_failure 'GLIBC_2.39'

echo 'AppImage launcher checks passed.'
