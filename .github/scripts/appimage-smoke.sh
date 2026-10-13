#!/usr/bin/env bash
# Run the final thin AppImage in a disposable, offline desktop session with
# the distribution's GTK 4 and WebKitGTK 6.0 installed.
#
# The script first checks that the package has no bundled runtime and resolves
# its libraries from the system, then starts the same AppImage in
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
  echo "mode $mode: rendered the Vue page with system WebKit and its sandbox enabled."
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

# Record the distribution runtime versions alongside the startup diagnostics.
if command -v dpkg-query >/dev/null 2>&1; then
  dpkg-query -W -f='${binary:Package} ${Version} ${Status}\n' >"$results/system-packages.txt" 2>/dev/null || true
else
  rpm -qa >"$results/system-packages.txt"
fi

shopt -s nullglob
images=("$artifact_dir"/*.AppImage)
[[ ${#images[@]} == 1 ]] || fail 'Expected exactly one final AppImage.'
appimage=${images[0]}
version=$(basename -- "$appimage" | sed -E 's/^dan-tick-(.*)-linux-amd64\.AppImage$/\1/')

task_root=$(mktemp -d /tmp/dan-tick-appimage-smoke.XXXXXX)
xvfb_pid=
finish() {
  local log_dir log_mode
  for log_dir in "$task_root"/*/state/dan-tick/logs; do
    [[ -d $log_dir ]] || continue
    log_mode=${log_dir#"$task_root"/}
    log_mode=${log_mode%%/*}
    cp -r "$log_dir" "$results/$log_mode-diagnostic-logs"
    chmod -R go+rX "$results/$log_mode-diagnostic-logs"
  done
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

[[ -x $appdir/AppRun && -x $appdir/usr/bin/dan-tick ]] \
  || fail 'AppRun or the application executable is missing or not executable.'
find "$appdir" -printf '%P\n' | sort >"$results/package-files.txt"

# The AppDir may contain desktop metadata and icons, but the application must
# be its only ELF. In particular it cannot carry WebKit, helpers or GIO modules.
while IFS= read -r -d '' package_file; do
  [[ $package_file == "$appdir/usr/bin/dan-tick" ]] && continue
  [[ $(od -An -tx1 -N4 "$package_file" | tr -d ' \n') != 7f454c46 ]] \
    || fail "The thin package contains another ELF: ${package_file#"$appdir"/}."
done < <(find "$appdir" -type f -print0)
for forbidden_dir in usr/lib usr/lib64 usr/libexec; do
  [[ ! -e $appdir/$forbidden_dir ]] || fail "The thin package contains $forbidden_dir."
done
[[ ! -e $appdir/usr/share/dan-tick/runtime-manifest.txt ]] \
  || fail 'The thin package still contains a bundled-runtime manifest.'

ldd "$appdir/usr/bin/dan-tick" >"$results/package-ldd.txt" 2>&1
grep -q 'not found' "$results/package-ldd.txt" && fail 'The package has unresolved shared libraries.'
for requirement in libgtk-4.so.1 libwebkitgtk-6.0.so.4 libjavascriptcoregtk-6.0.so.1 \
  libsoup-3.0.so.0 libgio-2.0.so.0 libglib-2.0.so.0; do
  resolved=$(awk -v name="$requirement" '$1 == name { print $3 }' "$results/package-ldd.txt" | head -1)
  case $resolved in
    /usr/lib/*|/usr/lib64/*|/lib/*|/lib64/*) ;;
    *) fail "$requirement resolves to '$resolved' instead of the system runtime." ;;
  esac
done

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

echo "AppImage $version passed thin-package inspection and the ${modes[*]} run modes with system GTK and WebKit."
