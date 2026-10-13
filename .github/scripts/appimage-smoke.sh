#!/usr/bin/env bash
# Run the final AppImage in a disposable, offline desktop session.
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

if [[ ${1:-} == --session ]]; then
  appimage=$2
  results=$3
  app_pid=
  capture_window() {
    local window_id
    window_id=$(xwininfo -root -tree 2>/dev/null | awk '/"dan-tick"/ { print $1; exit }')
    if [[ -n $window_id ]]; then
      import -window "$window_id" "$results/window.png" 2>>"$results/screenshot.log" || true
    else
      import -window root "$results/desktop.png" 2>>"$results/screenshot.log" || true
    fi
  }
  finish() {
    capture_window
    xwininfo -root -tree >"$results/windows.txt" 2>&1 || true
    ps -eo pid,ppid,comm,args >"$results/processes.txt" || true
    if [[ -n $app_pid ]]; then
      kill "$app_pid" 2>/dev/null || true
      wait "$app_pid" 2>/dev/null || true
    fi
  }
  trap finish EXIT
  "$appimage" --appimage-extract-and-run >"$results/application.log" 2>&1 &
  app_pid=$!
  python3 "$script_dir/appimage-smoke.py" "$app_pid" "$results"
  capture_window
  [[ -s $results/window.png ]] || { echo 'The application window could not be captured.' >&2; exit 1; }
  # A page that appears briefly before a helper crash must not pass.
  sleep 3
  kill -0 "$app_pid"
  python3 "$script_dir/appimage-smoke.py" "$app_pid" "$results" --timeout 5
  echo 'AppImage rendered the Vue page with the system WebKit helper and its sandbox enabled.'
  exit 0
fi

if [[ $# != 2 ]]; then
  echo "Usage: $0 <artifact directory> <diagnostics directory>" >&2
  exit 2
fi
[[ $(id -u) != 0 ]] || { echo 'Run the smoke test as a non-root user.' >&2; exit 1; }
artifact_dir=$(realpath "$1")
results=$(realpath "$2")
mkdir -p "$results"
shopt -s nullglob
images=("$artifact_dir"/*.AppImage)
[[ ${#images[@]} == 1 ]] || { echo 'Expected exactly one final AppImage.' >&2; exit 1; }

task_root=$(mktemp -d /tmp/dan-tick-appimage-smoke.XXXXXX)
xvfb_pid=
finish() {
  if [[ -d $task_root/state/dan-tick/logs ]]; then
    cp -r "$task_root/state/dan-tick/logs" "$results/diagnostic-logs"
    chmod -R go+rX "$results/diagnostic-logs"
  fi
  if [[ -n $xvfb_pid ]]; then
    kill "$xvfb_pid" 2>/dev/null || true
    wait "$xvfb_pid" 2>/dev/null || true
  fi
  rm -rf -- "$task_root"
}
trap finish EXIT
mkdir -p "$task_root"/{home,config,data,cache,state,runtime,tmp}
chmod 0700 "$task_root/runtime"
cp "${images[0]}" "$task_root/application.AppImage"
chmod +x "$task_root/application.AppImage"
cat /etc/os-release >"$results/distribution.txt"
if command -v dpkg-query >/dev/null; then
  dpkg-query --show libgtk-4-1 libwebkitgtk-6.0-4 >"$results/runtime-packages.txt"
else
  rpm --query gtk4 webkitgtk6.0 >"$results/runtime-packages.txt"
fi

# The package must work without FUSE inside CI. This still executes its final
# runtime and AppRun, rather than launching a separately compiled executable.
Xvfb :99 -screen 0 1280x800x24 -nolisten tcp >"$results/xvfb.log" 2>&1 &
xvfb_pid=$!
export DISPLAY=:99
for attempt in {1..50}; do
  if xdpyinfo >/dev/null 2>&1; then break; fi
  kill -0 "$xvfb_pid"
  sleep 0.1
done
xdpyinfo >/dev/null

env -u WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS -u DBUS_SESSION_BUS_ADDRESS -u WAYLAND_DISPLAY \
  HOME="$task_root/home" XDG_CONFIG_HOME="$task_root/config" XDG_DATA_HOME="$task_root/data" \
  XDG_CACHE_HOME="$task_root/cache" XDG_STATE_HOME="$task_root/state" \
  XDG_RUNTIME_DIR="$task_root/runtime" TMPDIR="$task_root/tmp" \
  GDK_BACKEND=x11 GTK_A11Y=atspi LIBGL_ALWAYS_SOFTWARE=1 \
  dbus-run-session -- bash "$0" --session "$task_root/application.AppImage" "$results" \
  >"$results/session.log" 2>&1
