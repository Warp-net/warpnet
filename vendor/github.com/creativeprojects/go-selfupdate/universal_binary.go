package selfupdate

import "debug/macho"

// IsDarwinUniversalBinary checks if the file is a universal binary (also called a fat binary).
func IsDarwinUniversalBinary(filename string) bool {
	file, err := macho.OpenFat(filename)
	if err == nil {
		file.Close()
		return true
	}
	return false
}
