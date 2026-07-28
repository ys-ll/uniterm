package container

import (
	"fmt"
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

func createArgs(rt Runtime, ns string, o CreateOptions) []string {
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
		argv = append(argv, "-v", v)
	}
	for _, e := range o.Env {
		argv = append(argv, "-e", e)
	}
	if o.Restart != "" && o.Restart != "no" {
		argv = append(argv, "--restart", o.Restart)
	}
	argv = append(argv, o.Image)
	return withNS(rt, ns, append(argv, o.Command...)...)
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
