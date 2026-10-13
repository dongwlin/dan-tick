#!/usr/bin/env bash
# Build the desktop application and package the self-contained AppImage.
#
# Everything runs in the pinned packaging image from build/linux/runtime/Dockerfile:
# the application is compiled against the same GTK 4 / WebKitGTK 6.0 libraries that
# are collected into the AppImage, and the patched WebKit runtime comes from the
# published runtime artifact. The host only needs Docker, curl and git.
#
# Usage: package.sh
#
# Environment:
#   DAN_TICK_RUNTIME_CACHE    where the runtime artifact is unpacked (bin/runtime)
#   DAN_TICK_RUNTIME_ARCHIVE  use a local runtime archive instead of downloading
#   DAN_TICK_REBUILD_IMAGE    set to 1 to rebuild the packaging image
#   DAN_TICK_BUILD_CACHE      persistent container cache (bin/build-cache)
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
root_dir=$(cd -- "$script_dir/../../.." && pwd)
# shellcheck source=../runtime/env.sh
source "$script_dir/../runtime/env.sh"

runtime_cache=${DAN_TICK_RUNTIME_CACHE:-"$root_dir/bin/runtime"}
runtime_tree="$runtime_cache/$RUNTIME_ID"
runtime_archive=${DAN_TICK_RUNTIME_ARCHIVE:-"$runtime_cache/$RUNTIME_ASSET"}
image=${DAN_TICK_PACKAGER_IMAGE:-"dan-tick-linux-packager:$RUNTIME_ID"}
cache_dir=${DAN_TICK_BUILD_CACHE:-"$root_dir/bin/build-cache"}

log() { printf '==> %s\n' "$*" >&2; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

verify_archive() {
  local archive=$1
  if [[ -z $RUNTIME_ASSET_SHA256 ]]; then
    die 'RUNTIME_ASSET_SHA256 is not pinned in build/linux/runtime/env.sh; publish the runtime first.'
  fi
  printf '%s  %s\n' "$RUNTIME_ASSET_SHA256" "$archive" | sha256sum --check --status \
    || die "checksum mismatch for $archive"
}

download_archive() {
  local url=$1 status
  status=$(curl --location --retry 3 --connect-timeout 20 --silent --show-error \
    --write-out '%{http_code}' --output "$runtime_archive.part" "$url") || {
    rm -f -- "$runtime_archive.part"
    die "could not download $url"
  }
  if [[ $status != 200 ]]; then
    rm -f -- "$runtime_archive.part"
    if [[ $status == 404 ]]; then
      cat >&2 <<EOF
error: $url is not published yet.
       Run the 'Linux WebKit Runtime' workflow to build and publish it, then pin
       the printed sha256 in build/linux/runtime/env.sh. To package a runtime
       built elsewhere, set DAN_TICK_RUNTIME_ARCHIVE to that archive and
       RUNTIME_ASSET_SHA256 to its checksum.
EOF
      exit 1
    fi
    die "downloading $url returned HTTP $status"
  fi
  mv -- "$runtime_archive.part" "$runtime_archive"
}

if [[ ! -d $runtime_tree/usr/lib ]]; then
  mkdir -p "$runtime_cache"
  if [[ ! -f $runtime_archive ]]; then
    log "downloading $RUNTIME_ASSET"
    download_archive "https://github.com/$RUNTIME_RELEASE_REPO/releases/download/$RUNTIME_RELEASE_TAG/$RUNTIME_ASSET"
  fi
  verify_archive "$runtime_archive"
  log "unpacking the WebKit runtime $RUNTIME_ID"
  rm -rf -- "$runtime_tree"
  mkdir -p "$runtime_tree"
  tar --extract --file "$runtime_archive" --directory "$runtime_tree"
  [[ -f $runtime_tree/usr/lib/x86_64-linux-gnu/webkitgtk-6.0/injected-bundle/libwebkitgtkinjectedbundle.so ]] \
    || die "the runtime archive does not contain the patched WebKit runtime."
fi

if [[ ${DAN_TICK_REBUILD_IMAGE:-} == 1 ]] || ! docker image inspect "$image" >/dev/null 2>&1; then
  log "building the packaging image $image"
  docker build \
    --file "$script_dir/../runtime/Dockerfile" \
    --target packaging \
    --build-arg "UBUNTU_BASE_IMAGE=$UBUNTU_BASE_IMAGE" \
    --build-arg "UBUNTU_BASE_DIGEST=$UBUNTU_BASE_DIGEST" \
    --build-arg "GO_IMAGE=$GO_IMAGE" \
    --build-arg "GO_IMAGE_DIGEST=$GO_IMAGE_DIGEST" \
    --build-arg "NODE_IMAGE=$NODE_IMAGE" \
    --build-arg "NODE_IMAGE_DIGEST=$NODE_IMAGE_DIGEST" \
    --build-arg "PNPM_VERSION=$PNPM_VERSION" \
    --build-arg "WAILS_VERSION=$WAILS_VERSION" \
    --build-arg "RUNTIME_ID=$RUNTIME_ID" \
    --tag "$image" \
    "$script_dir/../runtime" >&2
fi

mkdir -p "$cache_dir/home"
log "building the application and packaging the AppImage"
docker run --rm \
  --user "$(id -u):$(id -g)" \
  --env HOME=/cache/home \
  --env "DAN_TICK_WEBKIT_RUNTIME=/webkit-runtime" \
  --env "DAN_TICK_VERSION=${DAN_TICK_VERSION:-}" \
  --env "DAN_TICK_PACKAGE_VERSION=${DAN_TICK_PACKAGE_VERSION:-}" \
  --env "DAN_TICK_BUILD_COMMIT=${DAN_TICK_COMMIT:-$(git -C "$root_dir" rev-parse --short HEAD 2>/dev/null || echo unknown)}" \
  --env "CI=${CI:-}" \
  --volume "$root_dir:$root_dir" \
  --volume "$runtime_tree:/webkit-runtime:ro" \
  --volume "$cache_dir:/cache" \
  --workdir "$root_dir" \
  "$image" bash "$root_dir/build/linux/appimage/container-build.sh"

[[ -x $root_dir/bin/dan-tick-x86_64.AppImage ]] || die 'the AppImage was not produced.'
log "AppImage ready: $root_dir/bin/dan-tick-x86_64.AppImage ($(du -h "$root_dir/bin/dan-tick-x86_64.AppImage" | cut -f1))"
