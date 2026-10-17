package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	stdsync "sync"
	"time"

	_ "golang.org/x/image/webp" // registered for image.DecodeConfig
	"image"
	_ "image/gif"  // registered for image.DecodeConfig
	_ "image/jpeg" // registered for image.DecodeConfig
	_ "image/png"  // registered for image.DecodeConfig

	"github.com/mattn/go-ieproxy"
	"github.com/ys-ll/uniterm/backend/log"
	"github.com/ys-ll/uniterm/backend/mcpclient"
	"github.com/ys-ll/uniterm/backend/session"
	"github.com/ys-ll/uniterm/backend/store"
)

// llmProxy is the Proxy function for the shared LLM transport. It resolves
// the OS system proxy (Windows registry / macOS system config, including PAC
// scripts) and falls back to the HTTP_PROXY/HTTPS_PROXY environment
// variables — http.ProxyFromEnvironment alone misses users who enable
// "system proxy" mode in tools like Clash, which never sets env vars.
// ieproxy caches the system config on first read, so refresh it periodically
// via ReloadConf to pick up toggles made while the app is running.
// ReloadConf is not goroutine-safe, so all access is serialized here.
const llmProxyRefreshInterval = 30 * time.Second

var (
	llmProxyMu      stdsync.Mutex
	llmProxyFn      func(*http.Request) (*url.URL, error)
	llmProxyRefresh time.Time
)

func llmProxy(req *http.Request) (*url.URL, error) {
	llmProxyMu.Lock()
	defer llmProxyMu.Unlock()
	if llmProxyFn == nil || time.Since(llmProxyRefresh) >= llmProxyRefreshInterval {
		ieproxy.ReloadConf()
		llmProxyFn = ieproxy.GetProxyFunc()
		llmProxyRefresh = time.Now()
	}
	return llmProxyFn(req)
}

// llmDirectClient returns the package-level client for the "direct" proxy
// selection. Proxy is deliberately nil: direct means bypassing the system
// proxy AND the HTTP(S)_PROXY env vars. Separate from the system-proxy pool
// so both connection caches stay warm independently.
var (
	llmDirectOnce   stdsync.Once
	llmDirectShared *http.Client
)

func llmDirectClient() *http.Client {
	llmDirectOnce.Do(func() {
		llmDirectShared = &http.Client{Transport: &http.Transport{
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   8,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			ExpectContinueTimeout: 2 * time.Second,
		}}
	})
	return llmDirectShared
}

// llmHTTPClient returns the App-wide *http.Client used by every
// LLM-bound call. F-208: hoisted here so three back-to-back
// ChatCompletion calls reuse the same TCP+TLS connection instead of
// paying a fresh handshake each time. FetchModels uses a shorter
// timeout via a derived client (see FetchModels).
func (a *App) llmHTTPClient() *http.Client {
	a.httpClientOnce.Do(func() {
		tr := &http.Transport{
			Proxy:                 llmProxy,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   8,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			ExpectContinueTimeout: 2 * time.Second,
		}
		a.httpClient = &http.Client{Transport: tr}
	})
	return a.httpClient
}

// injectCacheControl adds ephemeral cache_control breakpoints on the
// static system prompt and tools array so Anthropic's prompt caching
// beta actually caches them across turns. Without this the
// prompt-caching-2024-07-31 header is sent but the request body has
// no breakpoints, so every turn re-ships and re-bills the static
// prefix (~3 KB in typical Claude Code sessions). F-303.
func injectCacheControl(reqBody map[string]interface{}) {
	if sys, ok := reqBody["system"].(string); ok && sys != "" {
		reqBody["system"] = []map[string]interface{}{{
			"type":          "text",
			"text":          sys,
			"cache_control": map[string]string{"type": "ephemeral"},
		}}
	}
	if tools, ok := reqBody["tools"].([]interface{}); ok && len(tools) > 0 {
		if last, ok := tools[len(tools)-1].(map[string]interface{}); ok {
			last["cache_control"] = map[string]string{"type": "ephemeral"}
		}
	}
}

// ChatCompletion streams the Anthropic API response via SSE, emitting Wails
// events for each token while collecting the full message. It returns the
// complete message JSON when the stream ends (backward-compatible).
func (a *App) ChatCompletion(apiKey, baseURL, model string, requestJSON string, protocol string, userAgent string, proxyID string, sessionID string) (string, error) {
	return a.chatCompletion(aiRequest{}, apiKey, baseURL, model, requestJSON, protocol, userAgent, proxyID, sessionID)
}

// ChatCompletionScoped keeps the generated RPC's cancellable context and routes
// every stream event to this request, including failures before streaming starts.
func (a *App) ChatCompletionScoped(ctx context.Context, requestID, apiKey, baseURL, model, requestJSON, protocol, userAgent, proxyID, sessionID string) (string, error) {
	if requestID == "" || len(requestID) > 128 || strings.ContainsAny(requestID, ":\r\n") {
		return "", fmt.Errorf("invalid AI request ID")
	}
	return a.chatCompletion(aiRequest{ctx: ctx, id: requestID}, apiKey, baseURL, model, requestJSON, protocol, userAgent, proxyID, sessionID)
}

type aiRequest struct {
	ctx  context.Context
	id   string
	emit func(string, ...any)
}

func (a *App) aiEmitter(r aiRequest) func(string, ...any) {
	sink := r.emit
	if sink == nil {
		sink = a.emit
	}
	return func(name string, data ...any) {
		if r.id != "" {
			name += ":" + r.id
		}
		sink(name, data...)
	}
}

func (a *App) aiStream(requests []aiRequest) (context.Context, context.CancelFunc, func(string, ...any)) {
	r := aiRequest{}
	if len(requests) > 0 {
		r = requests[0]
	}
	if r.ctx == nil {
		r.ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(r.ctx, 600*time.Second)
	// Legacy callers retain CancelChatStream; it cannot cancel a scoped call.
	if r.id == "" {
		myCancel := cancel
		a.chatCancel.Store(&myCancel)
		return ctx, func() { cancel(); a.chatCancel.CompareAndSwap(&myCancel, nil) }, a.aiEmitter(r)
	}
	return ctx, cancel, a.aiEmitter(r)
}

func (a *App) chatCompletion(request aiRequest, apiKey, baseURL, model, requestJSON, protocol, userAgent, proxyID, sessionID string) (result string, err error) {
	if request.id != "" {
		defer func() {
			emit := a.aiEmitter(request)
			if err != nil {
				emit("ai:error", map[string]string{"error": err.Error()})
			}
			emit("ai:end", map[string]bool{"ok": err == nil})
		}()
	}
	// Parse the incoming request body (always Anthropic format from frontend)
	var reqBody map[string]interface{}
	if err := json.Unmarshal([]byte(requestJSON), &reqBody); err != nil {
		return "", fmt.Errorf("invalid request JSON: %w", err)
	}

	if userAgent == "" {
		userAgent = "uniTerm"
	}

	// Attachment guard rail: validate image blocks before anything is sent so
	// an oversized / malformed / over-count attachment produces a readable
	// error instead of an upstream 400 (FR-3, FR-6).
	if err := validateRequestAttachments(reqBody); err != nil {
		return "", err
	}

	client, err := a.llmClientFor(proxyID)
	if err != nil {
		return "", err
	}

	switch protocol {
	case "openai":
		return a.chatCompletionOpenAI(apiKey, baseURL, model, reqBody, userAgent, sessionID, client, request)
	case "responses":
		return a.chatCompletionResponses(apiKey, baseURL, model, reqBody, userAgent, client, request)
	}
	return a.chatCompletionAnthropic(apiKey, baseURL, model, reqBody, userAgent, client, request)
}

// llmClientFor returns the HTTP client for the model's proxy selection:
// an empty reference dials direct, session.ProxyIDSystem uses the shared
// system-proxy client (see llmProxy), and a saved proxy ID builds a client
// routed through that proxy (F-208 connection reuse applies per client).
func (a *App) llmClientFor(proxyID string) (*http.Client, error) {
	switch proxyID {
	case "":
		return llmDirectClient(), nil
	case session.ProxyIDSystem:
		return a.llmHTTPClient(), nil
	}
	resolve, err := a.proxyResolver()
	if err != nil {
		return nil, err
	}
	p, ok := resolve(proxyID)
	if !ok {
		return nil, fmt.Errorf("referenced proxy not found or disabled: %s", proxyID)
	}
	return llmHTTPClientFor(&p), nil
}

// llmHTTPClientFor returns an *http.Client whose transport routes through the
// given upstream proxy (HTTP CONNECT for https URLs, SOCKS5 natively).
func llmHTTPClientFor(proxy *session.SocksProxy) *http.Client {
	tr := &http.Transport{
		Proxy: func(*http.Request) (*url.URL, error) {
			return proxyTransportURL(proxy)
		},
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 2 * time.Second,
	}
	return &http.Client{Transport: tr}
}

// proxyTransportURL renders a saved SOCKS5/HTTP proxy as a proxy URL for
// http.Transport. http.Transport natively speaks SOCKS5 via the socks5 scheme.
func proxyTransportURL(p *session.SocksProxy) (*url.URL, error) {
	u := &url.URL{Scheme: p.Kind, Host: net.JoinHostPort(p.Host, strconv.Itoa(p.Port))}
	if p.User != "" {
		u.User = url.UserPassword(p.User, p.Pass)
	}
	return u, nil
}

// chatCompletionAnthropic handles the native Anthropic Messages API with SSE streaming.
func (a *App) chatCompletionAnthropic(apiKey, baseURL, model string, reqBody map[string]interface{}, userAgent string, client *http.Client, requests ...aiRequest) (string, error) {
	// Computed once so a non-200 response can be reported as a readable
	// attachment error when the request carried images (FR-6).
	hasImages := requestHasImageAttachment(reqBody)

	reqBody["stream"] = true

	modifiedJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal modified request: %w", err)
	}

	// Anthropic base URL conventionally omits /v1 (client appends /v1/messages).
	// Tolerate legacy configs that already include the /v1 suffix.
	base := strings.TrimRight(baseURL, "/")
	var url string
	if strings.HasSuffix(base, "/v1") {
		url = base + "/messages"
	} else {
		url = base + "/v1/messages"
	}

	ctx, cancel, emit := a.aiStream(requests)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(modifiedJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")
	req.Header.Set("User-Agent", userAgent)

	// Use the shared client so this path honors the same system/env proxy
	// resolution and connection pool as the OpenAI/Responses paths.
	res, err := client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("AI_REQUEST_TIMEOUT")
		}
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		// F-305: cap error-body reads at 64 KiB.
		body, _ := io.ReadAll(io.LimitReader(res.Body, 64*1024))
		// A vision-less model answers a multimodal request with a 4xx; report
		// that as a readable attachment error instead of the raw upstream body.
		return "", upstreamError(res.StatusCode, body, hasImages)
	}

	var contentBlocks []map[string]interface{}
	var currentBlock map[string]interface{}
	var messageRole string
	var usage map[string]interface{}
	currentBlockIndex := -1

	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		dataStr := line[6:]

		var event map[string]interface{}
		if err := json.Unmarshal([]byte(dataStr), &event); err != nil {
			continue
		}

		eventType, _ := event["type"].(string)

		switch eventType {
		case "message_start":
			if msg, ok := event["message"].(map[string]interface{}); ok {
				messageRole, _ = msg["role"].(string)
			}

		case "content_block_start":
			currentBlockIndex++
			if block, ok := event["content_block"].(map[string]interface{}); ok {
				currentBlock = block
				emit("ai:block_start", map[string]interface{}{
					"index":         currentBlockIndex,
					"content_block": block,
				})
			}

		case "content_block_delta":
			delta, _ := event["delta"].(map[string]interface{})
			deltaType, _ := delta["type"].(string)

			if deltaType == "text_delta" {
				text, _ := delta["text"].(string)
				if currentBlock != nil {
					if currentBlock["text"] == nil {
						currentBlock["text"] = ""
					}
					currentBlock["text"] = currentBlock["text"].(string) + text
				}
				emit("ai:token", map[string]interface{}{
					"text":  text,
					"index": currentBlockIndex,
				})
			}
			if deltaType == "thinking_delta" && currentBlock != nil {
				thinking, _ := delta["thinking"].(string)
				if thinking == "" {
					continue
				}
				if s, ok := currentBlock["thinking"].(string); ok {
					currentBlock["thinking"] = s + thinking
				} else {
					currentBlock["thinking"] = thinking
				}
				emit("ai:thinking", aiThinkingEvent{
					Text:  thinking,
					Index: currentBlockIndex,
				})
			}
			if deltaType == "signature_delta" && currentBlock != nil {
				sig, _ := delta["signature"].(string)
				if s, ok := currentBlock["signature"].(string); ok {
					currentBlock["signature"] = s + sig
				} else {
					currentBlock["signature"] = sig
				}
			}
			if deltaType == "input_json_delta" && currentBlock != nil {
				partial, _ := delta["partial_json"].(string)
				if currentBlock["input"] == nil || fmt.Sprintf("%T", currentBlock["input"]) != "string" {
					currentBlock["input"] = ""
				}
				if s, ok := currentBlock["input"].(string); ok {
					currentBlock["input"] = s + partial
				}
			}

		case "content_block_stop":
			if currentBlock != nil {
				if blockType, _ := currentBlock["type"].(string); blockType == "tool_use" {
					if inputStr, ok := currentBlock["input"].(string); ok && inputStr != "" {
						var inputObj map[string]interface{}
						if err := json.Unmarshal([]byte(inputStr), &inputObj); err == nil {
							currentBlock["input"] = inputObj
						}
					}
				}
				contentBlocks = append(contentBlocks, currentBlock)
				currentBlock = nil
			}

		case "message_delta":
			if u, ok := event["usage"].(map[string]interface{}); ok {
				usage = u
			}
			if delta, ok := event["delta"].(map[string]interface{}); ok {
				if stopReason, ok := delta["stop_reason"].(string); ok {
					emit("ai:done", map[string]interface{}{
						"message": map[string]interface{}{
							"role":    messageRole,
							"content": contentBlocks,
						},
						"usage":       usage,
						"stop_reason": stopReason,
					})
				}
			}

		case "message_stop":
			fullMessage := map[string]interface{}{
				"role":    messageRole,
				"content": contentBlocks,
			}
			resultJSON, err := json.Marshal(fullMessage)
			if err != nil {
				return "", fmt.Errorf("marshal full message: %w", err)
			}
			return string(resultJSON), nil

		case "error":
			errData, _ := event["error"].(map[string]interface{})
			errMsg, _ := errData["message"].(string)
			return "", fmt.Errorf("stream error: %s", errMsg)
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	if len(contentBlocks) > 0 {
		fullMessage := map[string]interface{}{
			"role":    messageRole,
			"content": contentBlocks,
		}
		resultJSON, _ := json.Marshal(fullMessage)
		return string(resultJSON), nil
	}

	return "", fmt.Errorf("stream ended without message_stop")
}

// marshalAnthropicFinalMessage encodes a final message using a pooled
// *bytes.Buffer to avoid per-turn allocator churn in heavy sessions.
func marshalAnthropicFinalMessage(msg map[string]interface{}) ([]byte, error) {
	buf := finalMsgPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer finalMsgPool.Put(buf)
	enc := json.NewEncoder(buf)
	if err := enc.Encode(msg); err != nil {
		return nil, err
	}
	// json.Encoder always appends a trailing newline; trim it.
	out := buf.Bytes()
	if n := len(out); n > 0 && out[n-1] == '\n' {
		out = out[:n-1]
	}
	return out, nil
}

func anthropicToolToOpenAI(t map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        t["name"],
			"description": t["description"],
			"parameters":  t["input_schema"],
		},
	}
}

// convertAnthropicMessageToOpenAI converts one Anthropic-format message to OpenAI format.
func convertAnthropicMessageToOpenAI(msg map[string]interface{}) []map[string]interface{} {
	role, _ := msg["role"].(string)
	content := msg["content"]

	var results []map[string]interface{}

	switch role {
	case "user":
		out := map[string]interface{}{"role": "user"}
		// Image attachments force OpenAI's multi-part content form. Text-only
		// messages keep the plain string form they have always used, so the
		// existing text/tool_result behaviour is untouched.
		var parts []map[string]interface{}
		hasImage := false
		if contentStr, ok := content.(string); ok {
			out["content"] = contentStr
		} else if contentBlocks, ok := content.([]interface{}); ok {
			for _, block := range contentBlocks {
				if b, ok := block.(map[string]interface{}); ok {
					if bType, _ := b["type"].(string); bType == "text" {
						if hasImage {
							parts = append(parts, map[string]interface{}{"type": "text", "text": toString(b["text"])})
						} else {
							out["content"] = b["text"]
						}
					}
					if bType, _ := b["type"].(string); bType == "image" {
						dataURL, err := anthropicImageBlockToDataURL(b)
						if err != nil {
							// Validated before dispatch; skip rather than emit a
							// half-built request if a caller bypassed that guard.
							continue
						}
						if !hasImage {
							hasImage = true
							// Re-seed any text seen before the first image so
							// block order (and therefore prompt order) survives.
							if s, ok := out["content"].(string); ok && s != "" {
								parts = append(parts, map[string]interface{}{"type": "text", "text": s})
							}
							delete(out, "content")
						}
						parts = append(parts, map[string]interface{}{
							"type":      "image_url",
							"image_url": map[string]interface{}{"url": dataURL},
						})
					}
					if bType, _ := b["type"].(string); bType == "tool_result" {
						toolMsg := map[string]interface{}{
							"role":         "tool",
							"tool_call_id": b["tool_use_id"],
							"content":      toString(b["content"]),
						}
						results = append(results, toolMsg)
					}
				}
			}
		}
		if hasImage {
			out["content"] = parts
		}
		// Emit tool messages first, then any text user message. An OpenAI-format
		// assistant message with tool_calls must be immediately followed by the
		// matching tool messages; a user text block placed before them triggers a
		// 400 "insufficient tool messages following tool_calls" error.
		if _, hasContent := out["content"]; hasContent {
			results = append(results, out)
		}

	case "assistant":
		out := map[string]interface{}{"role": "assistant"}
		var toolCalls []map[string]interface{}
		if contentStr, ok := content.(string); ok {
			out["content"] = contentStr
		} else if contentBlocks, ok := content.([]interface{}); ok {
			for _, block := range contentBlocks {
				if b, ok := block.(map[string]interface{}); ok {
					if bType, _ := b["type"].(string); bType == "text" {
						out["content"] = b["text"]
					}
					if bType, _ := b["type"].(string); bType == "tool_use" {
						argsStr := "{}"
						if input, ok := b["input"]; ok {
							argsBytes, _ := json.Marshal(input)
							argsStr = string(argsBytes)
						}
						toolCalls = append(toolCalls, map[string]interface{}{
							"id":   b["id"],
							"type": "function",
							"function": map[string]interface{}{
								"name":      b["name"],
								"arguments": argsStr,
							},
						})
					}
				}
			}
		}
		if len(toolCalls) > 0 {
			out["tool_calls"] = toolCalls
		}
		// A message whose only blocks were thinking/signature would otherwise
		// serialize with neither content nor tool_calls and be rejected.
		if _, hasContent := out["content"]; !hasContent && len(toolCalls) == 0 {
			out["content"] = ""
		}
		results = append([]map[string]interface{}{out}, results...)

	default:
		out := map[string]interface{}{"role": role}
		if contentStr, ok := content.(string); ok {
			out["content"] = contentStr
		}
		results = append([]map[string]interface{}{out}, results...)
	}

	return results
}

func toString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func isOpenCodeGoBaseURL(baseURL string) bool {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return false
	}

	if !strings.EqualFold(u.Hostname(), "opencode.ai") {
		return false
	}

	path := strings.TrimRight(u.Path, "/")
	return path == "/zen/go/v1" || strings.HasPrefix(path, "/zen/go/v1/")
}

// chatCompletionOpenAI converts the Anthropic-format request to OpenAI,
// calls the OpenAI Chat Completions API with SSE streaming, and converts
// the response back to Anthropic format so the frontend sees no difference.
func (a *App) chatCompletionOpenAI(apiKey, baseURL, model string, reqBody map[string]interface{}, userAgent string, sessionID string, client *http.Client, requests ...aiRequest) (string, error) {
	// Computed once so a non-200 response can be reported as a readable
	// attachment error when the request carried images (FR-6).
	hasImages := requestHasImageAttachment(reqBody)

	url := strings.TrimRight(baseURL, "/") + "/chat/completions"

	// --- Build OpenAI-format request body ---
	openaiBody := map[string]interface{}{
		"model":      model,
		"stream":     true,
		"max_tokens": reqBody["max_tokens"],
	}

	// Convert tools
	if tools, ok := reqBody["tools"].([]interface{}); ok {
		var oaiTools []map[string]interface{}
		for _, t := range tools {
			if tm, ok := t.(map[string]interface{}); ok {
				oaiTools = append(oaiTools, anthropicToolToOpenAI(tm))
			}
		}
		if len(oaiTools) > 0 {
			openaiBody["tools"] = oaiTools
		}
	}

	// Convert messages + system
	var oaiMessages []map[string]interface{}
	if system, ok := reqBody["system"].(string); ok && system != "" {
		oaiMessages = append(oaiMessages, map[string]interface{}{
			"role":    "system",
			"content": system,
		})
	}
	if msgs, ok := reqBody["messages"].([]interface{}); ok {
		for _, m := range msgs {
			if mm, ok := m.(map[string]interface{}); ok {
				converted := convertAnthropicMessageToOpenAI(mm)
				oaiMessages = append(oaiMessages, converted...)
			}
		}
	}
	openaiBody["messages"] = oaiMessages

	requestJSON, err := json.Marshal(openaiBody)
	if err != nil {
		return "", fmt.Errorf("marshal openai request: %w", err)
	}

	ctx, cancel, emit := a.aiStream(requests)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(requestJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", userAgent)

	if sessionID != "" && isOpenCodeGoBaseURL(baseURL) {
		req.Header.Set("x-opencode-session", sessionID)
	}

	res, err := client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("AI_REQUEST_TIMEOUT")
		}
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		// F-305: cap the error-body read at 64 KiB so a hostile or
		// buggy upstream returning a multi-GB error body can't OOM
		// the Go process.
		body, _ := io.ReadAll(io.LimitReader(res.Body, 64*1024))
		// A vision-less model answers a multimodal request with a 4xx; report
		// that as a readable attachment error instead of the raw upstream body.
		return "", upstreamError(res.StatusCode, body, hasImages)
	}

	// --- Parse OpenAI SSE stream, emit Anthropic-format events ---
	var contentBlocks []map[string]interface{}
	var currentBlock map[string]interface{}
	var messageRole = "assistant"
	currentBlockIndex := -1
	activeToolCalls := make(map[int]map[string]interface{}) // index -> accumulating tool_call
	// F-307: per-block text and input buffers so accumulation is O(n)
	// instead of O(n²) string concat per token. Flushed to the block
	// map on content_block_stop / finish_reason.
	var currentTextBuf, currentInputBuf bytes.Buffer
	// Per-tool input buffer so each tool_call's argument concat stays
	// O(n). Keyed by the tool's index — multiple tool_calls can run
	// in parallel (one per idx).
	toolInputBufs := make(map[int]*bytes.Buffer)

	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// Accumulating buffer for an open "thinking" block (OpenAI-compatible
	// reasoning models stream delta.reasoning_content before/around content).
	var reasoningBuf bytes.Buffer

	flushCurrentTextBlock := func() {
		if currentBlock == nil {
			return
		}
		switch t, _ := currentBlock["type"].(string); t {
		case "text":
			if currentTextBuf.Len() > 0 {
				currentBlock["text"] = currentTextBuf.String()
			}
			currentTextBuf.Reset()
		case "thinking":
			if reasoningBuf.Len() > 0 {
				currentBlock["thinking"] = reasoningBuf.String()
			}
			reasoningBuf.Reset()
		}
		contentBlocks = append(contentBlocks, currentBlock)
		currentBlock = nil
	}

	// Emit message_start at the beginning
	// F-320: typed payload (frontend reads event.message.role).
	emit("ai:message_start", aiMessageStartEvent{
		Role: "assistant",
	})

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		dataStr := line[6:]

		if strings.TrimSpace(dataStr) == "[DONE]" {
			// Emit content_block_stop for any open block
			if currentBlock != nil {
				flushCurrentTextBlock()
				emit("ai:content_block_stop", aiContentBlockStopEvent{
					Index: currentBlockIndex,
				})
			}
			// Close any open tool_use blocks
			for idx, tc := range activeToolCalls {
				contentBlocks = append(contentBlocks, tc)
				emit("ai:content_block_stop", aiContentBlockStopEvent{
					Index: idx,
				})
			}
			activeToolCalls = make(map[int]map[string]interface{})

			// Emit message_delta and message_stop
			// F-320: typed payload.
			emit("ai:done", aiDoneEvent{
				Message: map[string]interface{}{
					"role":    messageRole,
					"content": contentBlocks,
				},
				StopReason: "end_turn",
			})

			fullMessage := map[string]interface{}{
				"role":    messageRole,
				"content": contentBlocks,
			}
			resultJSON, _ := json.Marshal(fullMessage)
			return string(resultJSON), nil
		}

		var ev openaiStreamEvent
		if err := json.Unmarshal([]byte(dataStr), &ev); err != nil {
			continue
		}
		if len(ev.Choices) == 0 {
			continue
		}
		choice := ev.Choices[0]
		delta := choice.Delta

		// Handle reasoning/thinking content (DeepSeek-R1, QwQ, o-series via
		// compatible gateways). Accumulated into a thinking block so it is
		// preserved in message history, and streamed live via ai:thinking.
		if delta.ReasoningContent != "" {
			if currentBlock == nil || currentBlock["type"] != "thinking" {
				// Close previous block if any
				if currentBlock != nil {
					flushCurrentTextBlock()
					emit("ai:content_block_stop", aiContentBlockStopEvent{
						Index: currentBlockIndex,
					})
				}
				currentBlockIndex++
				currentBlock = map[string]interface{}{
					"type":     "thinking",
					"thinking": "",
				}
				reasoningBuf.Reset()
				emit("ai:block_start", aiBlockStartEvent{
					Index:        currentBlockIndex,
					ContentBlock: map[string]interface{}{"type": "thinking"},
				})
			}
			reasoningBuf.WriteString(delta.ReasoningContent)
			emit("ai:thinking", aiThinkingEvent{
				Text:  delta.ReasoningContent,
				Index: currentBlockIndex,
			})
		}

		// Handle text content
		if delta.Content != "" {
			if currentBlock == nil || currentBlock["type"] != "text" {
				// Close previous block if any
				if currentBlock != nil {
					flushCurrentTextBlock()
					emit("ai:content_block_stop", aiContentBlockStopEvent{
						Index: currentBlockIndex,
					})
				}
				currentBlockIndex++
				currentBlock = map[string]interface{}{
					"type": "text",
					"text": "",
				}
				currentTextBuf.Reset()
				// F-320: typed payload.
				emit("ai:block_start", aiBlockStartEvent{
					Index:        currentBlockIndex,
					ContentBlock: currentBlock,
				})
			}
			currentTextBuf.WriteString(delta.Content)
			// F-320: typed struct + dropped unused fields — see
			// chatCompletionAnthropic for rationale.
			emit("ai:token", aiTokenEvent{
				Text:  delta.Content,
				Index: currentBlockIndex,
			})
		}

		// Handle tool_calls in delta
		for _, tc := range delta.ToolCalls {
			if tc.Index == nil {
				continue
			}
			idx := *tc.Index

			if _, exists := activeToolCalls[idx]; !exists {
				// Close current text block if open
				if currentBlock != nil {
					flushCurrentTextBlock()
					emit("ai:content_block_stop", aiContentBlockStopEvent{
						Index: currentBlockIndex,
					})
				}
				currentBlockIndex++
				activeToolCalls[idx] = map[string]interface{}{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  "",
					"input": "",
				}
				// F-320: typed payload.
				emit("ai:block_start", aiBlockStartEvent{
					Index: currentBlockIndex,
					ContentBlock: map[string]interface{}{
						"type": "tool_use",
						"id":   tc.ID,
					},
				})
			}

			atc := activeToolCalls[idx]
			if tc.Function.Name != "" {
				atc["name"] = tc.Function.Name
			}
			if args := tc.Function.Arguments; args != "" {
				// F-307: append to a per-tool *bytes.Buffer instead of
				// string concat (O(n²) over a long tool-args stream).
				buf, ok := toolInputBufs[idx]
				if !ok {
					buf = &bytes.Buffer{}
					toolInputBufs[idx] = buf
				}
				buf.WriteString(args)
				// F-320: typed payload.
				emit("ai:input_json_delta", aiInputJsonDeltaEvent{
					PartialJSON: args,
				})
			}
		}

		// Handle finish_reason on the choice level
		finishReason := choice.FinishReason
		if finishReason != "" && finishReason != "null" {
			// Close any open text block
			if currentBlock != nil {
				flushCurrentTextBlock()
				emit("ai:content_block_stop", aiContentBlockStopEvent{
					Index: currentBlockIndex,
				})
			}
			// Close tool_use blocks and parse their input JSON
			for idx, tc := range activeToolCalls {
				// F-307: prefer the per-tool buffer over the
				// possibly-empty tc["input"] string.
				if buf, ok := toolInputBufs[idx]; ok && buf.Len() > 0 {
					inputStr := buf.String()
					var inputObj map[string]interface{}
					if err := json.Unmarshal([]byte(inputStr), &inputObj); err == nil {
						tc["input"] = inputObj
					} else {
						tc["input"] = inputStr
					}
				} else if inputStr, ok := tc["input"].(string); ok && inputStr != "" {
					var inputObj map[string]interface{}
					if err := json.Unmarshal([]byte(inputStr), &inputObj); err == nil {
						tc["input"] = inputObj
					}
				}
				contentBlocks = append(contentBlocks, tc)
				emit("ai:content_block_stop", aiContentBlockStopEvent{
					Index: idx,
				})
			}
			activeToolCalls = make(map[int]map[string]interface{})
			toolInputBufs = nil
			currentInputBuf.Reset()

			stopReason := "end_turn"
			if finishReason == "tool_calls" {
				stopReason = "tool_use"
			} else if finishReason == "length" {
				stopReason = "max_tokens"
			} else if finishReason == "stop" {
				stopReason = "end_turn"
			}

			// F-320: typed payload.
			emit("ai:done", aiDoneEvent{
				Message: map[string]interface{}{
					"role":    messageRole,
					"content": contentBlocks,
				},
				StopReason: stopReason,
			})

			fullMessage := map[string]interface{}{
				"role":    messageRole,
				"content": contentBlocks,
			}
			resultJSON, _ := json.Marshal(fullMessage)
			return string(resultJSON), nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	if len(contentBlocks) > 0 || len(activeToolCalls) > 0 {
		for _, tc := range activeToolCalls {
			contentBlocks = append(contentBlocks, tc)
		}
		fullMessage := map[string]interface{}{
			"role":    messageRole,
			"content": contentBlocks,
		}
		resultJSON, _ := json.Marshal(fullMessage)
		return string(resultJSON), nil
	}

	return "", fmt.Errorf("stream ended without completion")
}

// anthropicToolToResponses converts an Anthropic tool definition to the
// Responses API function format (flat, unlike Chat Completions' nested form).
func anthropicToolToResponses(t map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"type":        "function",
		"name":        t["name"],
		"description": t["description"],
		"parameters":  t["input_schema"],
	}
}

// convertAnthropicMessageToResponses converts one Anthropic-format message to
// Responses API input items. Text turns become message items with
// input_text/output_text; tool_use becomes function_call; tool_result becomes
// function_call_output.
func convertAnthropicMessageToResponses(msg map[string]interface{}) []map[string]interface{} {
	role, _ := msg["role"].(string)
	content := msg["content"]

	var results []map[string]interface{}

	textType := "input_text"
	if role == "assistant" {
		textType = "output_text"
	}

	if contentStr, ok := content.(string); ok {
		if contentStr != "" {
			results = append(results, map[string]interface{}{
				"role": role,
				"content": []map[string]interface{}{
					{"type": textType, "text": contentStr},
				},
			})
		}
		return results
	}

	contentBlocks, ok := content.([]interface{})
	if !ok {
		return results
	}

	var textParts []map[string]interface{}
	for _, block := range contentBlocks {
		b, ok := block.(map[string]interface{})
		if !ok {
			continue
		}
		switch b["type"] {
		case "text":
			if txt, _ := b["text"].(string); txt != "" {
				textParts = append(textParts, map[string]interface{}{"type": textType, "text": txt})
			}
		case "image":
			// Images are only ever sent by the user; the Responses API takes a
			// flat input_image part whose image_url is the data URL itself
			// (unlike Chat Completions' nested image_url object).
			if role == "assistant" {
				continue
			}
			dataURL, err := anthropicImageBlockToDataURL(b)
			if err != nil {
				// Validated before dispatch; skip rather than emit a malformed item.
				continue
			}
			textParts = append(textParts, map[string]interface{}{
				"type":      "input_image",
				"image_url": dataURL,
			})
		case "tool_use":
			argsStr := "{}"
			if input, ok := b["input"]; ok {
				argsBytes, _ := json.Marshal(input)
				argsStr = string(argsBytes)
			}
			results = append(results, map[string]interface{}{
				"type":      "function_call",
				"call_id":   b["id"],
				"name":      b["name"],
				"arguments": argsStr,
			})
		case "tool_result":
			results = append(results, map[string]interface{}{
				"type":    "function_call_output",
				"call_id": b["tool_use_id"],
				"output":  toString(b["content"]),
			})
		}
	}

	if len(textParts) > 0 {
		msgItem := map[string]interface{}{"role": role, "content": textParts}
		if role == "assistant" {
			results = append([]map[string]interface{}{msgItem}, results...)
		} else {
			results = append(results, msgItem)
		}
	}

	return results
}

// chatCompletionResponses converts the Anthropic-format request to the OpenAI
// Responses API, calls /responses with SSE streaming, and converts the response
// events back to Anthropic-format events so the frontend sees no difference.
// Stateless: full history is sent as `input` each turn; reasoning items are ignored.
func (a *App) chatCompletionResponses(apiKey, baseURL, model string, reqBody map[string]interface{}, userAgent string, client *http.Client, requests ...aiRequest) (string, error) {
	// Computed once so a non-200 response can be reported as a readable
	// attachment error when the request carried images (FR-6).
	hasImages := requestHasImageAttachment(reqBody)

	url := strings.TrimRight(baseURL, "/") + "/responses"

	// --- Build Responses-format request body ---
	respBody := map[string]interface{}{
		"model":  model,
		"stream": true,
	}
	if mt, ok := reqBody["max_tokens"]; ok {
		respBody["max_output_tokens"] = mt
	}
	if system, ok := reqBody["system"].(string); ok && system != "" {
		respBody["instructions"] = system
	}

	if tools, ok := reqBody["tools"].([]interface{}); ok {
		var respTools []map[string]interface{}
		for _, t := range tools {
			if tm, ok := t.(map[string]interface{}); ok {
				respTools = append(respTools, anthropicToolToResponses(tm))
			}
		}
		if len(respTools) > 0 {
			respBody["tools"] = respTools
		}
	}

	var input []map[string]interface{}
	if msgs, ok := reqBody["messages"].([]interface{}); ok {
		for _, m := range msgs {
			if mm, ok := m.(map[string]interface{}); ok {
				input = append(input, convertAnthropicMessageToResponses(mm)...)
			}
		}
	}
	respBody["input"] = input

	requestJSON, err := json.Marshal(respBody)
	if err != nil {
		return "", fmt.Errorf("marshal responses request: %w", err)
	}

	ctx, cancel, emit := a.aiStream(requests)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(requestJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", userAgent)

	res, err := client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("AI_REQUEST_TIMEOUT")
		}
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		// F-305: cap the error-body read at 64 KiB so a hostile or
		// buggy upstream returning a multi-GB error body can't OOM
		// the Go process.
		body, _ := io.ReadAll(io.LimitReader(res.Body, 64*1024))
		// A vision-less model answers a multimodal request with a 4xx; report
		// that as a readable attachment error instead of the raw upstream body.
		return "", upstreamError(res.StatusCode, body, hasImages)
	}

	// --- Parse Responses SSE stream, emit Anthropic-format events ---
	var contentBlocks []map[string]interface{}
	// Map Responses output_index -> our content block index / accumulating block.
	blockByOutputIdx := make(map[int]map[string]interface{})
	idxByOutputIdx := make(map[int]int)
	nextBlockIndex := 0
	// F-307: parallel maps of *bytes.Buffer so text/input accumulation
	// is O(n) instead of O(n²) string concat per token. Outputs may run
	// in parallel (different output_index) so a single shared buffer
	// doesn't work — keep one per output_index.
	textBufs := make(map[int]*bytes.Buffer)
	inputBufs := make(map[int]*bytes.Buffer)
	// Reasoning summary accumulation, keyed by output_index like the others.
	reasoningBufs := make(map[int]*bytes.Buffer)

	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// F-320: typed payload.
	emit("ai:message_start", aiMessageStartEvent{
		Role: "assistant",
	})

	finish := func(stopReason string) (string, error) {
		fullMessage := map[string]interface{}{
			"role":    "assistant",
			"content": contentBlocks,
		}
		resultJSON, err := json.Marshal(fullMessage)
		if err != nil {
			return "", fmt.Errorf("marshal final message: %w", err)
		}
		// F-320: typed payload with json.RawMessage so the
		// already-marshaled message bytes pass through untouched.
		emit("ai:done", struct {
			Message    json.RawMessage `json:"message"`
			StopReason string          `json:"stop_reason"`
		}{
			Message:    json.RawMessage(resultJSON),
			StopReason: stopReason,
		})
		return string(resultJSON), nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		dataStr := line[6:]
		if strings.TrimSpace(dataStr) == "[DONE]" {
			continue
		}

		var ev responsesStreamEvent
		if err := json.Unmarshal([]byte(dataStr), &ev); err != nil {
			continue
		}

		switch ev.Type {
		case "response.output_item.added":
			var item responsesStreamItem
			if err := json.Unmarshal(ev.Item, &item); err != nil {
				continue
			}
			switch item.Type {
			case "message":
				block := map[string]interface{}{"type": "text", "text": ""}
				blockByOutputIdx[ev.OutputIndex] = block
				idxByOutputIdx[ev.OutputIndex] = nextBlockIndex
				// F-320: typed payload.
				emit("ai:block_start", aiBlockStartEvent{
					Index:        nextBlockIndex,
					ContentBlock: block,
				})
				nextBlockIndex++
			case "reasoning":
				// Reasoning items: keep the summary text as a thinking block
				// (mirrors the OpenAI path) and stream it live via ai:thinking.
				block := map[string]interface{}{"type": "thinking", "thinking": ""}
				blockByOutputIdx[ev.OutputIndex] = block
				idxByOutputIdx[ev.OutputIndex] = nextBlockIndex
				emit("ai:block_start", aiBlockStartEvent{
					Index:        nextBlockIndex,
					ContentBlock: map[string]interface{}{"type": "thinking"},
				})
				nextBlockIndex++
			case "function_call":
				block := map[string]interface{}{
					"type":  "tool_use",
					"id":    item.CallID,
					"name":  item.Name,
					"input": "",
				}
				blockByOutputIdx[ev.OutputIndex] = block
				idxByOutputIdx[ev.OutputIndex] = nextBlockIndex
				// F-320: typed payload.
				emit("ai:block_start", aiBlockStartEvent{
					Index: nextBlockIndex,
					ContentBlock: map[string]interface{}{
						"type": "tool_use",
						"id":   item.CallID,
						"name": item.Name,
					},
				})
				nextBlockIndex++
			}

		case "response.output_text.delta":
			block := blockByOutputIdx[ev.OutputIndex]
			if block == nil {
				continue
			}
			if ev.Delta == "" {
				continue
			}
			// F-307: append to per-block *bytes.Buffer instead of
			// O(n²) string concatenation. Flushed on output_item.done.
			buf, ok := textBufs[ev.OutputIndex]
			if !ok {
				buf = &bytes.Buffer{}
				textBufs[ev.OutputIndex] = buf
			}
			buf.WriteString(ev.Delta)
			// F-320: typed struct + dropped unused fields — see
			// chatCompletionAnthropic for rationale.
			emit("ai:token", aiTokenEvent{
				Text:  ev.Delta,
				Index: idxByOutputIdx[ev.OutputIndex],
			})

		case "response.reasoning_summary_text.delta":
			block := blockByOutputIdx[ev.OutputIndex]
			if block == nil || ev.Delta == "" {
				continue
			}
			buf, ok := reasoningBufs[ev.OutputIndex]
			if !ok {
				buf = &bytes.Buffer{}
				reasoningBufs[ev.OutputIndex] = buf
			}
			buf.WriteString(ev.Delta)
			emit("ai:thinking", aiThinkingEvent{
				Text:  ev.Delta,
				Index: idxByOutputIdx[ev.OutputIndex],
			})

		case "response.function_call_arguments.delta":
			block := blockByOutputIdx[ev.OutputIndex]
			if block == nil {
				continue
			}
			if ev.Delta == "" {
				continue
			}
			buf, ok := inputBufs[ev.OutputIndex]
			if !ok {
				buf = &bytes.Buffer{}
				inputBufs[ev.OutputIndex] = buf
			}
			buf.WriteString(ev.Delta)
			// F-320: typed payload.
			emit("ai:input_json_delta", aiInputJsonDeltaEvent{
				PartialJSON: ev.Delta,
			})

		case "response.output_item.done":
			block := blockByOutputIdx[ev.OutputIndex]
			if block == nil {
				continue
			}
			// F-307: flush per-block buffers once into the block map.
			if buf, ok := textBufs[ev.OutputIndex]; ok {
				if buf.Len() > 0 {
					block["text"] = buf.String()
				}
				delete(textBufs, ev.OutputIndex)
			}
			if buf, ok := inputBufs[ev.OutputIndex]; ok {
				if buf.Len() > 0 {
					inputStr := buf.String()
					if block["type"] == "tool_use" {
						var inputObj map[string]interface{}
						if json.Unmarshal([]byte(inputStr), &inputObj) == nil {
							block["input"] = inputObj
						} else {
							block["input"] = map[string]interface{}{}
						}
					} else {
						block["input"] = inputStr
					}
				}
				delete(inputBufs, ev.OutputIndex)
			}
			if buf, ok := reasoningBufs[ev.OutputIndex]; ok {
				if buf.Len() > 0 && block["type"] == "thinking" {
					block["thinking"] = buf.String()
				}
				delete(reasoningBufs, ev.OutputIndex)
			}
			contentBlocks = append(contentBlocks, block)
			// F-320: typed payload.
			emit("ai:content_block_stop", aiContentBlockStopEvent{
				Index: idxByOutputIdx[ev.OutputIndex],
			})
			delete(blockByOutputIdx, ev.OutputIndex)

		case "response.completed":
			stopReason := "end_turn"
			for _, b := range contentBlocks {
				if b["type"] == "tool_use" {
					stopReason = "tool_use"
					break
				}
			}
			return finish(stopReason)

		case "response.failed", "error":
			// Marshal the typed event back out for the error message; the
			// caller doesn't need the original map shape.
			body, _ := json.Marshal(ev)
			return "", fmt.Errorf("responses stream error: %s", string(body))
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	if len(contentBlocks) > 0 {
		return finish("end_turn")
	}

	return "", fmt.Errorf("stream ended without completion")
}

// CancelChatStream cancels the currently active ChatCompletion stream.
func (a *App) CancelChatStream() {
	if c := a.chatCancel.Load(); c != nil {
		(*c)()
	}
}

// ModelInfo represents a model entry from the /v1/models response.
type ModelInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// FetchModels fetches the available model list. openai/responses hit an
// OpenAI-compatible /models endpoint with a Bearer token; anthropic hits
// /v1/models with the x-api-key + anthropic-version headers, mirroring
// chatCompletionAnthropic's URL and auth handling.
func (a *App) FetchModels(apiKey, baseURL, protocol string, proxyID string) ([]ModelInfo, error) {
	base := strings.TrimRight(baseURL, "/")

	var url string
	if protocol == "anthropic" {
		// Base URL conventionally omits /v1; tolerate legacy configs with it.
		if strings.HasSuffix(base, "/v1") {
			url = base + "/models"
		} else {
			url = base + "/v1/models"
		}
	} else {
		url = base + "/models"
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if protocol == "anthropic" {
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("User-Agent", "uniTerm")

	// F-208: share the same transport as the LLM clients so the model
	// list call also benefits from the keep-alive pool; the request
	// itself carries its own deadline via the per-request context —
	// generous enough to cover a slow first proxy handshake.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	client, err := a.llmClientFor(proxyID)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", res.StatusCode, string(body))
	}

	var result struct {
		Data []ModelInfo `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse models response: %w", err)
	}
	return result.Data, nil
}

// SkillsStore methods

func (a *App) ListSkills() ([]store.SkillMeta, error) {
	if a.skillsStore == nil {
		return nil, fmt.Errorf("skills store not initialized")
	}
	return a.skillsStore.List()
}

func (a *App) GetSkillBody(name string) (string, error) {
	if a.skillsStore == nil {
		return "", fmt.Errorf("skills store not initialized")
	}
	return a.skillsStore.GetBody(name)
}

func (a *App) GetSkillFile(name, relPath string) (string, error) {
	if a.skillsStore == nil {
		return "", fmt.Errorf("skills store not initialized")
	}
	return a.skillsStore.GetSkillFile(name, relPath)
}

func (a *App) ListSkillFiles(name string) (store.SkillFileList, error) {
	if a.skillsStore == nil {
		return store.SkillFileList{}, fmt.Errorf("skills store not initialized")
	}
	return a.skillsStore.ListSkillFiles(name)
}

func (a *App) SetSkillEnabled(name string, enabled bool) error {
	if a.skillsStore == nil {
		return fmt.Errorf("skills store not initialized")
	}
	return a.skillsStore.SetEnabled(name, enabled)
}

func (a *App) SetSkillLocked(name string, locked bool) error {
	if a.skillsStore == nil {
		return fmt.Errorf("skills store not initialized")
	}
	return a.skillsStore.SetLocked(name, locked)
}

func (a *App) SetSkillSortOrder(name string, order int) error {
	if a.skillsStore == nil {
		return fmt.Errorf("skills store not initialized")
	}
	return a.skillsStore.SetSortOrder(name, order)
}

func (a *App) DeleteSkill(name string) error {
	if a.skillsStore == nil {
		return fmt.Errorf("skills store not initialized")
	}
	return a.skillsStore.Delete(name)
}

func (a *App) ImportSkillFromDir(srcDir string) (string, error) {
	if a.skillsStore == nil {
		return "", fmt.Errorf("skills store not initialized")
	}
	return a.skillsStore.ImportFromDir(srcDir)
}

func (a *App) ImportSkillFromZip(zipPath string) (string, error) {
	if a.skillsStore == nil {
		return "", fmt.Errorf("skills store not initialized")
	}
	return a.skillsStore.ImportFromZip(zipPath)
}

func (a *App) CreateSkill(name, description, body string) error {
	if a.skillsStore == nil {
		return fmt.Errorf("skills store not initialized")
	}
	return a.skillsStore.CreateSkill(name, description, body)
}

func (a *App) SaveSkill(name, description, body string) error {
	if a.skillsStore == nil {
		return fmt.Errorf("skills store not initialized")
	}
	return a.skillsStore.SaveSkill(name, description, body)
}

// RecentStore methods

// F-320: typed payloads for the ai:* Wails events. Replacing the
// per-token `map[string]interface{}` literal with a fixed struct saves
// the alloc per event; the json.Marshal on the Wails side now writes
// the same JSON shape (lowercase keys) so the frontend contract is
// unchanged.
type aiTokenEvent struct {
	Text  string `json:"text"`
	Index int    `json:"index"`
}

type aiBlockStartEvent struct {
	Index        int                    `json:"index"`
	ContentBlock map[string]interface{} `json:"content_block"`
}

type aiContentBlockStopEvent struct {
	Index int `json:"index"`
}

type aiInputJsonDeltaEvent struct {
	PartialJSON string `json:"partial_json"`
}

// aiThinkingEvent carries streamed reasoning/thinking text (Anthropic
// thinking_delta, OpenAI delta.reasoning_content, or Responses reasoning
// summary deltas). The frontend appends these to the current assistant
// message's thinking buffer and renders them in the collapsible thinking box.
type aiThinkingEvent struct {
	Text  string `json:"text"`
	Index int    `json:"index"`
}

type aiMessageStartEvent struct {
	Role string `json:"role"`
}

type aiDoneEvent struct {
	Message    map[string]interface{} `json:"message"`
	Usage      map[string]interface{} `json:"usage,omitempty"`
	StopReason string                 `json:"stop_reason"`
}

var finalMsgPool = stdsync.Pool{
	New: func() any {
		b := &bytes.Buffer{}
		b.Grow(4 * 1024)
		return b
	},
}

// F-306: typed SSE shapes for OpenAI Chat Completions. Only the few
// fields the loop reads (delta.content, delta.tool_calls[], choice.finish_reason)
// get decoded; the rest is discarded by the json decoder.
type openaiDeltaToolCall struct {
	Index    *int   `json:"index,omitempty"`
	ID       string `json:"id,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

type openaiStreamDelta struct {
	Content          string                `json:"content"`
	ReasoningContent string                `json:"reasoning_content"`
	ToolCalls        []openaiDeltaToolCall `json:"tool_calls"`
}

type openaiStreamChoice struct {
	Delta        openaiStreamDelta `json:"delta"`
	FinishReason string            `json:"finish_reason"`
}

type openaiStreamEvent struct {
	Choices []openaiStreamChoice `json:"choices"`
}

// F-306: typed SSE shapes for OpenAI Responses events. The wrapper
// captures the discriminator + output_index; nested item fields are
// decoded lazily per branch so we skip the ~99% of fields the loop
// discards.
type responsesStreamItem struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Name   string `json:"name"`
}

type responsesStreamEvent struct {
	Type        string          `json:"type"`
	OutputIndex int             `json:"output_index"`
	Item        json.RawMessage `json:"item"`
	Delta       string          `json:"delta"`
}

// --- Attachment support (merged from app_ai_attachment.go) ---

// Attachment support for the AI assistant.
//
// uniTerm is a model proxy: it never performs OCR, image recognition or any
// other multimodal inference itself. Attachments are validated here and then
// forwarded verbatim to the upstream multimodal model, which does the actual
// understanding. The frontend builds Anthropic-format content blocks; this
// file is the backend half — the safety net that keeps a malformed or
// oversized payload from reaching (and being rejected by) the upstream API,
// plus the block translation the OpenAI / Responses converters need.
//
// The limits below are mirrored in frontend/src/services/attachments.ts
// (ATTACHMENT_LIMITS). Keep both in sync: the frontend owns the user-facing
// validation messages, this file owns the enforcement that cannot be bypassed
// from the renderer.
const (
	// AttachmentMaxImageBytes caps one image attachment (5 MiB decoded).
	AttachmentMaxImageBytes = 5 * 1024 * 1024
	// AttachmentMaxTextBytes caps one text-file attachment (5 MiB).
	AttachmentMaxTextBytes = 5 * 1024 * 1024
	// AttachmentMaxImageSide is the longest-edge pixel cap for one image.
	// Providers reject oversized frames independently of byte size —
	// Anthropic refuses anything above 8000 x 8000 px — and a tall or wide
	// screenshot can easily be under the byte cap yet over this one.
	AttachmentMaxImageSide = 8000
	// AttachmentMaxImagesPerMessage is the hard ceiling on images inside one
	// upstream message. The product rule is AttachmentImagesPerMessage (see
	// below), but a single API message can legitimately carry more: aiStore
	// merges consecutive user messages when an assistant turn produced nothing
	// (a failed request, a cancelled turn), so two pasted screenshots from two
	// turns become one message. This ceiling only stops a runaway payload; it
	// is deliberately above the product rule so that legitimate history is
	// never rejected.
	AttachmentMaxImagesPerMessage = 8
	// AttachmentMaxTextFilesPerMessage caps text files per user message.
	AttachmentMaxTextFilesPerMessage = 3
)

// Sentinel errors surfaced to the frontend. The renderer matches on these
// prefixes and renders a localized, human-readable message — upstream 400
// bodies are never shown to the user for attachment failures.
var (
	errAttachmentTooLarge        = errors.New("AI_ATTACHMENT_TOO_LARGE")
	errAttachmentUnsupportedType = errors.New("AI_ATTACHMENT_UNSUPPORTED_TYPE")
	errAttachmentTooMany         = errors.New("AI_ATTACHMENT_TOO_MANY")
	errAttachmentEmpty           = errors.New("AI_ATTACHMENT_EMPTY")
	errAttachmentInvalid         = errors.New("AI_ATTACHMENT_INVALID")
	// errAttachmentTooLargePixels is the pixel-side counterpart of
	// errAttachmentTooLarge: the bytes fit but the frame does not.
	errAttachmentTooLargePixels = errors.New("AI_ATTACHMENT_TOO_LARGE_PIXELS")
	// errAttachmentUnsupportedByModel is returned instead of the raw upstream
	// body when a request carrying images is rejected in a way that indicates
	// the target model / endpoint has no vision support.
	errAttachmentUnsupportedByModel = errors.New("AI_ATTACHMENT_UNSUPPORTED_BY_MODEL")
)

// attachmentImageMediaTypes is the image whitelist. Mirrors
// ATTACHMENT_IMAGE_TYPES in the frontend service.
var attachmentImageMediaTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/jpg":  true,
	"image/gif":  true,
	"image/webp": true,
}

// anthropicImageBlockToDataURL renders an Anthropic image content block as a
// data URL and enforces the image whitelist / size cap.
//
//	{"type":"image","source":{"type":"base64","media_type":"image/png","data":"..."}}
//	  -> "data:image/png;base64,..."
//
// Only base64 sources are accepted: the renderer always inlines clipboard and
// file bytes, so a remote URL source would mean the model fetches an
// arbitrary address on the user's behalf.
func anthropicImageBlockToDataURL(block map[string]interface{}) (string, error) {
	source, ok := block["source"].(map[string]interface{})
	if !ok {
		return "", errAttachmentInvalid
	}
	if srcType, _ := source["type"].(string); srcType != "base64" {
		return "", errAttachmentUnsupportedType
	}

	mediaType := strings.ToLower(strings.TrimSpace(stringField(source, "media_type")))
	if !attachmentImageMediaTypes[mediaType] {
		return "", errAttachmentUnsupportedType
	}

	data := strings.TrimSpace(stringField(source, "data"))
	if data == "" {
		return "", errAttachmentEmpty
	}

	// Decode to measure the *real* byte count: base64 length alone is a poor
	// proxy (padding would push a file that is exactly at the limit over it).
	// This also rejects payloads that are not valid base64 before they reach
	// the upstream API.
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", errAttachmentInvalid
	}
	if len(decoded) == 0 {
		return "", errAttachmentEmpty
	}
	if len(decoded) > AttachmentMaxImageBytes {
		return "", errAttachmentTooLarge
	}

	// Byte size is only half of the upstream contract: providers also cap the
	// frame itself (Anthropic: 8000 x 8000 px), and a long screenshot is often
	// tiny in bytes yet thousands of pixels tall. DecodeConfig reads just the
	// image header, so this stays cheap. The whitelist has already pinned the
	// media type to png/jpeg, so the registered decoders cover every payload
	// that can get here; an undecodable header means the "image" is corrupt
	// and would be rejected upstream anyway.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(decoded))
	if err != nil {
		return "", errAttachmentInvalid
	}
	if cfg.Width > AttachmentMaxImageSide || cfg.Height > AttachmentMaxImageSide {
		return "", errAttachmentTooLargePixels
	}

	return "data:" + mediaType + ";base64," + data, nil
}

func stringField(m map[string]interface{}, key string) string {
	s, _ := m[key].(string)
	return s
}

// validateRequestAttachments walks every message in an Anthropic-format
// request and enforces the per-message attachment limits. Runs before any
// upstream call so the user gets a readable, localized message instead of a
// provider 400.
func validateRequestAttachments(reqBody map[string]interface{}) error {
	msgs, ok := reqBody["messages"].([]interface{})
	if !ok {
		return nil
	}
	for _, m := range msgs {
		msg, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		blocks, ok := msg["content"].([]interface{})
		if !ok {
			continue
		}

		images := 0
		for _, raw := range blocks {
			block, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			if bType, _ := block["type"].(string); bType != "image" {
				continue
			}
			images++
			if images > AttachmentMaxImagesPerMessage {
				return errAttachmentTooMany
			}
			if _, err := anthropicImageBlockToDataURL(block); err != nil {
				return err
			}
		}
	}
	return nil
}

// requestHasImageAttachment reports whether any message carries an image
// block. Used to decide how to phrase an upstream rejection.
func requestHasImageAttachment(reqBody map[string]interface{}) bool {
	msgs, ok := reqBody["messages"].([]interface{})
	if !ok {
		return false
	}
	for _, m := range msgs {
		msg, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		blocks, ok := msg["content"].([]interface{})
		if !ok {
			continue
		}
		for _, raw := range blocks {
			if block, ok := raw.(map[string]interface{}); ok {
				if bType, _ := block["type"].(string); bType == "image" {
					return true
				}
			}
		}
	}
	return false
}

// visionRejectionPhrases are the ways providers say "I can't take an image".
var (
	visionSubjectWords     = []string{"image", "vision", "multimodal", "content type"}
	visionRejectionPhrases = []string{
		"not support", "unsupported", "not allowed", "invalid", "unknown", "unrecognized",
	}
	// sizeRejectionPhrases are the ways providers say "this image is too
	// big" — in pixels, since bytes are already capped before dispatch.
	// Both halves must match: an image subject and a size phrase, so a
	// "max_tokens is too large" error is never misclassified.
	sizeRejectionPhrases = []string{
		"exceed", "too large", "dimension", "resolution", "width", "height", "pixel",
	}
)

// isImageSizeRejection reports whether an upstream error message means "the
// image frame is over the provider's pixel limits" rather than "no vision".
// The local pre-flight uses the Anthropic limits, but other providers cap
// tighter, so a frame that passes locally can still be refused upstream.
func isImageSizeRejection(detail string) bool {
	lower := strings.ToLower(detail)
	if !isVisionSubject(lower) {
		return false
	}
	for _, phrase := range sizeRejectionPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

// isVisionSubject reports whether the error text is about images at all.
func isVisionSubject(lower string) bool {
	for _, word := range visionSubjectWords {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

// isVisionRejection reports whether an upstream error message means "this model
// cannot take images" rather than a genuine request bug. Providers word this
// both ways round ("does not support image input", "invalid image_url"), so the
// check is order-independent.
func isVisionRejection(detail string) bool {
	lower := strings.ToLower(detail)
	if !isVisionSubject(lower) {
		return false
	}
	for _, phrase := range visionRejectionPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

// upstreamError turns a non-200 upstream response into an error the UI can
// render. Attachment-bearing requests that fail with a vision-related 4xx are
// reported as errAttachmentUnsupportedByModel (plus the upstream's own
// human-readable `error.message`, never the raw JSON/HTML envelope); a
// pixel-limit refusal gets its own sentinel so the UI can suggest downscaling.
// Everything else keeps the pre-existing "HTTP <code>: <body>" behaviour.
func upstreamError(status int, body []byte, hasImages bool) error {
	if hasImages && status >= 400 && status < 500 {
		detail := upstreamErrorDetail(body)
		if isVisionRejection(detail) {
			return fmt.Errorf("%w: %s", errAttachmentUnsupportedByModel, detail)
		}
		if isImageSizeRejection(detail) {
			return fmt.Errorf("%w: %s", errAttachmentTooLargePixels, detail)
		}
	}
	return fmt.Errorf("HTTP %d: %s", status, string(body))
}

// upstreamErrorDetail extracts the human-readable message from an OpenAI- or
// Anthropic-shaped error body, collapsing whitespace and truncating so a
// hostile or HTML-returning gateway cannot flood the chat panel.
func upstreamErrorDetail(body []byte) string {
	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	detail := ""
	if json.Unmarshal(body, &parsed) == nil {
		switch {
		case parsed.Error.Message != "":
			detail = parsed.Error.Message
		case parsed.Message != "":
			detail = parsed.Message
		}
	}
	detail = strings.Join(strings.Fields(detail), " ")
	if len(detail) > 300 {
		detail = detail[:300] + "…"
	}
	return detail
}

// --- Local file drag-drop (merged from app_ai_attachment_file.go) ---
// Local file drag-drop support for the AI composer.
//
// The webview never receives dropped file content: the window runs with
// EnableFileDrop, so OS drops are intercepted at the native layer and arrive
// in the renderer as absolute paths (see the WindowFilesDropped re-emit in
// main.go). The renderer hands each dropped path to this binding, which reads
// the file off disk and returns its bytes base64-encoded; the renderer wraps
// them in a File and reuses the regular attachment pipeline (whitelist,
// per-kind caps, count limits, pixel check — attachments.ts). The caps here
// are the same un-bypassable guard rails as validateRequestAttachments.

// errAttachmentIsDirectory marks a dropped path that is a directory rather
// than a file; the renderer maps it to its own localized message.
var errAttachmentIsDirectory = errors.New("AI_ATTACHMENT_IS_DIRECTORY")

// maxLocalAttachmentReadBytes caps one dropped file read. Mirrors
// AttachmentMaxImageBytes / AttachmentMaxTextBytes (the same value) — the
// renderer owns the per-kind user-facing messages, this owns the enforcement.
const maxLocalAttachmentReadBytes = AttachmentMaxImageBytes

// LocalAttachmentFile is one local file handed to the AI composer's
// drag-drop flow.
type LocalAttachmentFile struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	DataB64 string `json:"dataB64"`
}

// ReadLocalAttachmentFile reads a local file dropped onto the AI composer.
// Directories, empty files and files over the size cap are rejected before
// reading; anything else is returned with its content base64-encoded.
func (a *App) ReadLocalAttachmentFile(path string) (LocalAttachmentFile, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return LocalAttachmentFile{}, errAttachmentInvalid
	}
	info, err := os.Stat(path)
	if err != nil {
		return LocalAttachmentFile{}, err
	}
	if info.IsDir() {
		return LocalAttachmentFile{}, errAttachmentIsDirectory
	}
	if info.Size() == 0 {
		return LocalAttachmentFile{}, errAttachmentEmpty
	}
	if info.Size() > maxLocalAttachmentReadBytes {
		return LocalAttachmentFile{}, errAttachmentTooLarge
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return LocalAttachmentFile{}, err
	}
	return LocalAttachmentFile{
		Name:    info.Name(),
		Size:    info.Size(),
		DataB64: base64.StdEncoding.EncodeToString(data),
	}, nil
}

// MCP client wiring: uniTerm connects to external MCP servers and feeds their
// tools into the built-in AI assistant. This is the CLIENT half; the SERVER
// half (uniTerm exposed to external agents) lives in app_mcp.go / backend/mcp.
// The two share no state and persist independently.

// ensureMCPClient lazily builds the manager over mcp_clients.json.
func (a *App) ensureMCPClient() *mcpclient.Manager {
	a.mcpClientOnce.Do(func() {
		if a.mcpClientStore == nil {
			a.mcpClientStore = store.NewMCPClientStore(a.dataDir)
			if a.credentialStore != nil {
				a.mcpClientStore.SetPasswordStore(a.credentialStore)
			}
		}
		a.mcpClient = mcpclient.NewManager(a.mcpClientStore)
	})
	return a.mcpClient
}

// StartMCPClient loads persisted external servers and connects the enabled
// ones. Called after stores are ready (alongside StartMCP).
func (a *App) StartMCPClient() {
	if a.dataDir == "" {
		return
	}
	mgr := a.ensureMCPClient()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mgr.Reload(ctx)
}

// ListMCPClients returns every persisted external server config.
func (a *App) ListMCPClients() ([]store.MCPClientConfig, error) {
	a.businessMu.Lock()
	defer a.businessMu.Unlock()
	a.ensureMCPClient()
	all, err := a.mcpClientStore.Load()
	if err != nil {
		return nil, err
	}
	for i := range all {
		all[i].Revision = ""
		all[i].Revision, err = store.Revision(all[i])
		if err != nil {
			return nil, err
		}
	}
	return all, nil
}

// SaveMCPClient upserts one external server by name, persists, and reloads
// live connections (a newly added/enabled server connects immediately; a
// disabled/edited one is dropped and re-dialed as needed).
func (a *App) SaveMCPClient(cfg store.MCPClientConfig) error {
	return a.saveMCPClient(cfg, "")
}

// UpdateMCPClient edits or renames an existing server in one store save.
func (a *App) UpdateMCPClient(originalName string, cfg store.MCPClientConfig) error {
	if strings.TrimSpace(originalName) == "" {
		return errors.New("original MCP client name is required")
	}
	return a.saveMCPClient(cfg, originalName)
}

func (a *App) saveMCPClient(cfg store.MCPClientConfig, originalName string) error {
	a.businessMu.Lock()
	defer a.businessMu.Unlock()
	cfg.Name = strings.TrimSpace(cfg.Name)
	if err := cfg.Validate(); err != nil {
		return err
	}
	mgr := a.ensureMCPClient()
	all, err := a.mcpClientStore.Load()
	if err != nil {
		return err
	}
	replaced := false
	if originalName != "" {
		for _, c := range all {
			if c.Name == cfg.Name && c.Name != originalName {
				return errors.New("an MCP client with this name already exists")
			}
		}
	}
	for i := range all {
		if (originalName == "" && all[i].Name == cfg.Name) || all[i].Name == originalName {
			revision, err := store.Revision(all[i])
			if err != nil {
				return err
			}
			if revision != cfg.Revision {
				return store.ErrConflict
			}
			cfg.Revision = ""
			all[i] = cfg
			replaced = true
			break
		}
	}
	if !replaced {
		if cfg.Revision != "" {
			return store.ErrConflict
		}
		if originalName != "" {
			return errors.New("MCP client no longer exists")
		}
		all = append(all, cfg)
	}
	if err := a.mcpClientStore.Save(all); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mgr.Reload(ctx)
	return nil
}

// RemoveMCPClient deletes a persisted server and closes its live connection.
func (a *App) RemoveMCPClient(name string) error {
	a.businessMu.Lock()
	defer a.businessMu.Unlock()
	mgr := a.ensureMCPClient()
	all, err := a.mcpClientStore.Load()
	if err != nil {
		return err
	}
	kept := all[:0]
	for _, c := range all {
		if c.Name != name {
			kept = append(kept, c)
		}
	}
	if err := a.mcpClientStore.Save(kept); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mgr.Reload(ctx)
	return nil
}

// TestMCPClient dials a candidate config once, lists its tools, and closes —
// used by the settings "test" button without persisting first.
func (a *App) TestMCPClient(cfg store.MCPClientConfig) ([]mcpclient.DiscoveredTool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return a.ensureMCPClient().TestConnection(ctx, cfg)
}

// ListMCPTools flattens healthy external servers' tools for the AI panel.
// Never errors out: a broken connection degrades to "no tools from it".
func (a *App) ListMCPTools() ([]mcpclient.DiscoveredTool, error) {
	if a.dataDir == "" {
		return []mcpclient.DiscoveredTool{}, nil
	}
	return a.ensureMCPClient().ListTools(), nil
}

// CallMCPTool invokes an external tool by its qualified name and returns text.
func (a *App) CallMCPTool(qualified string, args map[string]any) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return a.ensureMCPClient().CallTool(ctx, qualified, args)
}

// MCPClientStatuses reports per-server health for the settings UI.
func (a *App) MCPClientStatuses() []mcpclient.ConnStatus {
	if a.dataDir == "" {
		return nil
	}
	mgr := a.ensureMCPClient()
	configs, err := a.mcpClientStore.Load()
	if err != nil {
		log.Writef("mcpclient: statuses: %v", err)
		return nil
	}
	return mgr.Statuses(configs)
}
