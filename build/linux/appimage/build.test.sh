#!/usr/bin/env bash
# Launcher behavior without a display, network, or actual account data.
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
fixture=$(mktemp -d /tmp/dan-tick-apprun-test.XXXXXX)
trap 'rm -rf -- "$fixture"' EXIT
app_dir="$fixture/path with spaces.AppDir"
mkdir -p "$app_dir/usr/bin" "$fixture/tools" "$fixture/unrelated"
install -m755 "$script_dir/AppRun" "$app_dir/AppRun"
cat >"$app_dir/usr/bin/dan-tick" <<'BINARY'
#!/usr/bin/env bash
printf '%s\n' "$PWD" "${GDK_BACKEND:-}" "${LD_LIBRARY_PATH:-}" "${WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS:-}" "$@" >"$LAUNCH_RESULT"
BINARY
cat >"$fixture/tools/ldd" <<'LDD'
#!/usr/bin/env bash
[[ $1 == -r && $2 == */usr/bin/dan-tick ]] || exit 2
printf '%s\n' "${LDD_OUTPUT:-libgtk-4.so.1 => /lib/libgtk-4.so.1}"
exit "${LDD_EXIT:-0}"
LDD
cat >"$fixture/tools/zenity" <<'ZENITY'
#!/usr/bin/env bash
printf '%s\n' "$@" >"$DIALOG_RESULT"
ZENITY
chmod +x "$app_dir/usr/bin/dan-tick" "$fixture/tools/ldd" "$fixture/tools/zenity"
export PATH="$fixture/tools:$PATH" LAUNCH_RESULT="$fixture/launched" DIALOG_RESULT="$fixture/dialog"
export GDK_BACKEND=wayland LD_LIBRARY_PATH=/custom/library
unset WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS

# Both launch styles must preserve argv and the caller's Wayland environment.
ln -s "$app_dir/AppRun" "$fixture/entry link"
cd "$fixture/unrelated"
"$fixture/entry link" 'argument with spaces' '--flag=value'
printf '%s\n' "$PWD" wayland /custom/library '' 'argument with spaces' '--flag=value' >"$fixture/expected"
cmp "$fixture/expected" "$LAUNCH_RESULT"
rm "$LAUNCH_RESULT"

expect_failure() {
  if "$app_dir/AppRun" >"$fixture/stdout" 2>"$fixture/stderr"; then
    echo 'A missing/incompatible runtime unexpectedly launched.' >&2
    exit 1
  fi
  [[ ! -e $LAUNCH_RESULT ]]
  grep -q 'apt install libgtk-4-1 libwebkitgtk-6.0-4' "$fixture/stderr"
  grep -q 'dnf install gtk4 webkitgtk6.0' "$fixture/stderr"
}

# Missing libraries and an older GTK with the same SONAME need a clear error.
export LDD_OUTPUT='libwebkitgtk-6.0.so.4 => not found' DISPLAY=:99
expect_failure
grep -q 'dan-tick 启动失败' "$DIALOG_RESULT"
export LDD_OUTPUT='undefined symbol: gdk_monitor_get_scale'
expect_failure
export LDD_OUTPUT="version 'GLIBC_2.38' not found" LDD_EXIT=1
expect_failure

echo 'AppImage launcher checks passed.'
