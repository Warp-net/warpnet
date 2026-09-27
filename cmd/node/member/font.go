//go:build !echo && !remote

package main

import (
	"os"
	"path/filepath"
	"runtime"
)

const fontConfig = `<?xml version="1.0"?>
<!DOCTYPE fontconfig SYSTEM "urn:fontconfig:fonts.dtd">
<fontconfig>
  <include>fonts.conf</include>
  <selectfont>
    <rejectfont>
      <pattern><patelt name="fontwrapper"><string>WOFF</string></patelt></pattern>
      <pattern><patelt name="fontwrapper"><string>WOFF2</string></patelt></pattern>
    </rejectfont>
  </selectfont>
</fontconfig>
`

func setLinuxFontConfig(dir string) error {
	if runtime.GOOS != "linux" || os.Getenv("SNAP") != "" {
		return nil
	}
	path := filepath.Join(dir, "fontconfig.conf")
	if current := os.Getenv("FONTCONFIG_FILE"); current != "" && current != path {
		return nil
	}
	if err := os.WriteFile(path, []byte(fontConfig), 0o600); err != nil {
		return err
	}
	return os.Setenv("FONTCONFIG_FILE", path)
}
