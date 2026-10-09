package importer

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

// Fixed vectors generated independently using Node crypto, not the Go decoder.
// Parameters follow mRemoteNG AeadCryptographyProvider and Pkcs5S2KeyGenerator:
// salt 00..0f, nonce 10..1f, PBKDF2-SHA1, 32-byte key, salt as AAD, 16-byte tag.
// PKCS5 uses the low byte of each UTF-16 code unit of the configuration password.
func TestMRemoteNGPasswordDecryption(t *testing.T) {
	for _, tt := range []struct {
		name, master, ciphertext string
		iterations               int
	}{
		{"default", "", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh+J3sB/kCVgWKp6OFLyKAQRkwuQk3mUoCpl00MXK8RPBtIawhA=", 1000},
		{"custom", "synthetic-master", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh+Qh3aAn9oOgiIonbyJDj4nbBTZh/KqshOCAj9Npvp3vg/Be6s=", 5000},
		{"unicode", "m\u0101ster\U0001f511", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh/yHUmameLIEZPxcy6oxhpp+xCeQRptfv6J6zl2v03eusivOWY=", 1000},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data := fmt.Sprintf(`<Connections EncryptionEngine="AES" BlockCipherMode="GCM" KdfIterations="%d"><Node Name="parent" Type="Container" Password="%s"><Node Name="middle" Type="Container" InheritPassword="True"><Node Name="inherited" Type="Connection" Protocol="SSH2" Hostname="192.0.2.1" Password="ignored" InheritPassword="True" /><Node Name="empty" Type="Connection" Protocol="VNC" Password="" InheritPassword="False" /></Node></Node></Connections>`, tt.iterations, tt.ciphertext)
			res := mustParseMRemoteNG(t, data, ParseOptions{Password: tt.master})
			if len(res.Connections) != 2 || len(res.Warnings) != 0 {
				t.Fatal("expected successful password import")
			}
			if res.Connections[0].Password != "synthetic-secret-\u2713" || res.Connections[1].Password != "" {
				t.Fatal("incorrect password decryption or inheritance")
			}
			if tt.name == "custom" {
				for _, master := range []string{"wrong-master", ""} {
					failed := mustParseMRemoteNG(t, data, ParseOptions{Password: master})
					if len(failed.Connections) != 2 || failed.Connections[0].Password != "" || len(failed.Warnings) != 1 {
						t.Fatal("incorrect master password must leave credentials blank and preserve connections")
					}
					if master != "" && strings.Contains(failed.Warnings[0], master) {
						t.Fatal("warning exposed master password")
					}
				}
			}
		})
	}
}

func TestMRemoteNGPasswordAuthentication(t *testing.T) {
	original, err := base64.StdEncoding.DecodeString("AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh+J3sB/kCVgWKp6OFLyKAQRkwuQk3mUoCpl00MXK8RPBtIawhA=")
	if err != nil {
		t.Fatal(err)
	}
	for _, offset := range []int{0, 16, 32, len(original) - 1} {
		altered := append([]byte(nil), original...)
		altered[offset] ^= 1
		data := fmt.Sprintf(`<Connections EncryptionEngine="AES" BlockCipherMode="GCM" KdfIterations="1000"><Node Type="Connection" Protocol="SSH2" Password="%s" /></Connections>`, base64.StdEncoding.EncodeToString(altered))
		res := mustParseMRemoteNG(t, data, ParseOptions{})
		if len(res.Warnings) != 1 || res.Connections[0].Password != "" {
			t.Fatal("modified salt, nonce, ciphertext or tag must not authenticate")
		}
	}
	for _, length := range []int{1, 15, 16, 31, 32, 47} {
		data := fmt.Sprintf(`<Connections EncryptionEngine="AES" BlockCipherMode="GCM" KdfIterations="1000"><Node Type="Connection" Protocol="SSH2" Password="%s" /></Connections>`, base64.StdEncoding.EncodeToString(original[:length]))
		res := mustParseMRemoteNG(t, data, ParseOptions{})
		if len(res.Warnings) != 1 || res.Connections[0].Password != "" {
			t.Fatal("truncated password payload must not authenticate")
		}
	}
}
