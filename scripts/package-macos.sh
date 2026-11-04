#!/usr/bin/env bash
set -euo pipefail

arch=${1:?Choose amd64 or arm64}
case "$arch" in amd64|arm64) ;; *) echo 'Unsupported architecture' >&2; exit 1 ;; esac
project=$(cd "$(dirname "$0")/.." && pwd)
bundle="$project/build/bin/Shell Anywhere.app"
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT

plutil -lint "$bundle/Contents/Info.plist"
codesign --verify --deep --strict "$bundle"
ditto "$bundle" "$stage/Shell Anywhere.app"
ln -s /Applications "$stage/Applications"
hdiutil create -volname 'Shell Anywhere' -srcfolder "$stage" -ov -format UDZO "$project/build/bin/ShellAnywhere-macos-$arch.dmg"
