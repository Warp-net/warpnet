//go:build !openbsd
// +build !openbsd

package protect

func unveil(path string, flags string) error {
	return nil
}

func unveilBlock() error {
	return nil
}

func pledge(promises string) error {
	return nil
}
