#!/usr/bin/env bash
set -euo pipefail

arch=${1:?Choose amd64 or arm64}
version=${2:?Pass the release version}
case "$arch" in amd64|arm64) ;; *) echo 'Unsupported architecture' >&2; exit 1 ;; esac
version=${version#v}
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.+-]+)?$ ]]; then
    echo 'Invalid release version' >&2
    exit 1
fi

project=$(cd "$(dirname "$0")/.." && pwd)
output="$project/build/bin"
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT

portable="$stage/ShellAnywhere-linux-$arch"
mkdir -p "$portable"
install -m 755 "$output/ShellAnywhere" "$portable/ShellAnywhere"
install -m 644 "$project/build/appicon.png" "$portable/shell-anywhere.png"
install -m 644 "$project/LICENSE" "$portable/LICENSE"
cat > "$portable/README.txt" <<'EOF'
Shell Anywhere

Run ./ShellAnywhere to start the app
Requires a Linux desktop with glibc 2.39+, GTK 3, WebKitGTK 4.1, and a D-Bus session
On Ubuntu 24.04: sudo apt install libgtk-3-0t64 libwebkit2gtk-4.1-0 xdg-utils
Saved credentials need a Secret Service provider, such as GNOME Keyring
Use the .deb package for desktop launcher integration on Debian/Ubuntu systems
EOF
tar -C "$stage" -czf "$output/ShellAnywhere-linux-$arch.tar.gz" "ShellAnywhere-linux-$arch"

package="$stage/package"
mkdir -p "$package/DEBIAN" "$package/usr/bin" "$package/usr/share/applications" "$package/usr/share/pixmaps" "$package/usr/share/doc/shell-anywhere"
install -m 755 "$output/ShellAnywhere" "$package/usr/bin/ShellAnywhere"
install -m 644 "$project/build/linux/shell-anywhere.desktop" "$package/usr/share/applications/shell-anywhere.desktop"
install -m 644 "$project/build/appicon.png" "$package/usr/share/pixmaps/shell-anywhere.png"
install -m 644 "$project/LICENSE" "$package/usr/share/doc/shell-anywhere/copyright"
cat > "$package/DEBIAN/control" <<EOF
Package: shell-anywhere
Version: ${version/-/~}
Section: net
Priority: optional
Architecture: $arch
Maintainer: Oxee
Installed-Size: $(du -sk "$package/usr" | cut -f1)
Depends: libc6 (>= 2.39), libgtk-3-0t64 | libgtk-3-0, libwebkit2gtk-4.1-0, xdg-utils
Recommends: gnome-keyring
Description: Shell Anywhere SSH and SFTP client
 Manage SSH hosts, local terminals, identities, and file transfers
EOF
dpkg-deb --build --root-owner-group "$package" "$output/ShellAnywhere-linux-$arch.deb"
