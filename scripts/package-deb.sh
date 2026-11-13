#!/usr/bin/env bash
# Build a .deb from a compiled binary, laid out like upstream's package.
# The file list, config files and post-install script come from upstream's own
# fpm config (.fpm_systemd in the source tree), so they follow upstream.
#
# usage: scripts/package-deb.sh TREE BINARY DEB_ARCH VERSION OUT_DEB [LIBCRONET_SO]
#   TREE      prepared source tree (scripts/prepare.sh output)
#   DEB_ARCH  amd64 | arm64
#   VERSION   e.g. 1.15.0-alpha.13 (the package version becomes 1.15.0~alpha.13+ut)
#   LIBCRONET_SO  optional; installed as /usr/lib/libcronet.so, where the naive
#                 outbound looks for it
set -euo pipefail
tree=$(cd "$1" && pwd); binary=$(realpath "$2"); arch=$3; version=$4; out=$(realpath -m "$5")
spec="$tree/.fpm_systemd"
[ -f "$spec" ] || { echo "package-deb: $spec not found" >&2; exit 1; }

stage=$(mktemp -d); chmod 755 "$stage"
trap 'rm -rf "$stage"' EXIT
mkdir -p "$stage/DEBIAN"
install -Dm755 "$binary" "$stage/usr/bin/sing-box"
if [ -n "${6:-}" ]; then install -Dm644 "$6" "$stage/usr/lib/libcronet.so"; fi

conffiles=(); postinst=
while IFS= read -r line; do
  case $line in
    --config-files\ *) conffiles+=("${line#--config-files }") ;;
    --after-install\ *) postinst=${line#--after-install } ;;
    --*|'') ;;
    *=*) install -Dm644 "$tree/${line%%=*}" "$stage${line#*=}" ;;
  esac
done < "$spec"

if [ ${#conffiles[@]} -gt 0 ]; then printf '%s\n' "${conffiles[@]}" > "$stage/DEBIAN/conffiles"; fi
if [ -n "$postinst" ]; then install -m755 "$tree/$postinst" "$stage/DEBIAN/postinst"; fi

cat > "$stage/DEBIAN/control" <<CONTROL
Package: sing-box
Version: ${version//-/\~}+ut
Architecture: $arch
Maintainer: 10ta <84114181+10ta@users.noreply.github.com>
Installed-Size: $(du -sk --exclude=DEBIAN "$stage" | cut -f1)
Section: net
Priority: optional
Homepage: https://github.com/10ta/sing-box-ut
Description: sing-box with a native VLESS + XHTTP client (sing-box-ut)
 The universal proxy platform, built from upstream sing-box with the XHTTP
 client and VLESS encryption of sing-box-lx and split upload/download.
 Replaces the official sing-box package.
CONTROL

mkdir -p "$(dirname "$out")"
dpkg-deb --root-owner-group -Zxz --build "$stage" "$out" >/dev/null
echo "package-deb: $out"
