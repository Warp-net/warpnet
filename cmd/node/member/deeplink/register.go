package deeplink

// Register claims warpnet:// for this binary. Best-effort; idempotent.
func Register() error {
	return registerPlatform()
}
