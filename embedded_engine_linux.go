//go:build linux && !android

package warpnet

import _ "embed"

//go:embed core/wallet/payment-engine
var paymentEngine []byte

func GetPaymentEngine() []byte {
	return paymentEngine
}
