package session

import "golang.org/x/crypto/ssh"

// sshKeyExchanges returns the KEX algorithm list for SSH connections,
// including legacy algorithms for compatibility with older servers.
func sshKeyExchanges() []string {
	return []string{
		// Default safe algorithms
		"mlkem768x25519-sha256",
		"curve25519-sha256",
		"curve25519-sha256@libssh.org",
		"ecdh-sha2-nistp256",
		"ecdh-sha2-nistp384",
		"ecdh-sha2-nistp521",
		"diffie-hellman-group14-sha256",
		"diffie-hellman-group16-sha512",
		"diffie-hellman-group-exchange-sha256",
		// Legacy algorithms for old servers (issue #208)
		ssh.InsecureKeyExchangeDH14SHA1,
		ssh.InsecureKeyExchangeDHGEXSHA1,
		ssh.InsecureKeyExchangeDH1SHA1,
	}
}
