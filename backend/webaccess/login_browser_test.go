package webaccess

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Opt in with UNITERM_WEB_LAN_HOST=<assigned non-loopback IPv4>, CHROME_PATH,
// and NODE_PATH pointing to node_modules containing the existing playwright-core.
func TestLANLoginWithChrome(t *testing.T) {
	host := os.Getenv("UNITERM_WEB_LAN_HOST")
	if host == "" {
		t.Skip("set UNITERM_WEB_LAN_HOST, CHROME_PATH, and NODE_PATH for the real Chrome LAN form regression")
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.To4() == nil || ip.IsLoopback() || ip.IsUnspecified() {
		t.Fatal("UNITERM_WEB_LAN_HOST must be an assigned non-loopback IPv4 address")
	}
	if os.Getenv("CHROME_PATH") == "" {
		t.Fatal("CHROME_PATH must identify the Chrome executable")
	}
	s, _ := testServer(t)
	token := GenerateToken()
	if err := s.Configure(Config{Enabled: true, Host: "0.0.0.0", StartPort: freePort(t)}, token); err != nil {
		t.Fatal(err)
	}
	status := s.Status()
	base := fmt.Sprintf("http://%s:%d", host, status.ActualPort)
	advertised := false
	for _, address := range status.URLs {
		advertised = advertised || address == base
	}
	if !advertised {
		t.Fatal("UNITERM_WEB_LAN_HOST is not an advertised local interface")
	}
	input, err := json.Marshal(map[string]string{"url": base, "token": token, "instanceId": status.InstanceID})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/lan_login_browser.mjs")
	// Pass credentials over stdin, never argv, URLs, or diagnostic files.
	cmd.Stdin = strings.NewReader(string(input))
	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		t.Log(strings.ReplaceAll(string(output), token, "<REDACTED>"))
	}
	if err != nil {
		t.Fatalf("Chrome LAN login regression: %v", err)
	}
}
