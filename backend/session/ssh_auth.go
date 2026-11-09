package session

import (
	"os"

	"github.com/ys-ll/uniterm/backend/credentials"
	"github.com/ys-ll/uniterm/backend/utils"
	"golang.org/x/crypto/ssh"
)

func makeSSHAuthMethods(config ConnectionConfig, kbCallback ssh.KeyboardInteractiveChallenge) ([]ssh.AuthMethod, error) {
	methods, _, err := makeSSHAuthMethodsForAttempt(config, kbCallback)
	return methods, err
}

func makeSSHAuthMethodsForAttempt(config ConnectionConfig, kbCallback ssh.KeyboardInteractiveChallenge) ([]ssh.AuthMethod, func(), error) {
	var methods []ssh.AuthMethod
	cleanup := func() {}

	switch config.AuthType {
	case "password":
		// A still-encrypted (enc:v1) password means the vault is locked or the
		// load path didn't decrypt this field — never send ciphertext to the
		// server as a password. The keyboard-interactive fallback below still
		// gives the user a real prompt.
		if !credentials.IsEncrypted(config.Password) {
			methods = append(methods, ssh.Password(config.Password))
		}
	case "key", "keyText":
		if signer, ok := parseAuthKeySigner(config); ok {
			methods = append(methods, ssh.PublicKeys(signer))
		} else if pub, ok := publicKeyFromFile(config); ok {
			method, release, err := agentPublicKeyAuthMethod(pub, config.KeyPath)
			if err != nil {
				return nil, cleanup, err
			}
			cleanup = release
			methods = append(methods, method)
		}
	case "kerberos":
		method, release, err := kerberosAuthMethodWithCleanup(config.Host, config.KerberosRealm)
		if err != nil {
			return nil, cleanup, err
		}
		cleanup = release
		methods = append(methods, method)
	case "agent":
		method, release, err := localAgentAuthMethod()
		if err != nil {
			return nil, cleanup, err
		}
		cleanup = release
		methods = append(methods, method)
	}

	if config.TOTPEnabled && config.AuthType != "kerberos" {
		kbCallback = automaticTOTPChallenge(config, kbCallback)
	}

	// Kerberos and agent authentication must report their own errors instead
	// of silently falling through to an SSH password prompt.
	if kbCallback != nil && config.AuthType != "kerberos" && (config.AuthType != "agent" || config.TOTPEnabled) {
		methods = append(methods, ssh.KeyboardInteractive(kbCallback))
	}

	return methods, cleanup, nil
}

// buildAuthMethods returns the auth methods used by non-interactive SIP sessions
// (SFTP, server monitor) that dial via dialSSHTCP. It shares parsePrivateKeyFile
// with the interactive SSH session so an encrypted private key + its passphrase
// (config.Password) authenticates identically everywhere — the "秘钥加密码" case
// from issue #647. Unattended callers offer keyboard-interactive only when
// automatic TOTP is enabled; unknown questions fail without waiting for input.
func buildAuthMethods(config ConnectionConfig) ([]ssh.AuthMethod, error) {
	methods, _, err := buildAuthMethodsWithCleanup(config)
	return methods, err
}

func buildAuthMethodsWithCleanup(config ConnectionConfig) ([]ssh.AuthMethod, func(), error) {
	methods, cleanup, err := buildPrimaryAuthMethods(config)
	if err == nil && config.TOTPEnabled && config.AuthType != "kerberos" {
		methods = append(methods, ssh.KeyboardInteractive(automaticTOTPChallenge(config, nil)))
	}
	return methods, cleanup, err
}

func buildPrimaryAuthMethods(config ConnectionConfig) ([]ssh.AuthMethod, func(), error) {
	cleanup := func() {}
	switch config.AuthType {
	case "key", "keyText":
		signer, ok := parseAuthKeySigner(config)
		if !ok {
			if pub, isPub := publicKeyFromFile(config); isPub {
				method, release, err := agentPublicKeyAuthMethod(pub, config.KeyPath)
				if err != nil {
					return nil, cleanup, err
				}
				return []ssh.AuthMethod{method}, release, nil
			}
			return nil, cleanup, utils.UserErr("ssh_key_unavailable", keySourceLabel(config))
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, cleanup, nil
	case "kerberos":
		method, release, err := kerberosAuthMethodWithCleanup(config.Host, config.KerberosRealm)
		if err != nil {
			return nil, cleanup, err
		}
		return []ssh.AuthMethod{method}, release, nil
	case "agent":
		method, release, err := localAgentAuthMethod()
		if err != nil {
			return nil, cleanup, err
		}
		return []ssh.AuthMethod{method}, release, nil
	default: // "", "password" and any unknown type fall back to password
		// Unattended dial: a still-encrypted (enc:v1) password means the vault
		// is locked — there is no interactive prompt to fall back on, so fail
		// with the vault error instead of sending ciphertext as a password.
		if credentials.IsEncrypted(config.Password) {
			return nil, cleanup, utils.UserErr("vault_locked")
		}
		return []ssh.AuthMethod{ssh.Password(config.Password)}, cleanup, nil
	}
}

// parseAuthKeySigner returns the SSH signer for an authType of "key" or
// "keyText". "keyText" parses the inline PEM text from KeyContent directly;
// "key" reads the private-key file at KeyPath. The passphrase (config.Password)
// decrypts an encrypted key in both cases. Returns (nil, false) on any error;
// the caller falls back to other auth methods so the SSH handshake surfaces a
// meaningful error to the user.
func parseAuthKeySigner(config ConnectionConfig) (ssh.Signer, bool) {
	if config.AuthType == "keyText" {
		return parsePrivateKey([]byte(config.KeyContent), config.Password)
	}
	return parsePrivateKeyFile(config.KeyPath, config.Password)
}

// keySourceLabel names the key source for error messages — the file path for
// "key", or a friendly label for inline "keyText" content.
func keySourceLabel(config ConnectionConfig) string {
	if config.AuthType == "keyText" {
		return "inline private key text"
	}
	return config.KeyPath
}

// parsePrivateKeyFile reads the private key at path and parses it via
// parsePrivateKey. A leading "~" in path (as written in OpenSSH config
// IdentityFile lines) is expanded to the user's home directory first.
// Returns (nil, false) on any error.
func parsePrivateKeyFile(path, passphrase string) (ssh.Signer, bool) {
	key, err := os.ReadFile(utils.ExpandHomePath(path))
	if err != nil {
		return nil, false
	}
	return parsePrivateKey(key, passphrase)
}

// parsePrivateKey parses a private key from raw bytes, using passphrase when
// the key is encrypted. Returns (nil, false) on any error.
func parsePrivateKey(key []byte, passphrase string) (ssh.Signer, bool) {
	if isPPK(key) {
		signer, err := parsePPK(key, passphrase)
		return signer, err == nil
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
