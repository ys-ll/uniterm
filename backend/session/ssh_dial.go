package session

import (
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type sshConnDialer func() (net.Conn, error)
type sshClientConfigFactory func() (*ssh.ClientConfig, func(), error)

// dialSSHWithCipherFallback keeps modern AEAD preference, but retries a
// handshake EOF once with CTR first for servers that falsely advertise GCM.
// The algorithm sets are resolved per connection (see resolveSSHAlgorithms)
// and tried in order; only a first-attempt EOF advances to the next set.
func dialSSHWithCipherFallback(addr string, sets []sshAlgoSet, newConfig sshClientConfigFactory, dial sshConnDialer) (*ssh.Client, error) {
	var lastErr error
	for i, set := range sets {
		conn, err := dial()
		if err != nil {
			return nil, fmt.Errorf("tcp dial: %w", err)
		}
		config, cleanup, err := newConfig()
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("ssh auth: %w", err)
		}
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			tcpConn.SetKeepAlive(true)
			tcpConn.SetKeepAlivePeriod(sshKeepAliveInterval)
		}
		attempt := *config
		attempt.Config = set.config()
		attempt.HostKeyAlgorithms = set.HostKeys
		sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, &attempt)
		cleanup()
		if err == nil {
			return ssh.NewClient(sshConn, chans, reqs), nil
		}
		conn.Close()
		lastErr = err
		if i == 0 && errors.Is(err, io.EOF) {
			continue
		}
		break
	}
	return nil, fmt.Errorf("ssh handshake: %w", lastErr)
}

// Markers within x/crypto's error strings. The library returns both as plain
// fmt.Errorf values with no typed sentinel, so the message is the only
// stable handle.
const (
	authExhaustedMarker  = "unable to authenticate"
	kbdIntColdFailMarker = "unexpected message type 51 (expected 60)"
)

// dialSSHWithAuthRetry offers keyboard-interactive alongside the primary
// methods (password/key) on the FIRST handshake, and retries once without it.
//
// Keeping keyboard-interactive in the first handshake means prompt-based
// logins (OTP/2FA, keyboard-interactive-only servers) and a password rejection
// followed by an interactive prompt complete in a single TCP handshake — with
// one handshake per failed attempt, every wrong password paid a full extra
// dial plus a second server-side fail delay before the user got to retype
// (issue #949).
//
// x/crypto surfaces a server that answers the keyboard-interactive initiation
// with USERAUTH_FAILURE before any info request as "ssh: unexpected message
// type 51 (expected 60)". RFC 4252 allows that reply and OpenSSH's own client
// treats it as a plain rejection, but the library aborts the handshake with
// that cryptic protocol error. Servers that fail keyboard-interactive cold are
// common (root login restricted to keys, dropbear without PAM), so the retry
// drops keyboard-interactive; its plain rejection is the honest auth failure
// the user should see.
func dialSSHWithAuthRetry(addr string, sets []sshAlgoSet, authConfig, fallbackConfig sshClientConfigFactory, dial sshConnDialer) (*ssh.Client, error) {
	if authConfig == nil {
		authConfig = fallbackConfig
	}
	client, err := dialSSHWithCipherFallback(addr, sets, authConfig, dial)
	if err == nil || !strings.Contains(err.Error(), kbdIntColdFailMarker) {
		return client, err
	}
	client, retryErr := dialSSHWithCipherFallback(addr, sets, fallbackConfig, dial)
	if retryErr == nil {
		return client, nil
	}
	return nil, retryErr
}

// dialSSHTCP dials addr (through the upstream proxy when non-nil), performs the
// SSH handshake, and returns a *ssh.Client. Used by the SFTP and monitor
// sessions, which previously dialed directly via ssh.Dial.
func dialSSHTCP(addr string, clientConfig *ssh.ClientConfig, upstream *SocksProxy) (*ssh.Client, error) {
	raw, err := dialFirstHop(addr, upstream)
	if err != nil {
		return nil, fmt.Errorf("tcp dial: %w", err)
	}
	conn, chans, reqs, err := ssh.NewClientConn(raw, addr, clientConfig)
	if err != nil {
		raw.Close()
		return nil, fmt.Errorf("ssh handshake: %w", err)
	}
	return ssh.NewClient(conn, chans, reqs), nil
}

// DialSSHClient 建立非交互 SSH 连接（容器 runner 用，无 PTY、无键盘交互回显）。
// 与 ssh_session 的交互式拨号共享认证与密钥交换配置。启用 TOTP 时，
// keyboard-interactive 自动应答已知提示；其他问题直接失败，不等待手动输入。
func DialSSHClient(config ConnectionConfig) (*ssh.Client, error) {
	addr := net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
	newConfig := func(challenge ssh.KeyboardInteractiveChallenge) sshClientConfigFactory {
		return func() (*ssh.ClientConfig, func(), error) {
			authMethods, cleanup, err := makeSSHAuthMethodsForAttempt(config, challenge)
			if err != nil {
				return nil, nil, err
			}
			return &ssh.ClientConfig{
				User:            config.User,
				Auth:            authMethods,
				Timeout:         30 * time.Second,
				HostKeyCallback: HostKeyCallback(config.Host, strconv.Itoa(config.Port), ""),
			}, cleanup, nil
		}
	}
	sets, err := resolveSSHDialAlgorithms(config.SSHAlgorithms)
	if err != nil {
		return nil, err
	}
	factory := newConfig(nil)
	return dialSSHWithAuthRetry(addr, sets, factory, factory, func() (net.Conn, error) {
		return net.DialTimeout("tcp", addr, 30*time.Second)
	})
}

// sshAlgoModeOf reports the effective mode of a connection's algorithm
// preferences, used for error annotation.
func sshAlgoModeOf(p *SSHAlgoConfig) string {
	if p == nil || p.Mode == "" {
		return SSHAlgoModeCompatible
	}
	return p.Mode
}

// resolveSSHDialAlgorithms returns the algorithm sets tried in order on a
// dial path: the connection's resolved base set, followed by the EOF-retry
// set when it differs from the base.
func resolveSSHDialAlgorithms(p *SSHAlgoConfig) ([]sshAlgoSet, error) {
	base, retry, err := resolveSSHAlgorithms(p)
	if err != nil {
		return nil, err
	}
	if slices.Equal(base.Ciphers, retry.Ciphers) && slices.Equal(base.KeyExchanges, retry.KeyExchanges) &&
		slices.Equal(base.MACs, retry.MACs) && slices.Equal(base.HostKeys, retry.HostKeys) {
		return []sshAlgoSet{base}, nil
	}
	return []sshAlgoSet{base, retry}, nil
}
