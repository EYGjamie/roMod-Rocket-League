#!/bin/sh
# Builds dist/roMod.app: a menu bar app without a Dock icon.
# The menu bar icon needs cgo, so this runs on a Mac with the Xcode Command
# Line Tools (xcode-select --install). VERSION sets the version shown.
set -eu
cd "$(dirname "$0")/.."

version=${VERSION:-dev}
app=dist/roMod.app
rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"

CGO_ENABLED=1 go build -trimpath -ldflags "-s -w -X main.version=$version" \
	-o "$app/Contents/MacOS/romod" ./cmd/romod

cat > "$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>roMod</string>
	<key>CFBundleIdentifier</key>
	<string>com.github.eygjamie.romod</string>
	<key>CFBundleExecutable</key>
	<string>romod</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>$version</string>
	<key>CFBundleIconFile</key>
	<string>icon</string>
	<key>LSMinimumSystemVersion</key>
	<string>12.0</string>
	<key>LSUIElement</key>
	<true/>
	<key>NSHighResolutionCapable</key>
	<true/>
</dict>
</plist>
PLIST

# The program icon comes from the Windows icon; without it the app still works.
tmp=$(mktemp -d)
if sips -s format png internal/assets/icon.ico --out "$tmp/icon.png" >/dev/null 2>&1 &&
	sips -s format icns "$tmp/icon.png" --out "$app/Contents/Resources/icon.icns" >/dev/null 2>&1; then
	:
else
	echo "Hinweis: Programmsymbol nicht erzeugt" >&2
fi
rm -rf "$tmp"

# An ad-hoc signature keeps macOS from treating the bundle as damaged.
codesign --force --deep --sign - "$app" >/dev/null 2>&1 || echo "Hinweis: nicht signiert" >&2
echo "$app"
