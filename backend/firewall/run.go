package firewall

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ys-ll/uniterm/backend/session"
)

// remoteDialConfig converts a ResolvedSSH into a session.ConnectionConfig for
// DialSSHClient. It mirrors packetcap's mapping so the app layer can hand us a
// plain resolved struct instead of reaching into the session package.
func remoteDialConfig(r *ResolvedSSH) session.ConnectionConfig {
	return session.ConnectionConfig{
		Host:       r.Host,
		Port:       r.Port,
		User:       r.User,
		Password:   r.Password,
		AuthType:   r.AuthType,
		KeyPath:    r.KeyPath,
		KeyContent: r.KeyContent,
		IdentityId: r.IdentityID,
	}
}

// runCommand connects to the remote host over SSH and runs a single command,
// returning its output (stdout). It is the shared primitive all firewall
// operations use; the command is built by each backend adapter.
func runCommand(r *ResolvedSSH, cmd string) (string, error) {
	if r == nil {
		return "", fmt.Errorf("firewall: missing resolved SSH config")
	}
	if r.Host == "" {
		return "", fmt.Errorf("firewall: missing remote host")
	}
	client, err := session.DialSSHClient(remoteDialConfig(r))
	if err != nil {
		return "", err
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("firewall: ssh session: %w", err)
	}
	defer sess.Close()

	// sess.Output returns stdout; on non-zero exit it returns a *ssh.ExitError
	// whose message text carries the exit status. stderr of the remote command
	// is discarded here (ufw/firewalld report the reason on the error string).
	out, err := sess.Output(cmd)
	if err != nil {
		return string(out), err
	}
	return strings.TrimSpace(string(out)), nil
}

// permissionDenied matches error text indicating the remote account lacks the
// privileges required to touch the firewall (non-root without passwordless
// sudo): "permission denied", "Operation not permitted", "not in the sudoers",
// or an explicit "must be root" / non-zero EUID message from firewalld/ufw.
var permissionDenied = regexp.MustCompile(`(?i)permission denied|operation not permitted|access denied|not a sudoers|must be root|euid:\s*[^0]+`)

// IsPermissionError reports whether err is a permission/privilege problem so
// the frontend can surface a dedicated "no permission" message.
func IsPermissionError(err error) bool {
	if err == nil {
		return false
	}
	return permissionDenied.MatchString(err.Error())
}

// shellQuote quotes a word for safe inclusion in a remote shell command.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
