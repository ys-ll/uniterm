package webaccess

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
func testServer(t *testing.T) (*Server, string) {
	t.Helper()
	s := New()
	s.SetAssets(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, r.URL.Path) }))
	dir := t.TempDir()
	if err := s.Load(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, dir
}
func request(t *testing.T, s *Server, method, path, body, origin string, cookie *http.Cookie) *http.Response {
	t.Helper()
	base := fmt.Sprintf("http://127.0.0.1:%d", s.Status().ActualPort)
	r, err := http.NewRequest(method, base+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if method == "POST" {
		r.Header.Set("Content-Type", "application/json")
	}
	if origin == "self" {
		origin = base
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	c := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}
func login(t *testing.T, s *Server, token string) *http.Cookie {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"token": token})
	r := request(t, s, "POST", "/web/login", string(body), "self", nil)
	if r.StatusCode != 204 || len(r.Cookies()) != 1 {
		t.Fatalf("login: %d", r.StatusCode)
	}
	return r.Cookies()[0]
}

func TestAuthenticationPersistenceRevocationAndRoutes(t *testing.T) {
	s, dir := testServer(t)
	if st := s.Status(); st.Running || st.Config.Enabled || st.Config.Host != "127.0.0.1" || st.Config.StartPort != 34116 {
		t.Fatalf("defaults: %+v", st)
	}
	s.Handle("/feature", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsBrowser(r.Context()) {
			t.Error("missing browser context")
		}
		fmt.Fprint(w, "feature")
	}))
	cfg := Config{Enabled: true, Host: "0.0.0.0", StartPort: freePort(t)}
	if s.Configure(cfg, "") == nil || s.Status().Running {
		t.Fatal("LAN without token accepted")
	}
	const token = "fixed-test-token-that-must-not-be-saved"
	if err := s.Configure(cfg, token); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/web/bootstrap", "/web/events", "/wails/runtime", "/wails/runtime.js", "/wails/event-payload/a", "/wails/stream/a", "/feature"} {
		if r := request(t, s, "GET", path, "", "", nil); r.StatusCode != 401 {
			t.Errorf("anonymous %s: %d", path, r.StatusCode)
		}
	}
	if r := request(t, s, "POST", "/web/login", `{"token":"wrong"}`, "self", nil); r.StatusCode != 401 {
		t.Fatal("wrong token accepted")
	}
	cookie := login(t, s, token)
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie: %+v", cookie)
	}
	if r := request(t, s, "GET", "/feature", "", "", cookie); r.StatusCode != 200 {
		t.Fatal(r.StatusCode)
	}
	if r := request(t, s, "GET", "/", "", "", cookie); r.StatusCode != 200 {
		t.Fatal(r.StatusCode)
	} else {
		b, _ := io.ReadAll(r.Body)
		if string(b) != "/web.html" {
			t.Fatal(string(b))
		}
	}
	for _, path := range []string{"/feature", "/web/events", "/web/bootstrap", "/wails/runtime", "/web/login"} {
		if r := request(t, s, "POST", path, `{}`, "http://evil.test", cookie); r.StatusCode != 403 {
			t.Errorf("cross-site %s: %d", path, r.StatusCode)
		}
	}
	if r := request(t, s, "POST", "/feature", `{}`, "", cookie); r.StatusCode != 403 {
		t.Fatal("originless mutation accepted")
	}
	b, err := os.ReadFile(filepath.Join(dir, "web_access.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), token) {
		t.Fatal("plaintext persisted")
	}
	fi, _ := os.Stat(filepath.Join(dir, "web_access.json"))
	if fi.Mode().Perm() != 0600 {
		t.Fatal(fi.Mode())
	}

	// A subscribed stream is registered before its ready frame and ends on rotate.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://127.0.0.1:%d/web/events", s.Status().ActualPort), nil)
	req.AddCookie(cookie)
	stream, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	scanner := bufio.NewScanner(stream.Body)
	if !scanner.Scan() || scanner.Text() != "event: ready" {
		t.Fatal("missing ready")
	}
	port := s.Status().ActualPort
	if err := s.Configure(cfg, "rotated-test-token"); err != nil {
		t.Fatal(err)
	}
	if s.Status().ActualPort != port {
		t.Fatal("rotation moved listener")
	}
	for scanner.Scan() {
	}
	if ctx.Err() != nil {
		t.Fatal("rotation left stream alive")
	}
	if request(t, s, "GET", "/feature", "", "", cookie).StatusCode != 401 {
		t.Fatal("old cookie survived")
	}
	s.Close()
	reloaded := New()
	reloaded.SetAssets(http.NotFoundHandler())
	defer reloaded.Close()
	if err := reloaded.Load(dir); err != nil {
		t.Fatal(err)
	}
	newCookie := login(t, reloaded, "rotated-test-token")
	if newCookie.Name == cookie.Name {
		t.Fatal("cookie name not isolated by incarnation")
	}
	if request(t, reloaded, "GET", "/web/bootstrap", "", "", cookie).StatusCode != 401 {
		t.Fatal("old instance cookie survived")
	}
	cfg.Enabled = false
	if err := reloaded.Configure(cfg, ""); err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal("disable did not release port", err)
	}
	l.Close()
}

func TestLoopbackOriginHostAndReconfigure(t *testing.T) {
	s, _ := testServer(t)
	cfg := Config{Enabled: true, Host: "127.0.0.1", StartPort: freePort(t)}
	if err := s.Configure(cfg, ""); err != nil {
		t.Fatal(err)
	}
	if request(t, s, "GET", "/web/bootstrap", "", "", nil).StatusCode != 200 {
		t.Fatal("loopback exemption missing")
	}
	if request(t, s, "GET", "/wails/runtime?object=0&method=0", "", "", nil).StatusCode != 405 {
		t.Fatal("legacy GET RPC exposed")
	}
	for _, tc := range []struct{ host, origin, site string }{
		{"evil.test", "", ""}, {"127.0.0.1:1", "", ""}, {"", "null", ""}, {"", "", "cross-site"}, {"", "", "same-site"},
	} {
		r := httptest.NewRequest("GET", s.Status().URLs[0]+"/wails/runtime", nil)
		r.RemoteAddr = "127.0.0.1:1234"
		if tc.host != "" {
			r.Host = tc.host
		}
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Sec-Fetch-Site", tc.site)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("%+v: %d", tc, w.Code)
		}
	}
	oldPort := s.Status().ActualPort
	cfg.StartPort = freePort(t)
	if err := s.Configure(cfg, ""); err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", oldPort))
	if err != nil {
		t.Fatal(err)
	}
	l.Close()
	if s.Status().ActualPort != cfg.StartPort {
		t.Fatal("reconfigure ignored")
	}
}

func TestLoginFormReferrerPolicyAndOriginChecks(t *testing.T) {
	s, _ := testServer(t)
	if err := s.Configure(Config{Enabled: true, Host: "0.0.0.0", StartPort: freePort(t)}, "login-policy-test-token"); err != nil {
		t.Fatal(err)
	}
	form := request(t, s, "GET", "/web/login", "", "", nil)
	if form.StatusCode != 200 || form.Header.Get("Referrer-Policy") != "same-origin" || !strings.Contains(form.Header.Get("Content-Security-Policy"), "form-action 'self'") {
		t.Fatal("login form must preserve its same-origin signal and restrict form submission")
	}
	other := request(t, s, "GET", "/web/bootstrap", "", "", nil)
	if other.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("login policy override escaped the login document")
	}
	base := s.Status().URLs[0]
	for _, tc := range []struct{ name, origin, host string }{
		{"missing origin", "", ""},
		{"opaque origin", "null", ""},
		{"cross origin", "http://other.invalid", ""},
		{"unlisted host", "http://other.invalid", "other.invalid"},
	} {
		r := httptest.NewRequest("POST", base+"/web/login", strings.NewReader(`{"token":"login-policy-test-token"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", tc.origin)
		// A plausible Referer alone must not authorize an originless mutation.
		r.Header.Set("Referer", base+"/web/login")
		if tc.host != "" {
			r.Host = tc.host
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden || w.Header().Get("Set-Cookie") != "" {
			t.Fatalf("%s login was not rejected: %d", tc.name, w.Code)
		}
	}
}

func TestMultipleInstancesLogoutDisableAndFailedReconfigure(t *testing.T) {
	first, _ := testServer(t)
	second, dir := testServer(t)
	cfg := Config{Enabled: true, Host: "127.0.0.1", StartPort: freePort(t)}
	if err := first.Configure(cfg, "first-token"); err != nil {
		t.Fatal(err)
	}
	if err := second.Configure(cfg, "second-token"); err != nil {
		t.Fatal(err)
	}
	if first.Status().ActualPort == second.Status().ActualPort {
		t.Fatal("instances shared listener")
	}
	c1 := login(t, first, "first-token")
	c2 := login(t, second, "second-token")
	if c1.Name == c2.Name || request(t, second, "GET", "/web/bootstrap", "", "", c1).StatusCode != 401 {
		t.Fatal("instance cookie not isolated")
	}
	if request(t, second, "POST", "/web/logout", `{}`, "self", c2).StatusCode != 204 {
		t.Fatal("logout response lost")
	}
	if request(t, second, "GET", "/web/bootstrap", "", "", c2).StatusCode != 401 {
		t.Fatal("logout retained auth")
	}
	c2 = login(t, second, "second-token")
	cfg.Enabled = false
	if err := second.Configure(cfg, ""); err != nil {
		t.Fatal(err)
	}
	cfg.Enabled = true
	if err := second.Configure(cfg, ""); err != nil {
		t.Fatal(err)
	}
	if request(t, second, "GET", "/web/bootstrap", "", "", c2).StatusCode != 401 {
		t.Fatal("disable retained auth")
	}
	// A failed atomic save keeps the running service and verifier intact.
	if err := os.Remove(filepath.Join(dir, "web_access.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "web_access.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := second.Configure(cfg, "cannot-save"); err == nil {
		t.Fatal("failed save hidden")
	}
	if !second.Status().Running || second.Status().Error == "" {
		t.Fatal("failure status missing")
	}
	login(t, second, "second-token")
	// Startup failure is visible and never turns into an unprotected listener.
	notReady := New()
	defer notReady.Close()
	notReady.Load(t.TempDir())
	if err := notReady.Configure(cfg, ""); err == nil || notReady.Status().Running || notReady.Status().Error == "" {
		t.Fatal("missing-assets listener failure hidden")
	}
}

func TestListenerScansOnlyAddressInUseAndRetainsListener(t *testing.T) {
	var first, second net.Listener
	for i := 0; i < 100; i++ {
		l, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := l.Addr().(*net.TCPAddr).Port
		if port > 65533 {
			l.Close()
			continue
		}
		next, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port+1))
		if err != nil {
			l.Close()
			continue
		}
		first, second = l, next
		break
	}
	if first == nil {
		t.Fatal("no consecutive ports")
	}
	defer first.Close()
	defer second.Close()
	port := first.Addr().(*net.TCPAddr).Port
	l, err := listenFrom("127.0.0.1", port, net.Listen)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if l.Addr().(*net.TCPAddr).Port != port+2 {
		t.Fatal(l.Addr())
	}
	if duplicate, err := net.Listen("tcp4", l.Addr().String()); err == nil {
		duplicate.Close()
		t.Fatal("bound listener was released")
	}
	for _, tc := range []struct {
		err   error
		count int
	}{{syscall.EACCES, 1}, {syscall.EADDRNOTAVAIL, 1}, {syscall.EADDRINUSE, 2}} {
		calls := 0
		_, err := listenFrom("127.0.0.1", 65534, func(string, string) (net.Listener, error) {
			calls++
			return nil, &net.OpError{Op: "listen", Err: tc.err}
		})
		if err == nil || calls != tc.count {
			t.Fatalf("%v: calls=%d, err=%v", tc.err, calls, err)
		}
	}
}

func TestFeatureUpgradeRevokedAndUnregistered(t *testing.T) {
	s, _ := testServer(t)
	s.Handle("/upgrade", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		fmt.Fprint(rw, "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: test\r\n\r\n")
		rw.Flush()
		io.Copy(io.Discard, conn)
	}))
	cfg := Config{Enabled: true, Host: "127.0.0.1", StartPort: freePort(t)}
	if err := s.Configure(cfg, ""); err != nil {
		t.Fatal(err)
	}
	c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", s.Status().ActualPort))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	fmt.Fprintf(c, "GET /upgrade HTTP/1.1\r\nHost: 127.0.0.1:%d\r\n\r\n", s.Status().ActualPort)
	reader := bufio.NewReader(c)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line == "\r\n" {
			break
		}
	}
	if err := s.Configure(cfg, "new-token"); err != nil {
		t.Fatal(err)
	}
	_, err = reader.ReadByte()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("upgrade not closed: %v", err)
	}
}
