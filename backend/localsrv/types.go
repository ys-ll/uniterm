// Package localsrv implements the "Local Services" tool: a set of small
// built-in servers (HTTP, TFTP, SSH/SFTP, FTP) that let other machines on
// the network reach files on this host. Each server runs in its own goroutine
// and streams log lines back to the frontend as Wails events.
package localsrv

// Service identifies one of the built-in servers.
type Service string

const (
	ServiceHTTP   Service = "http"
	ServiceTFTP   Service = "tftp"
	ServiceSSH    Service = "ssh"
	ServiceFTP    Service = "ftp"
	ServiceTelnet Service = "telnet"
	ServiceNFS    Service = "nfs"
	ServiceS3     Service = "s3"
)

// S3Config holds the parameters for the S3-compatible server.
type S3Config struct {
	RootDir        string `json:"rootDir"`
	Port           int    `json:"port"`
	AccessKey      string `json:"accessKey"`
	SecretKey      string `json:"secretKey"`
	TimeoutSeconds int    `json:"timeoutSeconds"` // 0 = no auto-stop
}

// NFSConfig holds the parameters for the NFS server.
type NFSConfig struct {
	Port           int `json:"port"`
	TimeoutSeconds int `json:"timeoutSeconds"` // 0 = no auto-stop
}

// TelnetConfig holds the parameters for the Telnet server. The password is
// required: the shell grants full access to this machine.
type TelnetConfig struct {
	Port           int    `json:"port"`
	Password       string `json:"password"`
	TimeoutSeconds int    `json:"timeoutSeconds"` // 0 = no auto-stop
}

// Emitter forwards server events to the frontend (wired to Wails' event bus).
type Emitter func(name string, payload any)

// Event names emitted by local servers. Every payload carries "service" and "id".
const (
	EventLog   = "localsrv:log"   // {service, id, message}
	EventState = "localsrv:state" // {service, id, running}
)

// HTTPConfig holds the parameters for the HTTP file server.
type HTTPConfig struct {
	RootDir       string `json:"rootDir"`
	Port          int    `json:"port"`
	ListDir       bool   `json:"listDir"`
	TimeoutSeconds int   `json:"timeoutSeconds"` // 0 = no auto-stop
}

// TFTPConfig holds the parameters for the TFTP server.
type TFTPConfig struct {
	RootDir        string `json:"rootDir"`
	Port           int    `json:"port"`
	BlockSize      int    `json:"blockSize"`
	TimeoutSeconds int    `json:"timeoutSeconds"` // 0 = no auto-stop
}

// SSHConfig holds the parameters for the SSH server (interactive shell +
// SFTP). Username and password are required: the shell grants full access
// to this machine.
type SSHConfig struct {
	RootDir        string `json:"rootDir"`
	Port           int    `json:"port"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	TimeoutSeconds int    `json:"timeoutSeconds"` // 0 = no auto-stop
}

// FTPConfig holds the parameters for the FTP server.
type FTPConfig struct {
	RootDir        string `json:"rootDir"`
	Port           int    `json:"port"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	Anonymous      bool   `json:"anonymous"`
	UTF8           bool   `json:"utf8"`
	TimeoutSeconds int    `json:"timeoutSeconds"` // 0 = no auto-stop
}
