package selfupdate

import "github.com/creativeprojects/go-selfupdate/internal"

func ExecutablePath() (string, error) {
	return internal.GetExecutablePath()
}
