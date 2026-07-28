package session

import (
	"fmt"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

// DialSSHClient 建立非交互 SSH 连接（容器 runner 用，无 PTY、无键盘交互回显）。
// 与 ssh_session 的交互式拨号共享认证与密钥交换配置。
func DialSSHClient(config ConnectionConfig) (*ssh.Client, error) {
	kb := func(user, instruction string, questions []string, echos []bool) ([]string, error) {
		return nil, fmt.Errorf("keyboard-interactive not supported in this context")
	}
	authMethods := makeSSHAuthMethods(config, kb)
	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	clientConfig := &ssh.ClientConfig{
		User:            config.User,
		Auth:            authMethods,
		Timeout:         30 * time.Second,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Config:          ssh.Config{KeyExchanges: sshKeyExchanges()},
	}
	conn, err := net.DialTimeout("tcp", addr, clientConfig.Timeout)
	if err != nil {
		return nil, fmt.Errorf("tcp dial: %w", err)
	}
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(sshKeepAliveInterval)
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, clientConfig)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("ssh handshake: %w", err)
	}
	return ssh.NewClient(sshConn, chans, reqs), nil
}
