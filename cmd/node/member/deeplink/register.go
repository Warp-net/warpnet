package deeplink

// Register makes the OS route warpnet:// URLs to this binary.
//
// Best-effort and idempotent: callers should log the error but not
// abort startup if registration fails. Without registration the app
// still works — users just can't click warpnet.site/user/{id} links.
//
// macOS: declared in Info.plist (CFBundleURLTypes generated from
// wails.json's info.protocols), no runtime work needed.
// Windows: writes HKCU\Software\Classes\warpnet (no admin required).
// Linux: writes ~/.local/share/applications/warpnet.desktop and
// invokes xdg-mime default.
func Register() error {
	return registerPlatform()
}
