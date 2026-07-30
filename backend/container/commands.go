package container

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// withNS 把 nerdctl 的全局 flag 插到子命令之前；其他运行时原样返回。
func withNS(rt Runtime, ns string, argv ...string) []string {
	out := []string{rt.Bin()}
	if rt == RuntimeNerdctl && ns != "" {
		out = append(out, "--namespace", ns)
	}
	return append(out, argv...)
}

// sanitizeVolumeSpec guards against path-traversal in volume bind-mounts.
// The host-side path must be absolute, must NOT contain any ".." segment
// after clean, and the separator must be ":".
//
// Accepts both "<abs-host>:<abs-container>" (bind mount) and
// "<name>:<abs-container>" (named volume, host side has no leading "/").
// Anything else — relative host paths, traversal segments, embedded
// whitespace for argv smuggling — is rejected.
func sanitizeVolumeSpec(v string) (string, error) {
	if v == "" {
		return "", fmt.Errorf("empty volume spec")
	}
	// Split on the first ":"; the host side has no scheme so this is safe.
	idx := strings.Index(v, ":")
	if idx < 0 {
		return "", fmt.Errorf("volume spec %q missing host:container separator", v)
	}
	host := v[:idx]
	container := v[idx+1:]
	if container == "" {
		return "", fmt.Errorf("volume spec %q missing container path", v)
	}
	// Host path can be either absolute (/foo or C:\foo) OR a named volume
	// ("myvolume") — named volumes have no path separators and no leading
	// slash. Reject anything that looks like a path with traversal.
	if strings.ContainsAny(host, `\/`) {
		cleaned := filepath.Clean(host)
		if !filepath.IsAbs(cleaned) {
			return "", fmt.Errorf("volume host path must be absolute: %q", host)
		}
		// filepath.Clean collapses ".."; if any segment was ".." the
		// cleaned form would either drop or shrink the path — check for
		// the raw literal too as defense in depth.
		if strings.Contains(host, "..") {
			return "", fmt.Errorf("volume host path contains traversal segment: %q", host)
		}
		if cleaned != host {
			return "", fmt.Errorf("volume host path must be cleaned: %q", host)
		}
	}
	if strings.ContainsAny(container, " \t\n\r") {
		return "", fmt.Errorf("volume container path contains whitespace: %q", container)
	}
	return host + ":" + container, nil
}

const jsonFormat = "{{json .}}"

func psArgs(rt Runtime, ns string) []string {
	return withNS(rt, ns, "ps", "-a", "--format", jsonFormat)
}

func inspectArgs(rt Runtime, ns, id string) []string {
	return withNS(rt, ns, "inspect", id)
}

func logsArgs(rt Runtime, ns, id string, tail int, follow, timestamps bool) []string {
	argv := []string{"logs", "--tail", strconv.Itoa(tail)}
	if timestamps {
		argv = append(argv, "--timestamps")
	}
	if follow {
		argv = append(argv, "-f")
	}
	return withNS(rt, ns, append(argv, id)...)
}

func execArgs(rt Runtime, ns, id, shell string) []string {
	return withNS(rt, ns, "exec", "-it", id, shell)
}

// action ∈ start/stop/restart/rm/pause/unpause
func actionArgs(rt Runtime, ns, action, id string) ([]string, error) {
	switch action {
	case "start", "stop", "restart", "rm", "pause", "unpause":
		return withNS(rt, ns, action, id), nil
	}
	return nil, fmt.Errorf("unsupported action %q", action)
}

func renameArgs(rt Runtime, ns, id, newName string) []string {
	return withNS(rt, ns, "rename", id, newName)
}

func statsArgs(rt Runtime, ns string) []string {
	return withNS(rt, ns, "stats", "--no-stream", "--format", jsonFormat)
}

func imagesArgs(rt Runtime, ns string) []string {
	return withNS(rt, ns, "images", "--format", jsonFormat)
}

func pullArgs(rt Runtime, ns, image string) []string {
	return withNS(rt, ns, "pull", image)
}

func removeImageArgs(rt Runtime, ns, imageID string) []string {
	return withNS(rt, ns, "rmi", imageID)
}

func createArgs(rt Runtime, ns string, o CreateOptions) ([]string, error) {
	argv := []string{"run", "-d"}
	if o.Name != "" {
		argv = append(argv, "--name", o.Name)
	}
	for _, p := range o.Ports {
		v := p.HostPort + ":" + p.ContainerPort
		if p.HostIP != "" {
			v = p.HostIP + ":" + v
		}
		if p.Protocol != "" && p.Protocol != "tcp" {
			v += "/" + p.Protocol
		}
		argv = append(argv, "-p", v)
	}
	for _, v := range o.Volumes {
		sanitized, err := sanitizeVolumeSpec(v)
		if err != nil {
			return nil, err
		}
		argv = append(argv, "-v", sanitized)
	}
	for _, e := range o.Env {
		argv = append(argv, "-e", e)
	}
	if o.Restart != "" && o.Restart != "no" {
		argv = append(argv, "--restart", o.Restart)
	}
	argv = append(argv, o.Image)
	return withNS(rt, ns, append(argv, o.Command...)...), nil
}

// detectArgs: command -v 走 shell，两 runner 均支持（见各 runner 的特例处理）。
func detectArgs(rt Runtime) []string {
	return []string{"sh", "-c", "command -v " + rt.Bin()}
}

// posixQuote 供 SSHRunner 把 argv 拼成远端 sh 命令行；LocalRunner 不使用。
func posixQuote(s string) string {
	return "'" + strings.ReplaceAll(s, `'`, `'\''`) + "'"
}

// JoinShellCommand 拼接 argv 为 POSIX shell 命令行。对纯安全字符的段不加引号，保持可读。
func JoinShellCommand(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		if a == "" {
			parts[i] = "''"
			continue
		}
		safe := true
		for _, r := range a {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
				strings.ContainsRune("-._/:={}+", r)) {
				safe = false
				break
			}
		}
		if safe {
			parts[i] = a
		} else {
			parts[i] = posixQuote(a)
		}
	}
	return strings.Join(parts, " ")
}
