package paymentengine

import _ "embed"

//go:embed bin/payment-engine-windows-amd64.exe.gz
var compressed []byte
