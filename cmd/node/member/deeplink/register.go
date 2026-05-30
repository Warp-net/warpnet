package deeplink

import log "github.com/sirupsen/logrus"

// Register claims warpnet:// for this binary. Best-effort; idempotent.
func Register() error {
	if err := registerPlatform(); err != nil {
		return err
	}
	log.Infof("deeplink: scheme %q registered", Scheme)
	return nil
}
