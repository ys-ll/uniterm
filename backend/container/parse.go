package container

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ys-ll/uniterm/backend/log"
)

// jsonLines 逐行解析 line-delimited JSON；坏行跳过并记日志。
func jsonLines(out []byte, fn func(map[string]any)) {
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(line, &m); err != nil {
			log.Writef("[container] skip bad json line: %v", err)
			continue
		}
		fn(m)
	}
}

// pick 按别名表取第一个存在的字段。
func pick(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v, true
		}
	}
	return nil, false
}

func pickStr(m map[string]any, keys ...string) string {
	v, ok := pick(m, keys...)
	if !ok {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return fmt.Sprintf("%v", t)
	default:
		return ""
	}
}

// pickStrList 处理 "a" 与 ["a","b"] 两种形态（docker vs podman 的 Names）。
func pickStrList(m map[string]any, keys ...string) []string {
	v, ok := pick(m, keys...)
	if !ok {
		return nil
	}
	switch t := v.(type) {
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// formatPortObjects 把 podman 的结构化端口对象数组格式化为 docker 风格的字符串，
// 如 0.0.0.0:9090->9090/tcp（host_ip 为空时补 0.0.0.0，与 docker 显示对齐）。
func formatPortObjects(arr []any) string {
	var parts []string
	for _, e := range arr {
		p, ok := e.(map[string]any)
		if !ok {
			continue
		}
		hostPort := pickStr(p, "host_port", "HostPort")
		ctrPort := pickStr(p, "container_port", "ContainerPort")
		if ctrPort == "" {
			continue
		}
		proto := pickStr(p, "protocol", "Protocol")
		if proto == "" {
			proto = "tcp"
		}
		s := ""
		if hostPort != "" {
			hostIP := pickStr(p, "host_ip", "HostIp")
			if hostIP == "" {
				hostIP = "0.0.0.0"
			}
			s = hostIP + ":" + hostPort + "->"
		}
		parts = append(parts, s+ctrPort+"/"+proto)
	}
	return strings.Join(parts, ", ")
}

// pickCreated 取创建时间并归一化为本地时间 "2006-01-02 15:04:05"。跨运行时字段差异：
//   - docker/nerdctl：CreatedAt 是带时区后缀的字符串（"2026-07-27 09:22:18 +0800 CST"）
//   - podman ps：CreatedAt 为空、Created 是 RFC3339
//   - podman images：Created 是 Unix 时间戳（数字）
func pickCreated(m map[string]any) string {
	const outLayout = "2006-01-02 15:04:05"
	// docker/nerdctl 的 Go 默认时间格式，含数字时区与时区名。
	layouts := []string{time.RFC3339, "2006-01-02 15:04:05 -0700 MST", "2006-01-02 15:04:05 -0700"}
	for _, k := range []string{"CreatedAt", "Created", "CreatedSince"} {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case string:
			if t == "" {
				continue
			}
			for _, l := range layouts {
				if ts, err := time.Parse(l, t); err == nil {
					return ts.Local().Format(outLayout)
				}
			}
			return t
		case float64:
			return time.Unix(int64(t), 0).Format(outLayout)
		}
	}
	return ""
}

func ParseContainers(rt Runtime, out []byte) ([]Container, error) {
	var list []Container
	jsonLines(out, func(m map[string]any) {
		name := ""
		if names := pickStrList(m, "Names", "Name"); len(names) > 0 {
			name = names[0]
		}
		ports := pickStr(m, "Ports")
		if ports == "" {
			// podman 把 Ports 输出成对象数组；docker/nerdctl 是字符串。
			if arr, ok := m["Ports"].([]any); ok && len(arr) > 0 {
				ports = formatPortObjects(arr)
			} else if ps := pickStrList(m, "Ports"); len(ps) > 0 {
				ports = strings.Join(ps, ", ")
			}
		}
		state := pickStr(m, "State")
		// nerdctl 无 State 字段，从 Status 派生
		if state == "" {
			status := pickStr(m, "Status")
			switch {
			case strings.HasPrefix(status, "Up"), status == "Running":
				state = "running"
			case strings.HasPrefix(status, "Exited"):
				state = "exited"
			case strings.HasPrefix(status, "Paused"):
				state = "paused"
			case status != "":
				state = "unknown"
			}
		}
		list = append(list, Container{
			ID:        pickStr(m, "ID", "Id", "ContainerID"),
			Name:      name,
			Image:     pickStr(m, "Image"),
			State:     state,
			Status:    pickStr(m, "Status"),
			Ports:     ports,
			CreatedAt: pickCreated(m),
		})
	})
	return list, nil
}

// pickSize 取镜像大小。docker/nerdctl 是人类可读字符串（"192MB"）；
// podman images 的 Size 是字节数（数字），需转成人类可读。
func pickSize(m map[string]any) string {
	v, ok := m["Size"]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return humanSize(int64(t))
	}
	return ""
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func ParseImages(rt Runtime, out []byte) ([]Image, error) {
	var list []Image
	jsonLines(out, func(m map[string]any) {
		repo := pickStr(m, "Repository", "repository")
		if names := pickStrList(m, "Repository", "repository", "Names"); repo == "" && len(names) > 0 {
			repo = names[0]
		}
		tag := pickStr(m, "Tag", "tag")
		if repo == "" {
			repo = "<none>"
		}
		if tag == "" {
			tag = "<none>"
		}
		list = append(list, Image{
			ID:         pickStr(m, "ID", "Id"),
			Repository: repo,
			Tag:        tag,
			Size:       pickSize(m),
			CreatedAt:  pickCreated(m),
		})
	})
	return list, nil
}

func ParseStats(rt Runtime, out []byte) ([]Stats, error) {
	var list []Stats
	jsonLines(out, func(m map[string]any) {
		name := pickStr(m, "Name")
		if names := pickStrList(m, "Name", "Names"); name == "" && len(names) > 0 {
			name = names[0]
		}
		list = append(list, Stats{
			ID:         pickStr(m, "ID", "Container", "ContainerID"),
			Name:       name,
			CPUPercent: pickStr(m, "CPUPerc", "CPUPercent"),
			MemUsage:   pickStr(m, "MemUsage"),
			MemPercent: pickStr(m, "MemPerc", "MemPercent"),
			NetIO:      pickStr(m, "NetIO"),
			BlockIO:    pickStr(m, "BlockIO"),
		})
	})
	return list, nil
}

// ParseInspect 解析 `inspect` 的 JSON 数组输出（取 [0]），归一化为 ContainerDetail。
func ParseInspect(rt Runtime, raw []byte) (ContainerDetail, error) {
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err != nil {
		return ContainerDetail{}, fmt.Errorf("inspect: %w", err)
	}
	if len(arr) == 0 {
		return ContainerDetail{}, fmt.Errorf("inspect: empty result")
	}
	m := arr[0]

	sub := func(key string) map[string]any {
		if v, ok := m[key].(map[string]any); ok {
			return v
		}
		return map[string]any{}
	}
	cfg, state, host, net := sub("Config"), sub("State"), sub("HostConfig"), sub("NetworkSettings")

	strList := func(v any) []string {
		arr, ok := v.([]any)
		if !ok {
			return nil
		}
		out := make([]string, 0, len(arr))
		for _, e := range arr {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	join := func(v any) string { return strings.Join(strList(v), " ") }

	d := ContainerDetail{
		ID:            pickStr(m, "Id", "ID"),
		Name:          strings.TrimPrefix(pickStr(m, "Name"), "/"),
		Image:         pickStr(cfg, "Image"),
		State:         pickStr(state, "Status"),
		StartedAt:     pickStr(state, "StartedAt"),
		FinishedAt:    pickStr(state, "FinishedAt"),
		OOMKilled:     state["OOMKilled"] == true,
		RestartPolicy: pickStr(host, "RestartPolicy", "RestartPolicyName"),
		Entrypoint:    join(cfg["Entrypoint"]),
		Command:       join(cfg["Cmd"]),
		WorkDir:       pickStr(cfg, "WorkingDir"),
		User:          pickStr(cfg, "User"),
		NetworkMode:   pickStr(host, "NetworkMode"),
		IP:            pickStr(net, "IPAddress", "IP"),
		Gateway:       pickStr(net, "Gateway"),
		Env:           strList(cfg["Env"]),
	}
	if rp, ok := host["RestartPolicy"].(map[string]any); ok {
		d.RestartPolicy = pickStr(rp, "Name")
	}
	if code, ok := state["ExitCode"].(float64); ok {
		c := int(code)
		d.ExitCode = &c
	}
	if pid, ok := state["Pid"].(float64); ok {
		d.Pid = int(pid)
	}
	if mounts, ok := m["Mounts"].([]any); ok {
		for _, mv := range mounts {
			if mm, ok := mv.(map[string]any); ok {
				d.Mounts = append(d.Mounts, Mount{
					Source:      pickStr(mm, "Source"),
					Destination: pickStr(mm, "Destination"),
					RW:          mm["RW"] == true,
				})
			}
		}
	}
	if ports, ok := net["Ports"].(map[string]any); ok {
		for key, bindings := range ports {
			port, proto := key, "tcp"
			if i := strings.LastIndex(key, "/"); i >= 0 {
				port, proto = key[:i], key[i+1:]
			}
			list, ok := bindings.([]any)
			if !ok {
				continue // null：暴露未发布
			}
			for _, bv := range list {
				if bm, ok := bv.(map[string]any); ok {
					d.Ports = append(d.Ports, PortMapping{
						HostIP:        pickStr(bm, "HostIp"),
						HostPort:      pickStr(bm, "HostPort"),
						ContainerPort: port,
						Protocol:      proto,
					})
				}
			}
		}
		sort.Slice(d.Ports, func(i, j int) bool {
			pi, ei := strconv.Atoi(d.Ports[i].ContainerPort)
			pj, ej := strconv.Atoi(d.Ports[j].ContainerPort)
			if ei == nil && ej == nil {
				return pi < pj
			}
			return d.Ports[i].ContainerPort < d.Ports[j].ContainerPort
		})
	}
	d.Status = d.State
	// 保证切片非 nil：Go nil slice 序列化为 JSON null，前端 .map() 会崩
	if d.Ports == nil {
		d.Ports = []PortMapping{}
	}
	if d.Mounts == nil {
		d.Mounts = []Mount{}
	}
	if d.Env == nil {
		d.Env = []string{}
	}
	return d, nil
}

// ParseNamespaces 解析 `nerdctl namespace ls` 表格输出（spike 校准格式）。
func ParseNamespaces(out []byte) []string {
	var ns []string
	for i, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if i == 0 && strings.EqualFold(fields[0], "NAME") {
			continue
		}
		ns = append(ns, fields[0])
	}
	return ns
}
