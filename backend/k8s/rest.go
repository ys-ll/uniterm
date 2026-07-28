package k8s

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ys-ll/uniterm/backend/log"
)

// maxK8sResponseBytes caps the size of any single REST response body so
// a runaway server (or a misconfigured apiserver returning huge CRDs
// or log lists) cannot exhaust process memory. 64 MiB matches the
// typical kubectl response cap and is well above any realistic single
// object (K8S-05).
const maxK8sResponseBytes = 64 * 1024 * 1024

// Do 是通用 REST 请求 —— 拼 URL、发请求、返回 (status, body, err)。
// path 必须以 "/" 开头。contentType 可以为空。
func Do(ctx context.Context, client *http.Client, base, method, path string, body []byte, contentType string) (int, []byte, error) {
	if !strings.HasPrefix(path, "/") {
		return 0, nil, fmt.Errorf("path must start with /: %q", path)
	}
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, reader)
	if err != nil {
		return 0, nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		log.Writef("[k8s] %s %s%s -> transport err: %v", method, base, path, err)
		return 0, nil, err
	}
	defer resp.Body.Close()
	// Cap the read so a pathological server can't OOM us (K8S-05).
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxK8sResponseBytes+1))
	if err != nil {
		log.Writef("[k8s] %s %s%s -> read body err: %v (status=%d)", method, base, path, err, resp.StatusCode)
		return resp.StatusCode, nil, err
	}
	if len(b) > maxK8sResponseBytes {
		return resp.StatusCode, nil, errors.New("response body exceeds 64 MiB limit")
	}
	preview := b
	if len(preview) > 300 {
		preview = preview[:300]
	}
	log.Writef("[k8s] %s %s%s -> %d bytes=%d body=%s", method, base, path, resp.StatusCode, len(b), string(preview))
	return resp.StatusCode, b, nil
}
