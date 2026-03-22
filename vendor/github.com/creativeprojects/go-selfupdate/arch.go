package selfupdate

import (
	"fmt"
)

const (
	minARM = 5
	maxARM = 7
)

// getAdditionalArch we can use depending on the type of CPU
func getAdditionalArch(arch string, goarm uint8, universalArch string) []string {
	const defaultArchCapacity = 3
	additionalArch := make([]string, 0, defaultArchCapacity)

	if arch == "arm" && goarm >= minARM && goarm <= maxARM {
		// more precise arch at the top of the list
		for v := goarm; v >= minARM; v-- {
			additionalArch = append(additionalArch, fmt.Sprintf("armv%d", v))
		}
		additionalArch = append(additionalArch, "arm")
		return additionalArch
	}

	additionalArch = append(additionalArch, arch)
	if arch == "amd64" {
		additionalArch = append(additionalArch, "x86_64")
	}
	if universalArch != "" {
		additionalArch = append(additionalArch, universalArch)
	}
	return additionalArch
}
