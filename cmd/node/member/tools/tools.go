//go:build tools
// +build tools

package tools

import (
	// for MicrosoftEdgeWebview2Setup.exe embedding
	_ "github.com/wailsapp/wails/v2/internal/webview2runtime"
)
