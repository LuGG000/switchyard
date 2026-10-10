#!/usr/bin/env bash
# Sets both PKGBUILDs to a released version: pkgver, pkgrel and the checksums, read from the
# release's checksums.txt (switchyard-bin) and from the tag tarball (switchyard).
# Usage: packaging/bump.sh 0.2.0
set -euo pipefail

version="${1:?usage: bump.sh <version without v>}"
here="$(cd "$(dirname "$0")" && pwd)"
base="https://github.com/LuGG000/switchyard"

checksums="$(curl -fsSL "$base/releases/download/v$version/checksums.txt")"
sum_of() { awk -v f="$1" '$2 == f { print $1 }' <<<"$checksums"; }
amd64="$(sum_of "switchyard_${version}_linux_amd64.tar.gz")"
arm64="$(sum_of "switchyard_${version}_linux_arm64.tar.gz")"
source_sum="$(curl -fsSL "$base/archive/refs/tags/v$version.tar.gz" | sha256sum | cut -d' ' -f1)"
for sum in "$amd64" "$arm64" "$source_sum"; do
  [[ "$sum" =~ ^[0-9a-f]{64}$ ]] || { echo "no checksum found for v$version" >&2; exit 1; }
done

sed -i \
  -e "s/^pkgver=.*/pkgver=$version/" \
  -e "s/^pkgrel=.*/pkgrel=1/" \
  -e "s/^sha256sums_x86_64=.*/sha256sums_x86_64=('$amd64')/" \
  -e "s/^sha256sums_aarch64=.*/sha256sums_aarch64=('$arm64')/" \
  "$here/aur-bin/PKGBUILD"
sed -i \
  -e "s/^pkgver=.*/pkgver=$version/" \
  -e "s/^pkgrel=.*/pkgrel=1/" \
  -e "s/^sha256sums=.*/sha256sums=('$source_sum')/" \
  "$here/aur/PKGBUILD"
echo "PKGBUILDs set to $version"
