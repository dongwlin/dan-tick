# syntax=docker/dockerfile:1.7
#
# Build and packaging environments for the self-contained Linux AppImage.
#
#   webkit-build    compiles the patched WebKitGTK runtime (slow, cached as a
#                   published runtime artifact instead of being rebuilt per run)
#   webkit-runtime  exports that runtime for `--output type=local`
#   packaging       one Ubuntu 24.04 environment with the GTK 4 / WebKitGTK 6.0
#                   stack, the Go and Node toolchains and the AppImage tooling;
#                   the desktop binary is compiled here and every library that
#                   ends up inside the AppImage is collected from here.
#
# Build with build/linux/runtime/build-webkit-runtime.sh and
# build/linux/appimage/package.sh; do not bind mount anything into the WebKit
# stage, it only needs the files next to this Dockerfile.

ARG UBUNTU_BASE_IMAGE=ubuntu:24.04
ARG UBUNTU_BASE_DIGEST
ARG GO_IMAGE=golang:1.25.4-bookworm
ARG GO_IMAGE_DIGEST
ARG NODE_IMAGE=node:24.18.0-bookworm-slim
ARG NODE_IMAGE_DIGEST

FROM ${GO_IMAGE}@${GO_IMAGE_DIGEST} AS go-toolchain
FROM ${NODE_IMAGE}@${NODE_IMAGE_DIGEST} AS node-toolchain

# ---------------------------------------------------------------------------
# Patched WebKitGTK runtime
# ---------------------------------------------------------------------------
FROM ${UBUNTU_BASE_IMAGE}@${UBUNTU_BASE_DIGEST} AS webkit-build

SHELL ["/bin/bash", "-euo", "pipefail", "-c"]
ARG DEBIAN_FRONTEND=noninteractive
ARG WEBKITGTK_URL
ARG WEBKITGTK_SHA256
ARG BUILD_JOBS=8
ARG WEBKIT_INSTALL_LIBEXECDIR=lib/x86_64-linux-gnu

# The source package lists the build dependencies distribution maintainers use
# for the GTK 4 variant as well, which keeps this list from drifting.
RUN sed -i 's/^Types: deb$/Types: deb deb-src/' /etc/apt/sources.list.d/ubuntu.sources \
    && apt-get update \
    && apt-get build-dep -y webkit2gtk \
    && apt-get install -y --no-install-recommends \
        ca-certificates curl ninja-build patch \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build
RUN curl -fsSL --retry 3 --connect-timeout 20 --output webkitgtk.tar.xz "$WEBKITGTK_URL" \
    && printf '%s  %s\n' "$WEBKITGTK_SHA256" webkitgtk.tar.xz | sha256sum --check --status \
    && mkdir source \
    && tar --extract --file webkitgtk.tar.xz --directory source --strip-components=1 \
    && rm webkitgtk.tar.xz

COPY relocatable-runtime.patch /build/relocatable-runtime.patch
RUN cd source \
    && patch -p1 --batch --forward --dry-run < /build/relocatable-runtime.patch \
    && patch -p1 --batch --forward < /build/relocatable-runtime.patch

# The runtime is installed with the distribution layout (multiarch library
# directory, helpers below <libdir>/webkitgtk-6.0) so that the AppImage keeps
# the same relative structure and the patch finds every path from the loaded
# library. Documentation, introspection, the MiniBrowser, gamepad, speech
# synthesis, journald logging and libbacktrace are left out: dan-tick renders
# its own Vue UI and does not use them. WebCore still requires the media code
# paths, so video and Web Audio stay enabled and the GStreamer libraries are
# bundled without plugins.
RUN cmake -S source -B build -G Ninja \
        -DPORT=GTK \
        -DCMAKE_BUILD_TYPE=Release \
        -DCMAKE_INSTALL_PREFIX=/usr \
        -DCMAKE_INSTALL_LIBDIR=lib/x86_64-linux-gnu \
        -DCMAKE_INSTALL_LIBEXECDIR="$WEBKIT_INSTALL_LIBEXECDIR" \
        -DUSE_GTK4=ON \
        -DENABLE_DOCUMENTATION=OFF \
        -DENABLE_INTROSPECTION=OFF \
        -DENABLE_MINIBROWSER=OFF \
        -DENABLE_JOURNALD_LOG=OFF \
        -DENABLE_BUBBLEWRAP_SANDBOX=ON \
        -DENABLE_GAMEPAD=OFF \
        -DENABLE_SPEECH_SYNTHESIS=OFF \
        -DUSE_FLITE=OFF \
        -DUSE_LIBBACKTRACE=OFF \
    && grep -E 'BUBBLEWRAP|WEBKIT_EXEC|LIBEXEC' build/CMakeCache.txt || true \
    && cmake --build build --parallel "$BUILD_JOBS" \
    && DESTDIR=/webkit-runtime cmake --install build

FROM scratch AS webkit-runtime
COPY --from=webkit-build /webkit-runtime/ /

# ---------------------------------------------------------------------------
# Application build and AppImage packaging
# ---------------------------------------------------------------------------
FROM ${UBUNTU_BASE_IMAGE}@${UBUNTU_BASE_DIGEST} AS packaging

SHELL ["/bin/bash", "-euo", "pipefail", "-c"]
ARG DEBIAN_FRONTEND=noninteractive
ARG PNPM_VERSION=12.6.0
ARG WAILS_VERSION=v3.0.0-beta.25
ARG RUNTIME_ID=unset

# Runtime libraries: these are the GTK 4 / WebKitGTK 6.0 components and the
# GIO, TLS, gdk-pixbuf and schema resources that are collected into the
# AppImage. Development packages are only needed to compile the application,
# which links against the very same libraries that get packaged.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        libgtk-4-1 libgtk-4-dev \
        libwebkitgtk-6.0-4 libjavascriptcoregtk-6.0-1 libwebkitgtk-6.0-dev \
        libglib2.0-0t64 libglib2.0-bin libglib2.0-dev \
        libgdk-pixbuf-2.0-0 libgdk-pixbuf-2.0-dev librsvg2-common \
        glib-networking gsettings-desktop-schemas dconf-gsettings-backend \
        adwaita-icon-theme hicolor-icon-theme \
        bubblewrap xdg-dbus-proxy \
        build-essential pkg-config binutils patchelf file \
        ca-certificates curl gzip tar xz-utils \
    && rm -rf /var/lib/apt/lists/*

# Go and Node come from their own pinned images; the copies are static enough
# to run on the Ubuntu baseline and avoid a second download step per build.
COPY --from=go-toolchain /usr/local/go /usr/local/go
COPY --from=node-toolchain /usr/local/bin/node /usr/local/bin/node
COPY --from=node-toolchain /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln --symbolic /usr/local/lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
    && ln --symbolic /usr/local/lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx \
    && npm install --global --no-fund --no-audit "pnpm@${PNPM_VERSION}" \
    && npm cache clean --force

ENV PATH=/usr/local/go/bin:/root/go/bin:/usr/local/bin:/usr/bin:/bin
ENV GOFLAGS=-mod=mod
RUN go install "github.com/wailsapp/wails/v3/cmd/wails3@${WAILS_VERSION}" \
    && cp /root/go/bin/wails3 /usr/local/bin/wails3 \
    && go clean -cache -modcache

# The AppImage tooling is fetched by the packaging script with a pinned version
# and checksum; only the directories it writes to are prepared here.
#
# The marker file records which runtime revision this image bundles: the
# packaging script refuses to collect libraries outside a pinned image.
RUN printf '%s\n' "$RUNTIME_ID" > /etc/dan-tick-packaging-image

ENV LANG=C.UTF-8
WORKDIR /workspace
