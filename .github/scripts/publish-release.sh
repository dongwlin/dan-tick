#!/usr/bin/env bash
set -euo pipefail

# Never synthesize a tag or publish a partial set of desktop artifacts.
[[ "${RELEASE_TAG:?}" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]
cd release-assets
test -s "dan-tick-${RELEASE_TAG}-linux-amd64.AppImage"
test -s "dan-tick-${RELEASE_TAG}-windows-amd64.exe"
test -s "dan-tick-${RELEASE_TAG}-windows-amd64-installer.exe"
sha256sum --check SHA256SUMS-linux-amd64.txt SHA256SUMS-windows-amd64.txt

# A failed upload leaves a draft that the same tag's next run can finish.
# Published releases are not silently replaced by a rebuild with a new date.
if draft=$(gh release view "$RELEASE_TAG" --json isDraft --jq '.isDraft'); then
  if [[ "$draft" != 'true' ]]; then
    echo "Release $RELEASE_TAG is already published; leaving its assets unchanged."
    exit 0
  fi
else
  gh release create "$RELEASE_TAG" --verify-tag --draft --title "$RELEASE_TAG" --generate-notes
fi
gh release upload "$RELEASE_TAG" ./* --clobber
gh release edit "$RELEASE_TAG" --draft=false
