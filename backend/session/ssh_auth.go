package session

import (
	"os"

	"golang.org/x/crypto/ssh"

	"github.com/ys-ll/uniterm/backend/log"
)

func makeSSHAuthMethods(config ConnectionConfig, kbCallback ssh.KeyboardInteractiveChallenge) []ssh.AuthMethod {
	var methods []ssh.AuthMethod

	switch config.AuthType {
	case "password":
		methods = append(methods, ssh.Password(config.Password))
	case "key":
		signer, err := parsePrivateKeyFile(config.KeyPath, config.Password)
		if err != nil {
			// Log the parse error so a bad key path or wrong passphrase is
			// observable — previously the bool return hid every parse failure
			// and the only visible symptom was a confusing SSH handshake error.
			log.Writef("ssh: key auth skipped, parse %s failed: %v", config.KeyPath, err)
			break
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	// Keyboard-interactive as fallback for password-less or failed-password scenarios.
	if kbCallback != nil {
		methods = append(methods, ssh.KeyboardInteractive(kbCallback))
	}

	return methods
}

// parsePrivateKeyFile reads the private key at path and parses it, using
// passphrase when the key is encrypted. Returns the underlying error from
// os.ReadFile / ssh.ParsePrivateKey* so the caller can surface a
// meaningful diagnostic (e.g. wrong passphrase, unsupported key format).
func parsePrivateKeyFile(path, passphrase string) (ssh.Signer, error) {
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if passphrase != "" {
		return ssh.ParsePrivateKeyWithPassphrase(key, []byte(passphrase))
	}
	return ssh.ParsePrivateKey(key)
}
