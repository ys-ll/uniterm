package k8s

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// WatchEvent 是 apiserver 推给我们的原始事件（object 保持 JSON 原文）。
type WatchEvent struct {
	Type   string          `json:"type"`
	Object json.RawMessage `json:"object"`
}

// startWatchStream 建立 watch 连接并在后台 goroutine 里逐行读取，
// 每收到一条 event 调 cb；stream 结束（EOF/错误/context 取消）时调 onEnd。
// 本函数立即返回，后台读循环由 goroutine 承担。
//
// On non-context errors the goroutine reconnects with exponential
// backoff (1s, 2s, 4s, 8s, max 30s) so an apiserver bounce does not
// silently kill the watch (K8S-02).
func startWatchStream(ctx context.Context, client *http.Client, base, path string,
	cb func(WatchEvent), onEnd func(error)) error {

	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("path must start with /: %q", path)
	}
	go runWatchLoop(ctx, client, base, path, cb, onEnd)
	return nil
}

func runWatchLoop(ctx context.Context, client *http.Client, base, path string,
	cb func(WatchEvent), onEnd func(error)) {
	backoff := time.Second
	const maxBackoff = 30 * time.Second
	for {
		err := runOneWatch(ctx, client, base, path, cb)
		if ctx.Err() != nil {
			onEnd(ctx.Err())
			return
		}
		if err == nil {
			// Clean EOF (server closed the stream). Surface once and stop —
			// the original contract is "stream ended → onEnd then exit".
			// An apiserver bounce shows up as a non-nil err and triggers
			// the backoff branch below (K8S-02).
			onEnd(nil)
			return
		}
		// Transport / scanner error — likely apiserver bounce. Backoff
		// and retry, but surface once via onEnd so the UI can show the
		// interruption (K8S-02).
		onEnd(err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

func runOneWatch(ctx context.Context, client *http.Client, base, path string,
	cb func(WatchEvent)) error {
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
	return scanner.Err()
}
