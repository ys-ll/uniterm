package k8s

import "testing"

// QA-023: fuzz tests for low-level parsers.
//
// Each fuzz is a thin wrapper that pins the safety contract:
// "any input, no panic, no hang, returns within maxCSIParam-bound."
// Run with `go test -fuzz=FuzzParseBytes -fuzztime=10s ./backend/k8s/`
// for a real fuzz campaign; CI can default to running just the seed
// corpus so the test stays fast.

func FuzzParseBytes(f *testing.F) {
	// Seed corpus with the canonical kubeconfig shape plus a few
	// adversarial inputs.
	f.Add([]byte(`apiVersion: v1
kind: Config
current-context: a
contexts:
- name: a
  context: {cluster: c1, user: u1}
clusters:
- name: c1
  cluster: {server: http://example.com}
users:
- name: u1
  user: {token: x}
`))
	f.Add([]byte(""))
	f.Add([]byte("{ not yaml"))
	f.Add([]byte("---"))
	f.Add([]byte("a\x00b"))           // NUL bytes
	f.Add([]byte("\xff\xfe\xfd\xfc")) // invalid UTF-8

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must NOT panic; result can be nil.
		_, _ = ParseBytes(data)
	})
}

func FuzzParseServerAddr(f *testing.F) {
	f.Add("host:80")
	f.Add("host")
	f.Add("host:abc")
	f.Add("[]:80")
	f.Add("user@host:22")
	f.Add(":")          // pathological: empty host + empty port
	f.Add(":80")        // empty host
	f.Add("host:")      // empty port
	f.Add("host:0")     // port zero
	f.Add("host:65535") // max port
	f.Add("host:65536") // out of range
	f.Add("a:b:c")      // multiple colons
	f.Add("[]:bad")     // bracketed but invalid port

	f.Fuzz(func(t *testing.T, s string) {
		host, port, err := ParseServerAddr(s)
		if err != nil {
			return
		}
		if port < 0 || port > 65535 {
			t.Errorf("port out of range: %d from %q", port, s)
		}
		_ = host
	})
}
