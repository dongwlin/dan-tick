#!/usr/bin/env bash
# Run the final AppImage in a disposable, offline desktop session on a system
# that has no GTK 4 and no WebKitGTK installed.
#
# The script first inspects the package itself (helper permissions, load paths,
# runtime manifest, bundled GIO TLS module), then starts the same AppImage in
# three ways: FUSE mount, --appimage-extract-and-run, and an extracted AppDir
# started from an unrelated working directory with spaces in its path.
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

fail() { printf 'error: %s\n' "$*" >&2; exit 1; }

# ---------------------------------------------------------------------------
# Session runner (re-entered per run mode)
# ---------------------------------------------------------------------------
if [[ ${1:-} == --session ]]; then
  appimage=$2
  results=$3
  mode=$4
  app_pid=
  capture_window() {
    local window_id
    window_id=$(xwininfo -root -tree 2>/dev/null | awk '/"dan-tick"/ { print $1; exit }')
    if [[ -n $window_id ]]; then
      import -window "$window_id" "$results/$mode-window.png" 2>>"$results/screenshot.log" || true
    else
      import -window root "$results/$mode-desktop.png" 2>>"$results/screenshot.log" || true
    fi
  }
  finish() {
    capture_window
    xwininfo -root -tree >"$results/$mode-windows.txt" 2>&1 || true
    ps -eo pid,ppid,comm,args >"$results/$mode-processes.txt" || true
    if [[ -n $app_pid ]]; then
      kill "$app_pid" 2>/dev/null || true
      wait "$app_pid" 2>/dev/null || true
    fi
  }
  trap finish EXIT

  case $mode in
    fuse)
      "$appimage" >"$results/$mode-application.log" 2>&1 &
      ;;
    extract-and-run)
      "$appimage" --appimage-extract-and-run >"$results/$mode-application.log" 2>&1 &
      ;;
    extracted)
      # Start the extracted AppDir from a working directory that contains spaces.
      (cd "$WORK_DIR/working directory" && exec ../squashfs-root/AppRun) \
        >"$results/$mode-application.log" 2>&1 &
      ;;
    *)
      fail "unknown run mode: $mode"
      ;;
  esac
  app_pid=$!

  python3 "$script_dir/appimage-smoke.py" "$app_pid" "$results" --timeout 60
  capture_window
  [[ -s $results/$mode-window.png || -s $results/$mode-desktop.png ]] \
    || fail 'the application window could not be captured.'
  # A page that appears briefly before a helper crash must not pass.
  sleep 3
  kill -0 "$app_pid"
  python3 "$script_dir/appimage-smoke.py" "$app_pid" "$results" --timeout 10
  echo "mode $mode: rendered the Vue page with the packaged WebKit helper and its sandbox enabled."
  exit 0
fi

# ---------------------------------------------------------------------------
# Package inspection
# ---------------------------------------------------------------------------
if [[ $# != 2 ]]; then
  echo "Usage: $0 <artifact directory> <diagnostics directory>" >&2
  exit 2
fi
[[ $(id -u) != 0 ]] || fail 'Run the smoke test as a non-root user.'
artifact_dir=$(realpath "$1")
results=$(realpath "$2")
mkdir -p "$results"
cat /proc/self/attr/current >"$results/security-label.txt" 2>&1 || true
cat /etc/os-release >"$results/distribution.txt"

# The point of this job is that the package needs no GTK or WebKit from the
# system, so their absence is a precondition, not an accident.
if command -v dpkg-query >/dev/null 2>&1; then
  dpkg-query -W -f='${Package} ${Status}\n' >"$results/system-packages.txt" 2>/dev/null || true
  awk '$2 == "install" && $3 == "ok" && $4 == "installed" { print $1 }' \
    "$results/system-packages.txt" >"$results/system-packages-installed.txt"
else
  rpm -qa >"$results/system-packages-installed.txt"
fi
for forbidden in libgtk-4-1 libwebkitgtk-6.0-4 libjavascriptcoregtk-6.0-1 libwebkit2gtk-4.1-0 \
  gtk4 webkitgtk6.0 webkitgtk6.0-devel webkit2gtk4.1; do
  if grep -qx "$forbidden" "$results/system-packages-installed.txt"; then
    fail "$forbidden is installed: this environment cannot prove self-containment."
  fi
done

shopt -s nullglob
images=("$artifact_dir"/*.AppImage)
[[ ${#images[@]} == 1 ]] || fail 'Expected exactly one final AppImage.'
appimage=${images[0]}
version=$(basename -- "$appimage" | sed -E 's/^dan-tick-(.*)-linux-amd64\.AppImage$/\1/')

task_root=$(mktemp -d /tmp/dan-tick-appimage-smoke.XXXXXX)
xvfb_pid=
finish() {
  if [[ -d $task_root/state/dan-tick/logs ]]; then
    cp -r "$task_root/state/dan-tick/logs" "$results/diagnostic-logs"
    chmod -R go+rX "$results/diagnostic-logs"
  fi
  [[ -n $xvfb_pid ]] && { kill "$xvfb_pid" 2>/dev/null || true; wait "$xvfb_pid" 2>/dev/null || true; }
  rm -rf -- "$task_root"
}
trap finish EXIT

mkdir -p "$task_root/home" "$task_root/config" "$task_root/data" "$task_root/cache" \
  "$task_root/state" "$task_root/runtime" "$task_root/tmp"
chmod 0700 "$task_root/runtime"
cp "$appimage" "$task_root/application.AppImage"
chmod +x "$task_root/application.AppImage"

inspection="$task_root/package-inspection"
mkdir -p "$inspection"
(cd "$inspection" && "$task_root/application.AppImage" --appimage-extract >>"$results/extract.log" 2>&1)
appdir="$inspection/squashfs-root"
[[ -d $appdir ]] || fail 'The AppImage could not be extracted.'

# A second copy is started through a path with spaces, from a working directory
# that contains a space as well.
space_root="$task_root/path with spaces"
mkdir -p "$space_root/working directory"
cp --archive "$appdir" "$space_root/squashfs-root"

runtime_libdir="$appdir/usr/lib/x86_64-linux-gnu"
for helper in WebKitWebProcess WebKitNetworkProcess WebKitGPUProcess; do
  path="$runtime_libdir/webkitgtk-6.0/$helper"
  [[ -f $path ]] || fail "The package does not contain $helper."
  helper_mode=$(stat -c %a "$path")
  [[ $helper_mode == 755 || $helper_mode == 775 ]] || fail "$helper is not executable (mode $helper_mode)."
done
[[ -f $runtime_libdir/webkitgtk-6.0/injected-bundle/libwebkitgtkinjectedbundle.so ]] \
  || fail 'The package does not contain the injected bundle.'
[[ -f $appdir/usr/share/dan-tick/runtime-manifest.txt ]] || fail 'The runtime manifest is missing.'
grep -q '^runtime-id: ' "$appdir/usr/share/dan-tick/runtime-manifest.txt" \
  || fail 'The runtime manifest does not name the WebKit runtime.'
cp "$appdir/usr/share/dan-tick/runtime-manifest.txt" "$results/runtime-manifest.txt"

# Every ELF of the package must resolve its libraries from inside the package.
{
  printf '## %s\n' 'usr/bin/dan-tick'
  ldd "$appdir/usr/bin/dan-tick"
  for helper in "$runtime_libdir"/webkitgtk-6.0/WebKit*Process "$runtime_libdir/webkitgtk-6.0/bwrap" \
    "$runtime_libdir/webkitgtk-6.0/xdg-dbus-proxy" \
    "$runtime_libdir/webkitgtk-6.0/injected-bundle/libwebkitgtkinjectedbundle.so"; do
    printf '## %s\n' "${helper#"$appdir"/}"
    ldd "$helper"
  done
} >"$results/package-ldd.txt" 2>&1
grep -q 'not found' "$results/package-ldd.txt" && fail 'The package has unresolved shared libraries.'
for requirement in libgtk-4.so.1 libwebkitgtk-6.0.so.4 libjavascriptcoregtk-6.0.so.1; do
  resolved=$(awk -v name="$requirement" '$1 == name { print $3 }' "$results/package-ldd.txt" | head -1)
  [[ $resolved == "$appdir"/* ]] || fail "$requirement resolves to '$resolved' instead of the packaged copy."
done

# The bundled GIO TLS module must complete a handshake against the system trust
# store, because the application reaches HTTPS APIs through libsoup only.
if [[ -f $appdir/usr/lib/x86_64-linux-gnu/gio/modules/libgiognutls.so ]]; then
  python3 "$script_dir/appimage-tls-check.py" "$appdir" "$results/gio-tls.txt" \
    || fail 'The bundled GIO TLS module could not complete a TLS handshake.'
else
  fail 'The package does not contain the GIO TLS module.'
fi

# ---------------------------------------------------------------------------
# Desktop sessions
# ---------------------------------------------------------------------------
Xvfb :99 -screen 0 1280x800x24 -nolisten tcp >"$results/xvfb.log" 2>&1 &
xvfb_pid=$!
export DISPLAY=:99
for _ in {1..50}; do
  xdpyinfo >/dev/null 2>&1 && break
  kill -0 "$xvfb_pid"
  sleep 0.1
done
xdpyinfo >/dev/null

if [[ -w /dev/fuse ]]; then
  echo 'fuse' >"$results/fuse-available.txt"
else
  echo 'no /dev/fuse in this environment' >"$results/fuse-available.txt"
fi

modes=(extract-and-run extracted)
[[ -w /dev/fuse ]] && modes=(fuse "${modes[@]}")

for run_mode in "${modes[@]}"; do
  mkdir -p "$task_root/$run_mode"/{home,config,data,cache,state,runtime,tmp}
  chmod 0700 "$task_root/$run_mode/runtime"
  env -u WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS -u DBUS_SESSION_BUS_ADDRESS -u WAYLAND_DISPLAY \
    HOME="$task_root/$run_mode/home" XDG_CONFIG_HOME="$task_root/$run_mode/config" \
    XDG_DATA_HOME="$task_root/$run_mode/data" XDG_CACHE_HOME="$task_root/$run_mode/cache" \
    XDG_STATE_HOME="$task_root/$run_mode/state" XDG_RUNTIME_DIR="$task_root/$run_mode/runtime" \
    TMPDIR="$task_root/$run_mode/tmp" WORK_DIR="$space_root" \
    GDK_BACKEND=x11 GTK_A11Y=atspi LIBGL_ALWAYS_SOFTWARE=1 \
    dbus-run-session -- bash "$0" --session "$task_root/application.AppImage" "$results" "$run_mode" \
    >"$results/$run_mode-session.log" 2>&1 || {
      cat "$results/$run_mode-session.log" >&2
      [[ -f $results/$run_mode-application.log ]] && head -n 40 "$results/$run_mode-application.log" >&2
      fail "run mode $run_mode failed"
    }
  cp "$results/webview.json" "$results/$run_mode-webview.json" 2>/dev/null || true
done

echo "AppImage $version passed package inspection and the ${modes[*]} run modes without system GTK or WebKit."
