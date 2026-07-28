package container

import (
	"os"
	"testing"
)

func TestParseContainersDocker(t *testing.T) {
	raw, err := os.ReadFile("testdata/docker_ps.jsonl")
	if err != nil {
		t.Skip("golden file missing, run spike task")
	}
	list, err := ParseContainers(RuntimeDocker, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 {
		t.Fatal("no containers parsed")
	}
	for _, c := range list {
		if c.ID == "" || c.Name == "" || c.Image == "" || c.State == "" {
			t.Fatalf("incomplete row: %+v", c)
		}
	}
}

// 单行坏数据不拖垮整列表
func TestParseContainersSkipsBadLine(t *testing.T) {
	out := []byte("{\"ID\":\"a1\",\"Image\":\"nginx\",\"Names\":\"web\",\"State\":\"running\",\"Status\":\"Up 1h\",\"Ports\":\"\",\"CreatedAt\":\"\"}\nnot-json\n")
	list, err := ParseContainers(RuntimeDocker, out)
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
