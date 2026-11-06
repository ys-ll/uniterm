package session

import (
	"testing"

	"golang.org/x/crypto/ssh"
)

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
