//go:build !ios

package paymentengine

import _ "embed"

//go:embed bin/payment-engine-darwin-amd64.gz
var compressed []byte
