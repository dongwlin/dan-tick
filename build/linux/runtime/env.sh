# Pinned inputs for the self-contained Linux runtime and the AppImage build.
#
# Every Linux artifact is produced from these pins: the same base image, the same
# WebKitGTK source (with build/linux/runtime/relocatable-runtime.patch) and the
# same toolchains. Sourced by the scripts in build/linux/; not executable itself.

# Release baseline for Linux: glibc 2.39, GTK 4.14. Building here keeps the
# resulting AppImage runnable on Ubuntu 26.04, Debian 13 and Fedora 44.
UBUNTU_BASE_IMAGE=ubuntu:24.04
UBUNTU_BASE_DIGEST=sha256:534baea6a22c03a63003dbc8dbe78fe34bc0d7e595d9a9dc9834884ff530eb55

# WebKitGTK is rebuilt from source so the runtime can locate its own helper
# processes, injected bundle and sandbox paths after being relocated into an
# AppImage. See relocatable-runtime.patch for the complete change.
WEBKITGTK_VERSION=2.52.6
WEBKITGTK_URL="https://webkitgtk.org/releases/webkitgtk-${WEBKITGTK_VERSION}.tar.xz"
WEBKITGTK_SHA256=179a2ea3f8f6edd4be7f31fdc55afc57bd0729f1fba648c61d4181539ac116fc

# Toolchains copied into the packaging image, so the desktop binary, the GTK /
# WebKit stack and every collected module come from one environment.
GO_IMAGE=golang:1.25.4-bookworm
GO_IMAGE_DIGEST=sha256:e17419604b6d1f9bc245694425f0ec9b1b53685c80850900a376fb10cb0f70cb
NODE_IMAGE=node:24.18.0-bookworm-slim
NODE_IMAGE_DIGEST=sha256:6f7b03f7c2c8e2e784dcf9295400527b9b1270fd37b7e9a7285cf83b6951452d
PNPM_VERSION=12.6.0
WAILS_VERSION=v3.0.0-beta.25

# Revision of the published WebKit runtime. Bump it whenever the WebKit version,
# the relocation patch or the build options change: the id selects the release
# asset, the Docker tag and the cache directory.
RUNTIME_REVISION=r1
RUNTIME_ID="webkitgtk-${WEBKITGTK_VERSION}-${RUNTIME_REVISION}"
RUNTIME_ASSET="dan-tick-webkit-runtime-${RUNTIME_ID}.tar.gz"
RUNTIME_RELEASE_TAG="webkit-runtime-${RUNTIME_ID}"
RUNTIME_RELEASE_REPO="dongwlin/dan-tick"

# Filled in from the published artifact; verified before every AppImage build.
RUNTIME_ASSET_SHA256=""
