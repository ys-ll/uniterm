package session

import (
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestResolveSSHAlgorithmsDefaultsToCompatible(t *testing.T) {
	base, retry, err := resolveSSHAlgorithms(nil)
	if err != nil {
		t.Fatalf("resolveSSHAlgorithms(nil): %v", err)
	}
	if !containsSSHAlgo(base.Ciphers, ssh.InsecureCipherAES128CBC) ||
		!containsSSHAlgo(base.Ciphers, ssh.InsecureCipherAES192CBC) ||
		!containsSSHAlgo(base.Ciphers, ssh.InsecureCipherAES256CBC) ||
		!containsSSHAlgo(base.Ciphers, ssh.InsecureCipherTripleDESCBC) {
		t.Fatalf("compatible ciphers missing legacy CBC family: %v", base.Ciphers)
	}
	if !containsSSHAlgo(base.HostKeys, ssh.KeyAlgoRSA) {
		t.Fatalf("compatible host keys missing plain RSA: %v", base.HostKeys)
	}
	if !containsSSHAlgo(base.KeyExchanges, ssh.InsecureKeyExchangeDH14SHA1) {
		t.Fatalf("compatible KEX missing SHA-1 group14: %v", base.KeyExchanges)
	}
	// The EOF retry set must differ only in cipher ordering.
	if !containsSSHAlgo(retry.Ciphers, ssh.CipherAES128CTR) || retry.Ciphers[0] != ssh.CipherAES128CTR {
		t.Fatalf("retry set should lead with CTR ciphers: %v", retry.Ciphers)
	}
}

func TestResolveSSHAlgorithmsSecureExcludesLegacy(t *testing.T) {
	base, _, err := resolveSSHAlgorithms(&SSHAlgoConfig{Mode: SSHAlgoModeSecure})
	if err != nil {
		t.Fatalf("resolveSSHAlgorithms(secure): %v", err)
	}
	legacy := []string{
		ssh.InsecureCipherAES128CBC, ssh.InsecureCipherAES192CBC, ssh.InsecureCipherAES256CBC,
		ssh.InsecureCipherTripleDESCBC, ssh.HMACSHA1, ssh.InsecureHMACSHA196,
		ssh.InsecureKeyExchangeDH14SHA1, ssh.InsecureKeyExchangeDHGEXSHA1, ssh.InsecureKeyExchangeDH1SHA1,
		ssh.KeyAlgoRSA, ssh.InsecureKeyAlgoDSA,
	}
	for _, algo := range legacy {
		if containsSSHAlgo(base.Ciphers, algo) || containsSSHAlgo(base.MACs, algo) ||
			containsSSHAlgo(base.KeyExchanges, algo) || containsSSHAlgo(base.HostKeys, algo) {
			t.Fatalf("secure mode offers legacy algorithm %q", algo)
		}
	}
	if len(base.Ciphers) == 0 || len(base.KeyExchanges) == 0 || len(base.HostKeys) == 0 {
		t.Fatalf("secure mode must keep a usable algorithm set: %+v", base)
	}
}

func TestResolveSSHAlgorithmsCustom(t *testing.T) {
	p := &SSHAlgoConfig{
		Mode:         SSHAlgoModeCustom,
		KeyExchanges: []string{ssh.InsecureKeyExchangeDH1SHA1},
		Ciphers:      []string{ssh.InsecureCipherAES256CBC},
		MACs:         []string{ssh.HMACSHA1},
		HostKeys:     []string{ssh.KeyAlgoRSA},
	}
	base, retry, err := resolveSSHAlgorithms(p)
	if err != nil {
		t.Fatalf("resolveSSHAlgorithms(custom): %v", err)
	}
	if len(base.Ciphers) != 1 || base.Ciphers[0] != ssh.InsecureCipherAES256CBC {
		t.Fatalf("custom ciphers = %v, want the configured single entry", base.Ciphers)
	}
	if !slicesEqual(base, retry) {
		t.Fatalf("custom mode must not get an implicit retry variant")
	}

	invalid := []SSHAlgoConfig{
		{Mode: SSHAlgoModeCustom},
		{Mode: SSHAlgoModeCustom, KeyExchanges: []string{"made-up-kex"}, Ciphers: []string{ssh.CipherAES128CTR}, MACs: []string{ssh.HMACSHA256}, HostKeys: []string{ssh.KeyAlgoED25519}},
		{Mode: SSHAlgoModeCustom, KeyExchanges: []string{ssh.KeyExchangeCurve25519}, Ciphers: []string{"made-up-cipher"}, MACs: []string{ssh.HMACSHA256}, HostKeys: []string{ssh.KeyAlgoED25519}},
		{Mode: SSHAlgoModeCustom, KeyExchanges: []string{ssh.KeyExchangeCurve25519}, Ciphers: []string{ssh.CipherAES128CTR}, MACs: []string{ssh.HMACSHA256}, HostKeys: []string{"made-up-hostkey"}},
		{Mode: "bogus"},
	}
	for i := range invalid {
		if _, _, err := resolveSSHAlgorithms(&invalid[i]); err == nil {
			t.Fatalf("resolveSSHAlgorithms(%+v) = nil error, want rejection", invalid[i])
		}
	}
}

func TestResolveSSHDialAlgorithms(t *testing.T) {
	// Custom mode yields exactly one attempt (no implicit cipher reorder).
	sets, err := resolveSSHDialAlgorithms(&SSHAlgoConfig{Mode: SSHAlgoModeCustom, KeyExchanges: []string{ssh.KeyExchangeCurve25519}, Ciphers: []string{ssh.CipherAES128CTR}, MACs: []string{ssh.HMACSHA256}, HostKeys: []string{ssh.KeyAlgoED25519}})
	if err != nil {
		t.Fatalf("resolveSSHDialAlgorithms(custom): %v", err)
	}
	if len(sets) != 1 {
		t.Fatalf("custom sets = %d, want 1", len(sets))
	}
	// Compatible mode yields base + CTR-first retry.
	sets, err = resolveSSHDialAlgorithms(nil)
	if err != nil {
		t.Fatalf("resolveSSHDialAlgorithms(nil): %v", err)
	}
	if len(sets) != 2 {
		t.Fatalf("compatible sets = %d, want 2", len(sets))
	}
	if sshAlgoModeOf(nil) != SSHAlgoModeCompatible || sshAlgoModeOf(&SSHAlgoConfig{}) != SSHAlgoModeCompatible {
		t.Fatalf("empty preferences must map to compatible mode")
	}
}

func slicesEqual(a, b sshAlgoSet) bool {
	eq := func(x, y []string) bool {
		if len(x) != len(y) {
			return false
		}
		for i := range x {
			if x[i] != y[i] {
				return false
			}
		}
		return true
	}
	return eq(a.Ciphers, b.Ciphers) && eq(a.KeyExchanges, b.KeyExchanges) && eq(a.MACs, b.MACs) && eq(a.HostKeys, b.HostKeys)
}

// startAlgoTestServer runs a local SSH server whose negotiated algorithms are
// restricted to the given config, returning the listener address.
func startAlgoTestServer(t *testing.T, serverAlgos ssh.Config) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("build signer: %v", err)
	}
	serverConfig := &ssh.ServerConfig{
		NoClientAuth: true,
	}
	serverConfig.Config = serverAlgos
	serverConfig.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, chans, reqs, err := ssh.NewServerConn(c, serverConfig)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)
				for ch := range chans {
					ch.Reject(ssh.UnknownChannelType, "test server")
				}
			}(conn)
		}
	}()
	return listener.Addr().String()
}

func dialWithAlgoSet(t *testing.T, addr string, set sshAlgoSet) error {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	cfg := &ssh.ClientConfig{
		User:            "test",
		Timeout:         5 * time.Second,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	set.apply(cfg)
	sshConn, _, _, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		return err
	}
	sshConn.Close()
	return nil
}

// A server that only speaks aes256-cbc must be reachable in compatible mode
// and must fail negotiation in secure mode.
func TestHandshakeAESCBCOnlyServer(t *testing.T) {
	addr := startAlgoTestServer(t, ssh.Config{
		KeyExchanges: []string{ssh.KeyExchangeDH14SHA256},
		Ciphers:      []string{ssh.InsecureCipherAES256CBC},
		MACs:         []string{ssh.HMACSHA256},
	})

	base, _, err := resolveSSHAlgorithms(nil)
	if err != nil {
		t.Fatalf("resolve compatible: %v", err)
	}
	if err := dialWithAlgoSet(t, addr, base); err != nil {
		t.Fatalf("compatible mode handshake against aes256-cbc-only server: %v", err)
	}

	secure, _, err := resolveSSHAlgorithms(&SSHAlgoConfig{Mode: SSHAlgoModeSecure})
	if err != nil {
		t.Fatalf("resolve secure: %v", err)
	}
	err = dialWithAlgoSet(t, addr, secure)
	if err == nil || !strings.Contains(err.Error(), "no common algorithm") {
		t.Fatalf("secure mode error = %v, want algorithm mismatch", err)
	}
	wrapped := wrapSSHAlgorithmError(SSHAlgoModeCompatible, err)
	if !strings.Contains(wrapped.Error(), "custom") {
		t.Fatalf("wrapped error = %v, want guidance", wrapped)
	}
}

func TestWrapSSHAlgorithmErrorPassthrough(t *testing.T) {
	plain := io.EOF
	if got := wrapSSHAlgorithmError(SSHAlgoModeCompatible, plain); got != plain {
		t.Fatalf("unrelated error must pass through unchanged")
	}
}


func TestSavedPasswordChallenge(t *testing.T) {
	cases := []struct {
		name      string
		config    ConnectionConfig
		questions []string
		echos     []bool
		want      bool
	}{
		{"single hidden password", ConnectionConfig{AuthType: "password", Password: "pw"}, []string{"Password:"}, []bool{false}, true},
		{"empty auth defaults to password", ConnectionConfig{Password: "pw"}, []string{"Password:"}, []bool{false}, true},
		{"OTP prompt", ConnectionConfig{AuthType: "password", Password: "pw"}, []string{"[OTP Code]:"}, []bool{false}, false},
		{"one time password", ConnectionConfig{AuthType: "password", Password: "pw"}, []string{"One-Time Password:"}, []bool{false}, false},
		{"unknown hidden prompt", ConnectionConfig{AuthType: "password", Password: "pw"}, []string{"Security answer:"}, []bool{false}, false},
		{"locked password", ConnectionConfig{AuthType: "password", Password: "enc:v1:locked"}, []string{"Password:"}, []bool{false}, false},
		{"private key passphrase", ConnectionConfig{AuthType: "key", Password: "passphrase"}, []string{"Password:"}, []bool{false}, false},
		{"password plus otp", ConnectionConfig{AuthType: "password", Password: "pw"}, []string{"Password:", "OTP:"}, []bool{false, false}, false},
		{"visible challenge", ConnectionConfig{AuthType: "password", Password: "pw"}, []string{"Name:"}, []bool{true}, false},
		{"missing echo metadata", ConnectionConfig{AuthType: "password", Password: "pw"}, []string{"Password:"}, nil, false},
		{"no saved password", ConnectionConfig{AuthType: "password"}, []string{"Password:"}, []bool{false}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSavedPasswordChallenge(tc.config, tc.questions, tc.echos); got != tc.want {
				t.Fatalf("isSavedPasswordChallenge() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSSHCipherPreferences(t *testing.T) {
	modern := sshCiphers()
	legacyRetry := sshCiphersCTRFirst()
	if modern[0] != ssh.CipherAES128GCM {
		t.Fatalf("default first cipher = %q, want %q", modern[0], ssh.CipherAES128GCM)
	}
	if legacyRetry[0] != ssh.CipherAES128CTR {
		t.Fatalf("retry first cipher = %q, want %q", legacyRetry[0], ssh.CipherAES128CTR)
	}
}
