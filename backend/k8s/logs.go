package k8s

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// startLogStream 建立 Pod 日志流并在后台 goroutine 里逐行读取，
// 每收到一行调 cb；stream 结束（EOF/错误/context 取消）时调 onEnd。
//
// Same reconnect-with-backoff shape as startWatchStream so a pod
// restart or apiserver bounce does not silently kill the log stream
// (K8S-02).
func startLogStream(ctx context.Context, client *http.Client, base, path string,
	cb func(string), onEnd func(error)) error {

	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("path must start with /: %q", path)
	}
	go runLogLoop(ctx, client, base, path, cb, onEnd)
	return nil
}

func runLogLoop(ctx context.Context, client *http.Client, base, path string,
	cb func(string), onEnd func(error)) {
	backoff := time.Second
	const maxBackoff = 30 * time.Second
	for {
		err := runOneLogStream(ctx, client, base, path, cb)
		if ctx.Err() != nil {
			onEnd(ctx.Err())
			return
		}
		if err == nil {
			// Clean EOF — pod log ended. Surface once and stop.
			onEnd(nil)
			return
		}
		// Transport / scanner error — likely apiserver bounce. Backoff
		// and retry, but surface once via onEnd (K8S-02).
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

func runOneLogStream(ctx context.Context, client *http.Client, base, path string,
	cb func(string)) error {
	req, err := http.NewRequestWithContext(ctx, "GET", base+path, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		buf := make([]byte, 512)
		n, _ := resp.Body.Read(buf)
		resp.Body.Close()
		return fmt.Errorf("log HTTP %d: %s", resp.StatusCode, string(buf[:n]))
	}
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		cb(scanner.Text())
	}
	return scanner.Err()
}

// buildLogPath 组装 Pod 日志查询；previous 为一次性历史日志，故 follow = !previous。
func buildLogPath(ns, pod, container string, tailLines int, timestamps, previous bool) string {
	follow := !previous
	q := fmt.Sprintf("container=%s&follow=%t&previous=%t&tailLines=%d&timestamps=%t",
		container, follow, previous, tailLines, timestamps)
	return fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/log?%s", ns, pod, q)
}
