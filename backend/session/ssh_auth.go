package session

import (
	"os"

	"golang.org/x/crypto/ssh"
)

func makeSSHAuthMethods(config ConnectionConfig, kbCallback ssh.KeyboardInteractiveChallenge) []ssh.AuthMethod {
	var methods []ssh.AuthMethod

	switch config.AuthType {
	case "password":
		methods = append(methods, ssh.Password(config.Password))
	case "key":
		if signer, ok := parsePrivateKeyFile(config.KeyPath, config.Password); ok {
			methods = append(methods, ssh.PublicKeys(signer))
		}
	}

	// Keyboard-interactive as fallback for password-less or failed-password scenarios.
	if kbCallback != nil {
		methods = append(methods, ssh.KeyboardInteractive(kbCallback))
	}

	return methods
}

// parsePrivateKeyFile reads the private key at path and parses it, using
// passphrase when the key is encrypted. Returns (nil, false) on any error;
// the caller is expected to fall back to other auth methods so the SSH
// handshake surfaces a meaningful error to the user.
func parsePrivateKeyFile(path, passphrase string) (ssh.Signer, bool) {
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	if passphrase != "" {
		signer, err := ssh.ParsePrivateKeyWithPassphrase(key, []byte(passphrase))
		if err != nil {
			return nil, false
		}
		return signer, true
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, false
	}
	return signer, true
}
