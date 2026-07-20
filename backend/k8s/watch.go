package k8s

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// WatchEvent 是 apiserver 推给我们的原始事件（object 保持 JSON 原文）。
type WatchEvent struct {
	Type   string          `json:"type"`
	Object json.RawMessage `json:"object"`
}

// startWatchStream 建立 watch 连接并在后台 goroutine 里逐行读取，
// 每收到一条 event 调 cb；stream 结束（EOF/错误/context 取消）时调 onEnd。
// 本函数立即返回，后台读循环由 goroutine 承担。
func startWatchStream(ctx context.Context, client *http.Client, base, path string,
	cb func(WatchEvent), onEnd func(error)) error {

	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("path must start with /: %q", path)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		body := make([]byte, 512)
		n, _ := resp.Body.Read(body)
		resp.Body.Close()
		return fmt.Errorf("watch HTTP %d: %s", resp.StatusCode, string(body[:n]))
	}

	go func() {
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var ev WatchEvent
			if err := json.Unmarshal(line, &ev); err != nil {
				continue
			}
			cb(ev)
		}
		onEnd(scanner.Err())
	}()

	return nil
}
