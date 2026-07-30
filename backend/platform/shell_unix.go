//go:build !windows

package platform

import "os"

func defaultShellOS() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	if _, err := hasExecutable("bash"); err == nil {
		return "bash"
	}
	return "sh"
}
