//go:build !windows

package main

import "github.com/wailsapp/wails/v3/pkg/application"

// releaseWebView2 is a no-op outside Windows: WebView2 is Windows-only, the
// real implementation lives in webview2_release.go.
func releaseWebView2(window *application.WebviewWindow) {}
