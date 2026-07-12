#!/bin/bash
# Download conpty.dll and OpenConsole.exe from Microsoft's Windows Terminal release.
# Called before each build so the DLLs are always available alongside the binary.

set -e

VERSION="1.24.11321.0"
NUPKG="Microsoft.Windows.Console.ConPTY.${VERSION}.nupkg"
URL="https://github.com/microsoft/terminal/releases/download/v${VERSION}/${NUPKG}"
TMPDIR=$(mktemp -d)
TARGET_DIR="$(dirname "$0")/.."

echo "Downloading ConPTY ${VERSION}..."

curl -sSL -o "$TMPDIR/$NUPKG" "$URL"
unzip -qo "$TMPDIR/$NUPKG" \
    "runtimes/win-x64/native/conpty.dll" \
    "build/native/runtimes/x64/OpenConsole.exe" \
    -d "$TMPDIR"

cp "$TMPDIR/runtimes/win-x64/native/conpty.dll" "$TARGET_DIR/"
cp "$TMPDIR/build/native/runtimes/x64/OpenConsole.exe" "$TARGET_DIR/"

rm -rf "$TMPDIR"
echo "Done: conpty.dll + OpenConsole.exe"
