//go:build darwin

package deeplink

// macOS registers warpnet:// via Info.plist (wails.json info.protocols).
func registerPlatform() error {
	return nil
}
