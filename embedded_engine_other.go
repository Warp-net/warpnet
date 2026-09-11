//go:build !linux && !windows && !darwin

package warpnet

func GetPaymentEngine() []byte {
	return nil
}
