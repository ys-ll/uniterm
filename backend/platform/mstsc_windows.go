//go:build windows

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// mstscRdpFile is the temp file written before each launch so mstsc picks up
// the pre-filled address/port/username/domain. A fixed name keeps the temp
// directory from accumulating one file per launch.
const mstscRdpFile = "uniterm-launch-mstsc.rdp"

// LaunchMstsc opens the connection in the Windows Remote Desktop client
// (mstsc.exe). A minimal .rdp file is written to the temp directory to
// pre-fill the target, username and domain; mstsc prompts for the password
// itself, since neither its command line nor the .rdp format can carry one
// securely. Only offered on Windows; other platforms return an error.
func LaunchMstsc(opts MstscOptions) error {
	if opts.Host == "" {
		return fmt.Errorf("empty host")
	}
	port := opts.Port
	if port <= 0 {
		port = 3389
	}
	// Mirror mstsc's own username syntax: an empty domain falls back to a
	// "DOMAIN\user" prefix in the user name.
	user, domain := opts.User, opts.Domain
	if user != "" && domain == "" {
		if i := strings.LastIndex(user, `\`); i >= 0 {
			domain, user = user[:i], user[i+1:]
		}
	}

	var b strings.Builder
	// "full address" without the port plus a separate "server port" entry is
	// the pair mstsc itself writes for non-default ports.
	fmt.Fprintf(&b, "full address:s:%s\r\n", opts.Host)
	fmt.Fprintf(&b, "server port:i:%d\r\n", port)
	if user != "" {
		fmt.Fprintf(&b, "username:s:%s\r\n", user)
	}
	if domain != "" {
		fmt.Fprintf(&b, "domain:s:%s\r\n", domain)
	}
	if opts.Admin {
		b.WriteString("administrative session:i:1\r\n")
	}
	path := filepath.Join(os.TempDir(), mstscRdpFile)
	if err := os.WriteFile(path, []byte(b.String()), 0600); err != nil {
		return err
	}

	// No HideConsoleWindow here: it sets STARTUPINFO SW_HIDE, which mstsc
	// honors and keeps its remote-desktop window invisible after connecting.
	// As a GUI program mstsc never opens a console window anyway.
	cmd := exec.Command("mstsc.exe", path)
	return cmd.Start()
}
