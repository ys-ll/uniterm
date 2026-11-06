package session

import (
	"encoding/base32"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

const testTOTPSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

func TestTOTPCodeRFC6238(t *testing.T) {
	// RFC 6238 Appendix B SHA-1 vectors, reduced to the configured six digits.
	for _, tc := range []struct {
		seconds int64
		want    string
	}{
		{59, "287082"}, {1111111109, "081804"}, {1111111111, "050471"},
		{1234567890, "005924"}, {2000000000, "279037"}, {20000000000, "353130"},
	} {
		got, err := totpCode(testTOTPSecret, time.Unix(tc.seconds, 0))
		if err != nil || got != tc.want {
			t.Fatalf("TOTP(%d) = %q, %v; want %q", tc.seconds, got, err, tc.want)
		}
	}
	for _, secret := range []string{"", "invalid!secret", "enc:v1:secret", "AAAA"} {
		if _, err := totpCode(secret, time.Now()); err == nil {
			t.Fatal("expected invalid/locked secret error")
		}
	}
	padded := base32.StdEncoding.EncodeToString([]byte("12345678901"))
	if _, err := totpCode("  "+strings.ToLower(padded)+" \n", time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestTOTPChallengeMixedAndRejected(t *testing.T) {
	config := ConnectionConfig{AuthType: "password", Password: "saved", TOTPEnabled: true, TOTPSecret: testTOTPSecret}
	var prompts []string
	cb := automaticTOTPChallenge(config, func(_, _ string, q []string, e []bool) ([]string, error) {
		prompts = append(prompts, q...)
		answers := make([]string, len(q))
		for i := range answers {
			answers[i] = "manual"
		}
		return answers, nil
	})
	answers, err := cb("", "", []string{"Password:", "[OTP Code]:", "Security question:"}, []bool{false, false, false})
	if err != nil || answers[0] != "saved" || len(answers[1]) != 6 || answers[2] != "manual" {
		t.Fatalf("mixed answers: %v, %v", answers, err)
	}
	if len(prompts) != 1 || prompts[0] != "Security question:" {
		t.Fatalf("unexpected manual questions: %v", prompts)
	}
	answers, err = cb("", "", []string{"Verification Code:"}, []bool{false})
	if err != nil || answers[0] != "manual" {
		t.Fatalf("rejected OTP must go manual: %v, %v", answers, err)
	}
}

func TestTOTPChallengePromptSafety(t *testing.T) {
	for _, question := range []string{"[OTP Code]:", "Verification Code:", "One-Time Password:", "TOTP:", "Authentication code:", "Authenticator code:"} {
		config := ConnectionConfig{AuthType: "password", Password: "saved", TOTPEnabled: true, TOTPSecret: testTOTPSecret}
		if isSavedPasswordChallenge(config, []string{question}, []bool{false}) {
			t.Fatalf("password would be sent to %q", question)
		}
		answers, err := automaticTOTPChallenge(config, nil)("", "", []string{question}, []bool{false})
		if err != nil || len(answers) != 1 || len(answers[0]) != 6 {
			t.Fatalf("OTP prompt %q: %v, %v", question, answers, err)
		}
	}
	config := ConnectionConfig{AuthType: "key", Password: "key-passphrase", TOTPEnabled: true, TOTPSecret: testTOTPSecret}
	for _, tc := range []struct {
		q string
		e []bool
	}{{"Password:", []bool{false}}, {"Name:", []bool{false}}, {"OTP:", []bool{true}}, {"OTP:", nil}} {
		if _, err := automaticTOTPChallenge(config, nil)("", "", []string{tc.q}, tc.e); err == nil {
			t.Fatalf("unsafe challenge accepted: %q", tc.q)
		}
	}
}

func TestTOTPSSHAuthentication(t *testing.T) {
	server := &ssh.ServerConfig{KeyboardInteractiveCallback: func(conn ssh.ConnMetadata, challenge ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
		// Both fields in one challenge models PAM password + OTP authentication.
		answers, err := challenge(conn.User(), "", []string{"Password:", "[OTP Code]:"}, []bool{false, false})
		if err != nil {
			return nil, err
		}
		now := time.Now()
		valid := false
		for _, delta := range []time.Duration{-30 * time.Second, 0, 30 * time.Second} {
			code, _ := totpCode(testTOTPSecret, now.Add(delta))
			valid = valid || answers[1] == code
		}
		if answers[0] != "saved" || !valid {
			return nil, fmt.Errorf("bad credentials")
		}
		return nil, nil
	}}
	addr := startAuthTestServer(t, server)
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portText)
	config := ConnectionConfig{Type: "ssh", Host: host, Port: port, User: "test", AuthType: "password", Password: "saved", TOTPEnabled: true, TOTPSecret: testTOTPSecret}
	t.Run("connection test", func(t *testing.T) {
		if _, err := probeSSH(config); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("unattended transfer auth", func(t *testing.T) {
		methods, cleanup, err := buildAuthMethodsWithCleanup(config)
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()
		client, err := ssh.Dial("tcp", addr, &ssh.ClientConfig{User: "test", Auth: methods, HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 5 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		client.Close()
	})
	t.Run("disabled", func(t *testing.T) {
		config.TOTPEnabled = false
		if _, err := probeSSH(config); err == nil {
			t.Fatal("disabled OTP authenticated")
		}
	})
}
