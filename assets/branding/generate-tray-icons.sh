#!/bin/sh
set -eu

# Run from any directory; ImageMagick rasterizes the authored vector source.
branding_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
project_dir=$(CDPATH= cd -- "$branding_dir/../.." && pwd)
output_dir="$project_dir/internal/desktop/icons"
scratch_dir=$(mktemp -d)
trap 'rm -rf "$scratch_dir"' EXIT HUP INT TERM
mkdir -p "$output_dir"

for theme in light dark; do
  case "$theme" in
    light) ink='#08142f' ;;
    dark) ink='#ffffff' ;;
  esac
  sed \
    -e 's/\.halo {[^}]*}/.halo { fill: none; stroke: none; }/' \
    -e 's/\.outline {[^}]*}/.outline { fill: none; stroke: none; }/' \
    -e "s/\.body {[^}]*}/.body { fill: $ink; }/" \
    -e 's/\.face {[^}]*}/.face { fill: none; }/' \
    -e "s/stroke: #08142f; stroke-width: 2/stroke: $ink; stroke-width: 2/" \
    "$branding_dir/tray-icon.svg" > "$scratch_dir/$theme.svg"
  magick -background none -density 384 "$scratch_dir/$theme.svg" \
    -resize 32x32 -strip "PNG32:$scratch_dir/$theme-32.png"
  magick -background none -density 384 "$scratch_dir/$theme.svg" \
    -resize 16x16 -strip "PNG32:$scratch_dir/$theme-16.png"
  magick "$scratch_dir/$theme-16.png" "$scratch_dir/$theme-32.png" \
    -strip "$output_dir/tray-$theme.ico"
done

# Linux does not expose a reliable tray panel theme in Wails beta.25. A white
# outer keyline and navy edge keep the blue mark visible on either panel color.
magick -background none -density 384 "$branding_dir/tray-icon.svg" \
  -resize 24x24 -strip "PNG32:$output_dir/tray-linux.png"
