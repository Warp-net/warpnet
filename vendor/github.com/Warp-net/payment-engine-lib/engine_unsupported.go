//go:build android || ios || (!(linux && amd64) && !(linux && arm64) && !(darwin && amd64) && !(darwin && arm64) && !(windows && amd64))

package paymentengine

func GetPaymentEngine() ([]byte, error) { return nil, ErrUnsupported }
