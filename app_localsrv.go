package main

// app_localsrv.go exposes the Local Services tool (HTTP / TFTP / SSH-SFTP /
// FTP / Telnet / NFS servers) to the frontend. Each server runs asynchronously;
// log lines and state changes stream back as "localsrv:*" events carrying the
// server id returned by the start call.
//
// All StartXxxServer calls accept timeoutSeconds: when >0 the manager arms
// an auto-stop timer so the server shuts itself down after that many seconds.
// Pass 0 to disable auto-stop (the default).

import (
	"github.com/ys-ll/uniterm/backend/localsrv"
)

// StartHTTPServer starts an HTTP file server serving rootDir on port.
// When listDir is true, directory contents are listed. Returns the server id.
func (a *App) StartHTTPServer(rootDir string, port int, listDir bool, timeoutSeconds int) (string, error) {
	return a.localsrvManager.StartHTTPServer(localsrv.HTTPConfig{
		RootDir:        rootDir,
		Port:           port,
		ListDir:        listDir,
		TimeoutSeconds: timeoutSeconds,
	})
}

// StartTFTPServer starts a TFTP server serving rootDir on UDP port.
// blockSize overrides the default 512-byte block size. Returns the server id.
func (a *App) StartTFTPServer(rootDir string, port, blockSize int, timeoutSeconds int) (string, error) {
	return a.localsrvManager.StartTFTPServer(localsrv.TFTPConfig{
		RootDir:        rootDir,
		Port:           port,
		BlockSize:      blockSize,
		TimeoutSeconds: timeoutSeconds,
	})
}

// StartSSHServer starts an SSH server serving rootDir on port: an interactive
// local shell plus an SFTP subsystem restricted to rootDir. Authentication is
// by username/password; both are required. Returns the server id.
func (a *App) StartSSHServer(rootDir string, port int, username, password string, timeoutSeconds int) (string, error) {
	return a.localsrvManager.StartSSHServer(localsrv.SSHConfig{
		RootDir:        rootDir,
		Port:           port,
		Username:       username,
		Password:       password,
		TimeoutSeconds: timeoutSeconds,
	})
}

// StartFTPServer starts an FTP server serving rootDir on port.
// When anonymous is true, any login is accepted. Returns the server id.
func (a *App) StartFTPServer(rootDir string, port int, username, password string, anonymous, utf8 bool, timeoutSeconds int) (string, error) {
	return a.localsrvManager.StartFTPServer(localsrv.FTPConfig{
		RootDir:        rootDir,
		Port:           port,
		Username:       username,
		Password:       password,
		Anonymous:      anonymous,
		UTF8:           utf8,
		TimeoutSeconds: timeoutSeconds,
	})
}

// StopLocalServer stops a running local server by id. A no-op for unknown ids.
func (a *App) StopLocalServer(id string) {
	a.localsrvManager.Stop(id)
}

// StartTelnetServer starts a Telnet server on port. The password is required.
// Returns the server id.
func (a *App) StartTelnetServer(port int, password string, timeoutSeconds int) (string, error) {
	return a.localsrvManager.StartTelnetServer(localsrv.TelnetConfig{
		Port:           port,
		Password:       password,
		TimeoutSeconds: timeoutSeconds,
	})
}

// StartNFSServer starts the experimental NFSv4 server on port. The filesystem
// is in-memory (data lost on stop). Returns the server id.
func (a *App) StartNFSServer(port int, timeoutSeconds int) (string, error) {
	return a.localsrvManager.StartNFSServer(localsrv.NFSConfig{
		Port:           port,
		TimeoutSeconds: timeoutSeconds,
	})
}

// StartS3Server starts the experimental S3-compatible server exposing rootDir
// as an object store on port. Access/secret keys are accepted for client
// compatibility but not cryptographically verified. Returns the server id.
func (a *App) StartS3Server(rootDir string, port int, accessKey, secretKey string, timeoutSeconds int) (string, error) {
	return a.localsrvManager.StartS3Server(localsrv.S3Config{
		RootDir:        rootDir,
		Port:           port,
		AccessKey:      accessKey,
		SecretKey:      secretKey,
		TimeoutSeconds: timeoutSeconds,
	})
}
