//go:build !android

package paymentengine

import _ "embed"

//go:embed bin/payment-engine-linux-amd64.gz
var compressed []byte
