#!/usr/bin/env bash
# Build the patched WebKitGTK runtime and export it as the pinned artifact.
#
# The runtime is the expensive part of the Linux pipeline, so it is built here,
# once per runtime revision, and published as a release asset that every
# AppImage build downloads and verifies. Run this script with Docker available;
# pass a destination directory as the first argument to override bin/runtime.
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=env.sh
source "$script_dir/env.sh"

root_dir=$(cd -- "$script_dir/../../.." && pwd)
output_dir=${1:-"$root_dir/bin/runtime"}
build_jobs=${BUILD_JOBS:-8}

rm -rf -- "$output_dir/tree"
mkdir -p "$output_dir/tree"

docker build \
  --file "$script_dir/Dockerfile" \
  --target webkit-runtime \
  --build-arg "UBUNTU_BASE_IMAGE=$UBUNTU_BASE_IMAGE" \
  --build-arg "UBUNTU_BASE_DIGEST=$UBUNTU_BASE_DIGEST" \
  --build-arg "GO_IMAGE=$GO_IMAGE" \
  --build-arg "GO_IMAGE_DIGEST=$GO_IMAGE_DIGEST" \
  --build-arg "NODE_IMAGE=$NODE_IMAGE" \
  --build-arg "NODE_IMAGE_DIGEST=$NODE_IMAGE_DIGEST" \
  --build-arg "WEBKITGTK_URL=$WEBKITGTK_URL" \
  --build-arg "WEBKITGTK_SHA256=$WEBKITGTK_SHA256" \
  --build-arg "BUILD_JOBS=$build_jobs" \
  --output "type=local,dest=$output_dir/tree" \
  "$script_dir"

# Record what went into the runtime so a published artifact can be traced back
# to the exact inputs without rebuilding it.
patch_sha256=$(sha256sum "$script_dir/relocatable-runtime.patch" | cut -d' ' -f1)
cat >"$output_dir/tree/runtime-info.txt" <<EOF
runtime-id: $RUNTIME_ID
webkitgtk-version: $WEBKITGTK_VERSION
webkitgtk-source: $WEBKITGTK_URL
webkitgtk-source-sha256: $WEBKITGTK_SHA256
relocation-patch-sha256: $patch_sha256
base-image: $UBUNTU_BASE_IMAGE@$UBUNTU_BASE_DIGEST
EOF

# Deterministic archive: same inputs, same bytes, so the published checksum
# keeps its meaning.
tar --create --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner \
  --directory "$output_dir/tree" --file - . \
  | gzip --no-name --best >"$output_dir/$RUNTIME_ASSET"
rm -rf -- "$output_dir/tree"

sha256=$(sha256sum "$output_dir/$RUNTIME_ASSET" | cut -d' ' -f1)
printf 'Runtime artifact: %s\n' "$output_dir/$RUNTIME_ASSET"
printf 'sha256: %s\n' "$sha256"
printf 'Publish it with:\n  gh release create %s --title %s --notes "Pinned WebKitGTK runtime for dan-tick AppImages" %s\n' \
  "$RUNTIME_RELEASE_TAG" "$RUNTIME_ID" "$output_dir/$RUNTIME_ASSET"
printf 'Then set in build/linux/runtime/env.sh:\n  RUNTIME_ASSET_SHA256=%s\n' "$sha256"
