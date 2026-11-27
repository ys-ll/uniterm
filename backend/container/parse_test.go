package container

import (
	"os"
"strings"
	"testing"
)

func TestParseContainersDockerTSV(t *testing.T) {
	out := []byte("a1b2c3d4e5f6|nginx:latest|web|running|Up 5 days|0.0.0.0:8080->80/tcp|2026-01-02 03:04:05 +0000 UTC\n" +
		"a1b2c3d4e5f7|registry.example.com/foo:1|api|exited|Exited (0) 2 hours ago||2026-01-02 03:04:06 +0000 UTC\n" +
		"bad line without separators\n")
	list, err := ParseContainers(RuntimeDocker, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d containers: %+v", len(list), list)
	}
	if list[0].Name != "web" || list[0].State != "running" || list[0].Ports != "0.0.0.0:8080->80/tcp" {
		t.Fatalf("row0: %+v", list[0])
	}
	if list[1].Name != "api" || list[1].State != "exited" || list[1].Ports != "" {
		t.Fatalf("row1: %+v", list[1])
	}
}

// 单行坏数据不拖垮整列表
func TestParseContainersSkipsBadLine(t *testing.T) {
	out := []byte("{\"ID\":\"a1\",\"Image\":\"nginx\",\"Names\":[\"web\"],\"State\":\"running\",\"Status\":\"Up 1h\",\"Ports\":\"\",\"CreatedAt\":\"\"}\nnot-json\n")
	list, err := ParseContainers(RuntimePodman, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "web" {
		t.Fatalf("got %+v", list)
	}
}

// podman 的 Names 是数组
func TestParseContainersPodmanNamesArray(t *testing.T) {
	out := []byte("{\"ID\":\"b2\",\"Image\":\"docker.io/library/nginx:latest\",\"Names\":[\"web\",\"web-1\"],\"State\":\"running\",\"Status\":\"Up 2 hours\",\"Ports\":\"\",\"Created\":\"2026-07-20 09:30:12 +0800 CST\"}\n")
	list, err := ParseContainers(RuntimePodman, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "web" {
		t.Fatalf("got %+v", list)
	}
}

// podman images 的 Names 同样是数组
func TestParseImagesPodmanNamesArray(t *testing.T) {
	out := []byte("{\"ID\":\"c3\",\"Names\":[\"docker.io/library/nginx:latest\",\"docker.io/library/nginx:1.27\"],\"Size\":\"192MB\",\"Created\":\"2026-07-20 09:30:12 +0800 CST\"}\n")
	list, err := ParseImages(RuntimePodman, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Repository != "docker.io/library/nginx:latest" {
		t.Fatalf("got %+v", list)
	}
}

// stats 的 Name/Names 也可能是数组
func TestParseStatsPodmanNamesArray(t *testing.T) {
	out := []byte("{\"ID\":\"d4\",\"Names\":[\"web\",\"web-1\"],\"CPUPerc\":\"0.50%\",\"MemUsage\":\"10MiB / 1GiB\",\"MemPerc\":\"1.00%\",\"NetIO\":\"1kB / 2kB\",\"BlockIO\":\"0B / 0B\"}\n")
	list, err := ParseStats(RuntimePodman, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "web" {
		t.Fatalf("got %+v", list)
	}
}

// ── WSLC (OCI schema) ────────────────────────────────────────

func TestParseContainersWSLC(t *testing.T) {
	raw, err := os.ReadFile("testdata/wslc_ps.json")
	if err != nil {
		t.Skip("golden file missing")
	}
	list, err := ParseContainers(RuntimeWSLC, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("want 3 containers, got %d", len(list))
	}
	// State=2 → running
	if list[0].State != "running" {
		t.Errorf("container 0 state: want running, got %q", list[0].State)
	}
	if list[0].Name != "test" || list[0].Image != "nginx:latest" {
		t.Errorf("container 0 fields: %+v", list[0])
	}
	// State=3 → exited
	if list[2].State != "exited" {
		t.Errorf("container 2 state: want exited, got %q", list[2].State)
	}
	// Ports: BindingAddress + numeric Protocol
	if list[1].Ports != "127.0.0.1:18888->8888/tcp" {
		t.Errorf("container 1 ports: want 127.0.0.1:18888->8888/tcp, got %q", list[1].Ports)
	}
	// CreatedAt: Unix timestamp
	if list[0].CreatedAt == "" {
		t.Error("container 0 createdAt should not be empty")
	}
}

func TestParseContainersWSLCJSONArray(t *testing.T) {
	// Inline JSON array (not line-delimited)
	out := []byte(`[{"Id":"abc","Name":"web","Image":"nginx","State":2,"CreatedAt":1787571632,"Ports":[]}]`)
	list, err := ParseContainers(RuntimeWSLC, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].State != "running" {
		t.Fatalf("got %+v", list)
	}
}

func TestParseVersions(t *testing.T) {
	out := []byte(`{"Client":{"Version":"27.3.1","ApiVersion":"1.47"},"Server":{"Version":"26.1.4","ApiVersion":"1.45"}}`)
	client, server, comp := ParseVersions(RuntimeDocker, out)
	if client != "27.3.1" || server != "26.1.4" || comp != "" {
		t.Fatalf("got client=%q server=%q comp=%q", client, server, comp)
	}
	// daemon 不可达时只有 Client 段
	client, server, comp = ParseVersions(RuntimeDocker, []byte(`{"Client":{"Version":"27.3.1"},"Server":{"Version":""}}`))
	if client != "27.3.1" || server != "" || comp != "" {
		t.Fatalf("got client=%q server=%q comp=%q", client, server, comp)
	}
	// docker 的 Server.Components 首个组件是 Engine
	client, server, comp = ParseVersions(RuntimeDocker, []byte(`{"Client":{"Version":"27.3.1"},"Server":{"Version":"26.1.4","Components":[{"Name":"Engine","Version":"26.1.4"},{"Name":"containerd","Version":"1.6.33"}]}}`))
	if client != "27.3.1" || server != "26.1.4" || comp != "Engine" {
		t.Fatalf("got client=%q server=%q comp=%q", client, server, comp)
	}
}

func TestParseVersionsNerdctlComponents(t *testing.T) {
	// nerdctl 真实输出：Server 无 Version，版本在 Components 的 containerd 项里。
	out := []byte(`{"Client":{"Version":"v2.2.1","Components":[{"Name":"buildctl","Version":""}]},"Server":{"Components":[{"Name":"containerd","Version":"v1.7.13","Details":{"GitCommit":"7c3aca7a"}},{"Name":"runc","Version":"1.1.12"}]}}`)
	client, server, comp := ParseVersions(RuntimeNerdctl, out)
	if client != "v2.2.1" || server != "v1.7.13" || comp != "containerd" {
		t.Fatalf("got client=%q server=%q comp=%q", client, server, comp)
	}
}

func TestParseRuntimeInfo(t *testing.T) {
	// docker 扁平键
	info := ParseRuntimeInfo(RuntimeDocker, []byte(`{"ServerVersion":"27.3.1","OperatingSystem":"Ubuntu 22.04","OSType":"linux","Architecture":"x86_64","KernelVersion":"5.15.0","Driver":"overlay2","CgroupDriver":"cgroupfs","CgroupVersion":"1","NCPU":8,"MemTotal":16300000000}`))
	if info.Os != "Ubuntu 22.04" || info.OsType != "linux" || info.Arch != "x86_64" ||
		info.Driver != "overlay2" || info.CgroupDriver != "cgroupfs" || info.CgroupVersion != "1" ||
		info.NCPU != "8" || info.MemTotal == "" {
		t.Fatalf("docker info: %+v", info)
	}
	// podman 嵌套结构
	info = ParseRuntimeInfo(RuntimePodman, []byte(`{"host":{"os":"linux","Distribution":{"distribution":"fedora","version":"39"},"arch":"amd64","kernel":"6.5.0","cpus":4,"memory":8000000000,"cgroupsVersion":"v2","cgroupManager":"systemd"},"store":{"driver":"overlay"},"version":{"Version":"5.0.0"}}`))
	if info.Os != "fedora 39" || info.OsType != "linux" || info.Arch != "amd64" ||
		info.Driver != "overlay" || info.CgroupVersion != "v2" || info.CgroupDriver != "systemd" ||
		info.NCPU != "4" || info.ServerVersion != "5.0.0" {
		t.Fatalf("podman info: %+v", info)
	}
}

func TestParseImageHistory(t *testing.T) {
	out := []byte(`{"Comment":"","CreatedAt":"2026-01-02T03:04:05Z","CreatedBy":"/bin/sh -c #(nop) CMD [\"sh\"]","ID":"<missing>","Size":12345,"Tags":null,"CreatedSince":"3 weeks ago"}
{"Comment":"","CreatedAt":"2026-01-02T03:04:06Z","CreatedBy":"COPY app /app","ID":"abc123","Size":6789012,"Tags":["latest"],"CreatedSince":"3 weeks ago"}
not-json-line
`)
	layers, err := ParseImageHistory(RuntimeDocker, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(layers) != 2 {
		t.Fatalf("got %d layers, want 2: %+v", len(layers), layers)
	}
	if layers[0].CreatedBy != `/bin/sh -c #(nop) CMD ["sh"]` || layers[0].Size != "12.1KB" {
		t.Fatalf("layer0: %+v", layers[0])
	}
	if layers[1].ID != "abc123" || layers[1].CreatedAt != "3 weeks ago" {
		t.Fatalf("layer1: %+v", layers[1])
	}
}

// WSLC 无 history 命令，层列表来自 image inspect 的 RootFS.Layers。
func TestParseImageLayersFromInspectWSLC(t *testing.T) {
	raw, err := os.ReadFile("testdata/wslc_image_inspect.json")
	if err != nil {
		t.Skip("golden file missing")
	}
	layers, err := ParseImageLayersFromInspect(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(layers) != 7 {
		t.Fatalf("got %d layers, want 7: %+v", len(layers), layers)
	}
	// RootFS.Layers 自底向上；结果反转成新层在前。
	if layers[0].ID != "sha256:7777777777777777777777777777777777777777777777777777777777777777" {
		t.Errorf("first layer should be the newest: %+v", layers[0])
	}
	if layers[6].ID != "sha256:1111111111111111111111111111111111111111111111111111111111111111" {
		t.Errorf("last layer should be the base: %+v", layers[6])
	}
	if layers[0].CreatedBy != "" || layers[0].Size != "" {
		t.Errorf("inspect layers carry no command/size: %+v", layers[0])
	}
}

func TestParseImageLayersFromInspectErrors(t *testing.T) {
	if _, err := ParseImageLayersFromInspect([]byte(`not json`)); err == nil {
		t.Fatal("want error for invalid json")
	}
	if _, err := ParseImageLayersFromInspect([]byte(`[]`)); err == nil {
		t.Fatal("want error for empty result")
	}
	// 无 RootFS 时不报错，返回空列表（列表页对无层镜像仍可用）。
	layers, err := ParseImageLayersFromInspect([]byte(`[{"Id":"x"}]`))
	if err != nil || len(layers) != 0 {
		t.Fatalf("got %v, %v", layers, err)
	}
}

func TestParseImagesWSLC(t *testing.T) {
	raw, err := os.ReadFile("testdata/wslc_images.json")
	if err != nil {
		t.Skip("golden file missing")
	}
	list, err := ParseImages(RuntimeWSLC, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 images, got %d", len(list))
	}
	if list[0].Repository != "nginx" || list[0].Tag != "latest" {
		t.Errorf("image 0: %+v", list[0])
	}
	// Size is numeric bytes → human readable
	if list[0].Size == "" {
		t.Error("image 0 size should not be empty")
	}
}

func TestParseImagesWSLCJSONArray(t *testing.T) {
	out := []byte(`[{"Id":"x1","Repository":"redis","Tag":"7","Size":32000000,"CreatedAt":1786000000}]`)
	list, err := ParseImages(RuntimeWSLC, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Repository != "redis" {
		t.Fatalf("got %+v", list)
	}
}

// ── normalizeState ───────────────────────────────────────────

func TestNormalizeState(t *testing.T) {
	tests := []struct {
		state, status, want string
	}{
		// WSLC numeric codes
		{"0", "", "created"},
		{"1", "", "created"},
		{"2", "", "running"},
		{"3", "", "exited"},
		{"4", "", "paused"},
		{"5", "", "exited"},
		// Docker/Podman string states (passthrough)
		{"running", "", "running"},
		{"Running", "", "running"},
		{"exited", "", "exited"},
		{"Exited (0)", "", "exited"},
		{"paused", "", "paused"},
		{"created", "", "created"},
		// nerdctl: no State, derive from Status
		{"", "Up 2 hours", "running"},
		{"", "Exited (0) 5 min ago", "exited"},
		{"", "Paused", "paused"},
		// Unrecognized numeric → fall back to Status
		{"99", "Up 1h", "running"},
		{"99", "Exited (1)", "exited"},
		{"99", "", "99"},
	}
	for _, tt := range tests {
		got := normalizeState(tt.state, tt.status)
		if got != tt.want {
			t.Errorf("normalizeState(%q, %q) = %q, want %q", tt.state, tt.status, got, tt.want)
		}
	}
}

// ── formatPortObjects WSLC ──────────────────────────────────

func TestFormatPortObjectsWSLC(t *testing.T) {
	arr := []any{
		map[string]any{
			"BindingAddress": "0.0.0.0",
			"ContainerPort":  float64(8080),
			"HostPort":       float64(9090),
			"Protocol":       float64(6), // TCP
		},
	}
	got := formatPortObjects(arr)
	want := "0.0.0.0:9090->8080/tcp"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatPortObjectsWSLCUDP(t *testing.T) {
	arr := []any{
		map[string]any{
			"ContainerPort": float64(53),
			"HostPort":      float64(5353),
			"Protocol":      float64(17), // UDP
		},
	}
	got := formatPortObjects(arr)
	want := "0.0.0.0:5353->53/udp"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}


func TestParseInspectDocker(t *testing.T) {
	raw, err := os.ReadFile("testdata/docker_inspect.json")
	if err != nil {
		t.Skip("golden file missing, run spike task")
	}
	d, err := ParseInspect(RuntimeDocker, raw)
	if err != nil {
		t.Fatal(err)
	}
	if d.ID == "" || d.Image == "" || d.State == "" {
		t.Fatalf("incomplete detail: %+v", d)
	}
}

// 合成样本验证字段映射与 Name 去斜杠
func TestParseInspectSynthetic(t *testing.T) {
	raw := []byte(`[{
		"Id": "3f8b2c1a9e77",
		"Name": "/web",
		"Config": {"Image": "nginx:latest", "Cmd": ["nginx","-g","daemon off;"], "Entrypoint": ["/entrypoint.sh"], "Env": ["A=1","B=2"], "WorkingDir": "/app", "User": "nginx"},
		"State": {"Status": "running", "OOMKilled": false, "ExitCode": 0, "StartedAt": "2026-07-20T01:30:12Z", "FinishedAt": "0001-01-01T00:00:00Z", "Pid": 1234},
		"HostConfig": {"NetworkMode": "bridge", "RestartPolicy": {"Name": "always"}, "Binds": ["/data:/usr/share/nginx/html:ro"]},
		"NetworkSettings": {"IPAddress": "172.17.0.2", "Gateway": "172.17.0.1", "Ports": {"80/tcp": [{"HostIp":"0.0.0.0","HostPort":"8080"}], "443/tcp": null}},
		"Mounts": [{"Source": "/data", "Destination": "/usr/share/nginx/html", "RW": false}]
	}]`)
	d, err := ParseInspect(RuntimeDocker, raw)
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "web" || d.User != "nginx" || d.RestartPolicy != "always" || d.IP != "172.17.0.2" {
		t.Fatalf("got %+v", d)
	}
	if len(d.Env) != 2 || len(d.Mounts) != 1 || d.Mounts[0].RW {
		t.Fatalf("got %+v", d)
	}
	// 443/tcp 为 null（暴露未发布）应跳过
	if len(d.Ports) != 1 || d.Ports[0].HostPort != "8080" || d.Ports[0].ContainerPort != "80" || d.Ports[0].Protocol != "tcp" {
		t.Fatalf("ports: %+v", d.Ports)
	}
	if !strings.Contains(d.Command, "nginx") {
		t.Fatalf("command: %q", d.Command)
	}
}

func TestParseNamespaces(t *testing.T) {
	// nerdctl namespace ls 表格输出（ spike 校准；兼容 NAME 表头行）
	out := "NAME        CONTAINERS    IMAGES    VOLUMES\ndefault     2             5         0\nk8s.io      30            60        0\n"
	ns := ParseNamespaces([]byte(out))
	if len(ns) != 2 || ns[0] != "default" || ns[1] != "k8s.io" {
		t.Fatalf("got %v", ns)
	}
}
