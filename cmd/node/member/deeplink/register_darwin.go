//go:build darwin

package deeplink

// registerPlatform is a no-op on macOS. The scheme is registered via
// CFBundleURLTypes in Info.plist, which Wails generates from
// wails.json's info.protocols block at bundle time. LaunchServices
// picks up the association as soon as the .app is on disk.
func registerPlatform() error {
	return nil
}
