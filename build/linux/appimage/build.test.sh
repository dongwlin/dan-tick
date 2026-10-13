#!/usr/bin/env bash
# Check launcher behavior without a display, network or real account data.
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
fixture=$(mktemp -d /tmp/dan-tick-apprun-test.XXXXXX)
trap 'rm -rf -- "$fixture"' EXIT

app_dir="$fixture/path with spaces.AppDir"
mkdir -p "$app_dir/usr/bin" "$fixture/tools" "$fixture/unrelated"
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

cat >"$fixture/tools/ldd" <<'LDD'
#!/usr/bin/env bash
[[ $1 == -r && $2 == */usr/bin/dan-tick ]] || exit 2
printf '%s\n' "${LDD_OUTPUT:-libgtk-4.so.1 => /usr/lib/libgtk-4.so.1}"
exit "${LDD_EXIT:-0}"
LDD
cat >"$fixture/tools/zenity" <<'ZENITY'
#!/usr/bin/env bash
printf '%s\n' "$@" >"$DIALOG_RESULT"
ZENITY
chmod +x "$fixture/tools/ldd" "$fixture/tools/zenity"

export PATH="$fixture/tools:$PATH" LAUNCH_RESULT="$fixture/launched" DIALOG_RESULT="$fixture/dialog"
export GDK_BACKEND=x11 LD_LIBRARY_PATH=/custom/library XDG_DATA_DIRS=/custom/data
export GIO_EXTRA_MODULES=/custom/gio GDK_PIXBUF_MODULE_FILE=/custom/loaders.cache
unset WEBKIT_EXEC_PATH WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS

# Preserve argv, cwd and existing system resource choices in both launch styles.
ln -s "$app_dir/AppRun" "$fixture/entry link"
cd "$fixture/unrelated"
for entry in "$app_dir/AppRun" "$fixture/entry link"; do
  "$entry" 'argument with spaces' '--flag=value'
  {
    printf '%s\n' "$PWD"
    printf '%s\n' 'GIO_EXTRA_MODULES=/custom/gio' 'GDK_PIXBUF_MODULE_FILE=/custom/loaders.cache'
    printf '%s\n' 'XDG_DATA_DIRS=/custom/data' 'LD_LIBRARY_PATH=/custom/library'
    printf '%s\n' 'WEBKIT_EXEC_PATH=' 'GDK_BACKEND=x11' 'WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS='
    printf '%s\n' 'arguments=argument with spaces --flag=value'
  } >"$fixture/expected"
  cmp "$fixture/expected" "$LAUNCH_RESULT"
  rm "$LAUNCH_RESULT"
done

expect_failure() {
  local expected=$1
  if "$app_dir/AppRun" >"$fixture/stdout" 2>"$fixture/stderr"; then
    echo 'An incomplete runtime unexpectedly launched.' >&2
    exit 1
  fi
  [[ ! -e $LAUNCH_RESULT ]]
  grep -q "$expected" "$fixture/stderr"
}

# Explain missing system dependencies, unresolved symbols and damaged packages.
export DISPLAY=:99
export LDD_OUTPUT='libwebkitgtk-6.0.so.4 => not found'
expect_failure 'libwebkitgtk-6.0.so.4 => not found'
grep -q 'sudo apt install libgtk-4-1 libwebkitgtk-6.0-4' "$DIALOG_RESULT"
grep -q 'libwebkitgtk-6.0.so.4 => not found' "$DIALOG_RESULT"

export LDD_OUTPUT='undefined symbol: gtk_example'
expect_failure 'undefined symbol: gtk_example'

export LDD_OUTPUT="version 'GLIBC_2.39' not found" LDD_EXIT=1
expect_failure 'GLIBC_2.39'

mv "$app_dir/usr/bin/dan-tick" "$fixture/binary"
expect_failure '安装包不完整'

echo 'AppImage launcher checks passed.'
