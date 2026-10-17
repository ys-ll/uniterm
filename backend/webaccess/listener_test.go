package webaccess

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net"
	"testing"
)

// Reserve the whole fixture range before releasing the two server slots.
func holdListenerPorts(t *testing.T, host string) []net.Listener {
	t.Helper()
	for attempt := 0; attempt < 100; attempt++ {
		// Keep the fixture outside default ephemeral ranges: concurrent browser
		// client connections can claim released ephemeral slots before Configure.
		port := 20000 + rand.IntN(10000)
		first, err := net.Listen("tcp4", fmt.Sprintf("%s:%d", host, port))
		if err != nil {
			if addressInUse(err) {
				continue
			}
			t.Fatal(err)
		}
		listeners := []net.Listener{first}
		for offset := 1; offset < 4; offset++ {
			var l net.Listener
			if offset == 1 {
				l, err = net.Listen("tcp4", fmt.Sprintf("%s:%d", host, port+offset))
			} else {
				// A reusable bind can succeed over TIME_WAIT or another
				// interface; reserve genuinely free slots for the new servers.
				l, err = listenExclusive("tcp4", fmt.Sprintf("0.0.0.0:%d", port+offset))
			}
			if err != nil {
				break
			}
			listeners = append(listeners, l)
		}
		if len(listeners) == 4 {
			t.Cleanup(func() {
				for _, l := range listeners {
					_ = l.Close()
				}
			})
			return listeners
		}
		for _, l := range listeners {
			_ = l.Close()
		}
	}
	t.Fatal("could not reserve four consecutive TCP ports")
	return nil
}

func TestListenerExcludesWildcardLoopbackOverlap(t *testing.T) {
	for _, tc := range []struct{ occupied, host string }{
		{"127.0.0.1", "0.0.0.0"},
		{"0.0.0.0", "127.0.0.1"},
	} {
		t.Run(tc.occupied+"_then_"+tc.host, func(t *testing.T) {
			held := holdListenerPorts(t, tc.occupied)
			port := held[0].Addr().(*net.TCPAddr).Port
			_ = held[2].Close()
			_ = held[3].Close()
			first, _ := testServer(t)
			second, _ := testServer(t)
			servers := []*Server{first, second}
			cfg := Config{Enabled: true, Host: tc.host, StartPort: port}
			for i, s := range servers {
				if err := s.Configure(cfg, "listener-test-token"); err != nil {
					t.Fatal(err)
				}
				if st := s.Status(); !st.Running || st.ActualPort != port+2+i {
					t.Fatalf("listener claimed an occupied port or skipped a free one: %+v; want %d", st, port+2+i)
				}
				// The returned socket must remain bound against another instance
				// using either address form.
				for _, host := range []string{tc.host, tc.occupied} {
					if l, err := listenExclusive("tcp4", fmt.Sprintf("%s:%d", host, port+2+i)); err == nil {
						_ = l.Close()
						t.Fatalf("running listener allowed another bind on %s:%d", host, port+2+i)
					}
				}
			}
			for i, s := range servers {
				// Check release before opening HTTP connections, so TCP TIME_WAIT
				// from an accepted connection cannot mask a leaked listener.
				disabled := cfg
				disabled.Enabled = false
				if err := s.Configure(disabled, ""); err != nil {
					t.Fatal(err)
				}
				for _, host := range []string{tc.host, tc.occupied} {
					l, err := listenExclusive("tcp4", fmt.Sprintf("%s:%d", host, port+2+i))
					if err != nil {
						t.Fatalf("disabled listener leaked its port: %v", err)
					}
					_ = l.Close()
				}
				if err := s.Configure(cfg, ""); err != nil {
					t.Fatal(err)
				}
				if st := s.Status(); st.ActualPort != port+2+i || st.URLs[0] != fmt.Sprintf("http://127.0.0.1:%d", port+2+i) {
					t.Fatalf("same starting port did not reclaim the released slot: %+v", st)
				}
			}
			for _, s := range servers {
				cookie := login(t, s, "listener-test-token")
				response := request(t, s, "GET", "/web/bootstrap", "", "", cookie)
				var bootstrap struct {
					InstanceID string `json:"instanceId"`
				}
				if err := json.NewDecoder(response.Body).Decode(&bootstrap); err != nil {
					t.Fatal(err)
				}
				_ = response.Body.Close()
				if response.StatusCode != 200 || bootstrap.InstanceID != s.Status().InstanceID {
					t.Fatalf("advertised address reached a different listener: %+v", bootstrap)
				}
			}
		})
	}
}
