package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ys-ll/uniterm/backend/store"
)

// TestLLMProxyEnvFallback verifies that llmProxy resolves proxies declared
// via the HTTP_PROXY/HTTPS_PROXY environment variables (go-ieproxy prefers
// explicit env vars over the OS system proxy config).
//
// Direct-connection and system-proxy branches are intentionally not unit
// tested: they depend on the host's real system proxy state and on net/http's
// process-wide env-proxy cache, so they cannot be made hermetic.
func TestLLMProxyEnvFallback(t *testing.T) {
	t.Setenv("HTTP_PROXY", "127.0.0.1:7890")
	t.Setenv("HTTPS_PROXY", "127.0.0.1:7891")
	t.Setenv("NO_PROXY", "")

	// Force a fresh resolution so the cached func sees the test env.
	llmProxyMu.Lock()
	llmProxyFn = nil
	llmProxyMu.Unlock()

	req, err := http.NewRequest("GET", "https://api.openai.com/models", nil)
	if err != nil {
		t.Fatal(err)
	}
	u, err := llmProxy(req)
	if err != nil {
		t.Fatal(err)
	}
	if u == nil || u.Host != "127.0.0.1:7891" {
		t.Fatalf("want https proxy 127.0.0.1:7891, got %v", u)
	}

	req, err = http.NewRequest("GET", "http://example.com/models", nil)
	if err != nil {
		t.Fatal(err)
	}
	u, err = llmProxy(req)
	if err != nil {
		t.Fatal(err)
	}
	if u == nil || u.Host != "127.0.0.1:7890" {
		t.Fatalf("want http proxy 127.0.0.1:7890, got %v", u)
	}
}

// sseContentType is the media type SSE endpoints answer with.
const sseContentType = "text/event-stream"

// TestChatCompletionOpenAI_ReasoningContent verifies that delta.reasoning_content
// chunks from OpenAI-compatible reasoning models (DeepSeek-R1, QwQ, ...) are
// accumulated into a thinking block that survives in the final message, with
// the text block following it, so the frontend can persist and display it.
func TestChatCompletionOpenAI_ReasoningContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", sseContentType)
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":\"Let me think\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":\" about this.\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"The answer\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\" is 42.\"},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	reqBody := map[string]interface{}{
		"max_tokens": 1024,
		"messages":   []interface{}{map[string]interface{}{"role": "user", "content": "hi"}},
	}
	result, err := (&App{}).chatCompletionOpenAI("k", srv.URL, "deepseek-r1", reqBody, "test", "", srv.Client())
	if err != nil {
		t.Fatal(err)
	}

	var msg struct {
		Role    string `json:"role"`
		Content []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Thinking string `json:"thinking"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(result), &msg); err != nil {
		t.Fatalf("unmarshal %q: %v", result, err)
	}
	if len(msg.Content) != 2 {
		t.Fatalf("want 2 blocks, got %d: %s", len(msg.Content), result)
	}
	if msg.Content[0].Type != "thinking" || msg.Content[0].Thinking != "Let me think about this." {
		t.Fatalf("thinking block wrong: %+v", msg.Content[0])
	}
	if msg.Content[1].Type != "text" || msg.Content[1].Text != "The answer is 42." {
		t.Fatalf("text block wrong: %+v", msg.Content[1])
	}
}

func TestIsOpenCodeGoBaseURL(t *testing.T) {
	tests := []struct {
		baseURL string
		want    bool
	}{
		{"https://opencode.ai/zen/go/v1", true},
		{"https://opencode.ai/zen/go/v1/", true},
		{"https://opencode.ai/zen/go/v1/chat/completions", true},
		{" https://OPENCODE.AI/zen/go/v1/ ", true},
		{"https://opencode.ai/inference/go/openai/v1", true},
		{"https://opencode.ai/inference/go/openai/v1/", true},
		{"https://opencode.ai/inference/go/openai/v1/chat/completions", true},
		{"https://api.openai.com/v1", false},
		{"https://openrouter.ai/api/v1", false},
		{"http://localhost:11434/v1", false},
		{"https://example.com/zen/go/v1", false},
		{"https://opencode.ai.example.com/zen/go/v1", false},
		{"https://opencode.ai/zen/v1", false},
		{"https://opencode.ai/zen/go/v10", false},
		{"https://opencode.ai/zen/go/v1-extra", false},
		{"https://opencode.ai/inference/openai/v1", false},
		{"https://opencode.ai/inference/go/openai/v10", false},
		{"https://opencode.ai.example.com/inference/go/openai/v1", false},
		{"https://opencode.ai/%zz", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.baseURL, func(t *testing.T) {
			if got := isOpenCodeGoBaseURL(tt.baseURL); got != tt.want {
				t.Fatalf("isOpenCodeGoBaseURL(%q) = %v, want %v", tt.baseURL, got, tt.want)
			}
		})
	}
}

type llmRoundTripFunc func(*http.Request) (*http.Response, error)

func (f llmRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestChatCompletionOpenAI_SessionHeader(t *testing.T) {
	tests := []struct {
		name      string
		baseURL   string
		sessionID string
		want      string
	}{
		{"OpenCode Go", "https://opencode.ai/zen/go/v1", "session-123", "session-123"},
		{"OpenCode Go inference", "https://opencode.ai/inference/go/openai/v1", "session-123", "session-123"},
		{"inference empty session", "https://opencode.ai/inference/go/openai/v1", "", ""},
		{"empty session", "https://opencode.ai/zen/go/v1", "", ""},
		{"OpenCode Zen", "https://opencode.ai/zen/v1", "session-123", ""},
		{"OpenAI", "https://api.openai.com/v1", "session-123", ""},
		{"OpenRouter", "https://openrouter.ai/api/v1", "session-123", ""},
		{"local provider", "http://localhost:11434/v1", "session-123", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got http.Header
			client := &http.Client{Transport: llmRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				got = req.Header.Clone()
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{sseContentType}},
					Body:       io.NopCloser(strings.NewReader("data: [DONE]\n\n")),
				}, nil
			})}
			reqBody := map[string]interface{}{
				"max_tokens": 1024,
				"messages":   []interface{}{map[string]interface{}{"role": "user", "content": "hi"}},
			}
			if _, err := (&App{}).chatCompletionOpenAI("k", tt.baseURL, "model", reqBody, "test", tt.sessionID, client); err != nil {
				t.Fatal(err)
			}
			if got == nil {
				t.Fatal("no HTTP request sent")
			}
			if value := got.Get("x-opencode-session"); value != tt.want {
				t.Fatalf("x-opencode-session = %q, want %q", value, tt.want)
			}
			if tt.want == "" && got.Values("x-opencode-session") != nil {
				t.Fatal("x-opencode-session must be absent")
			}
		})
	}
}

// TestChatCompletionAnthropic_ThinkingBlocks verifies extended-thinking
// support: thinking_delta chunks accumulate into the thinking block, the
// signature_delta is preserved (required when replaying the block back to
// the API), and interleaved text blocks still arrive after it.
func TestChatCompletionAnthropic_ThinkingBlocks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", sseContentType)
		fmt.Fprint(w, "data: {\"type\":\"message_start\",\"message\":{\"role\":\"assistant\"}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"thinking\",\"thinking\":\"\"}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"Hmm, \"}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"let me see.\"}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"signature_delta\",\"signature\":\"sig123\"}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"text_delta\",\"text\":\"42.\"}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_stop\",\"index\":1}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"message_stop\"}\n\n")
	}))
	defer srv.Close()

	reqBody := map[string]interface{}{
		"max_tokens": 1024,
		"messages":   []interface{}{map[string]interface{}{"role": "user", "content": "hi"}},
	}
	result, err := (&App{}).chatCompletionAnthropic("k", srv.URL, "claude", reqBody, "test", srv.Client())
	if err != nil {
		t.Fatal(err)
	}

	var msg struct {
		Content []struct {
			Type      string `json:"type"`
			Text      string `json:"text"`
			Thinking  string `json:"thinking"`
			Signature string `json:"signature"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(result), &msg); err != nil {
		t.Fatalf("unmarshal %q: %v", result, err)
	}
	if len(msg.Content) != 2 {
		t.Fatalf("want 2 blocks, got %d: %s", len(msg.Content), result)
	}
	if msg.Content[0].Type != "thinking" || msg.Content[0].Thinking != "Hmm, let me see." {
		t.Fatalf("thinking block wrong: %+v", msg.Content[0])
	}
	if msg.Content[0].Signature != "sig123" {
		t.Fatalf("signature not preserved: %+v", msg.Content[0])
	}
	if msg.Content[1].Type != "text" || msg.Content[1].Text != "42." {
		t.Fatalf("text block wrong: %+v", msg.Content[1])
	}
}

// --- Attachment support -----------------------------------------------------

// tinyPNGBase64 is a valid 1x1 PNG, used as the image payload in converter and
// wire-format tests.
const tinyPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

// 1x1 GIF and WebP fixtures for the extended image whitelist.
const tinyGIFBase64 = "R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7"
const tinyWebPBase64 = "UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA=="

// imageBlock builds an Anthropic-format image content block.
func imageBlock(mediaType, data string) map[string]interface{} {
	return map[string]interface{}{
		"type": "image",
		"source": map[string]interface{}{
			"type":       "base64",
			"media_type": mediaType,
			"data":       data,
		},
	}
}

// TestConvertAnthropicMessageToOpenAI_ImageBlock covers the Chat Completions
// conversion: an image block must become OpenAI's nested image_url part, with
// the surrounding text preserved as a text part.
func TestConvertAnthropicMessageToOpenAI_ImageBlock(t *testing.T) {
	msg := map[string]interface{}{
		"role": "user",
		"content": []interface{}{
			map[string]interface{}{"type": "text", "text": "what is wrong here?"},
			imageBlock("image/png", tinyPNGBase64),
		},
	}

	got := convertAnthropicMessageToOpenAI(msg)
	if len(got) != 1 {
		t.Fatalf("want 1 message, got %d: %+v", len(got), got)
	}
	if got[0]["role"] != "user" {
		t.Fatalf("want role user, got %v", got[0]["role"])
	}
	parts, ok := got[0]["content"].([]map[string]interface{})
	if !ok {
		t.Fatalf("content is not a part array: %T (%+v)", got[0]["content"], got[0]["content"])
	}
	if len(parts) != 2 {
		t.Fatalf("want 2 parts, got %d: %+v", len(parts), parts)
	}
	if parts[0]["type"] != "text" || parts[0]["text"] != "what is wrong here?" {
		t.Fatalf("text part wrong: %+v", parts[0])
	}
	if parts[1]["type"] != "image_url" {
		t.Fatalf("want image_url part, got %+v", parts[1])
	}
	imgURL, ok := parts[1]["image_url"].(map[string]interface{})
	if !ok {
		t.Fatalf("image_url is not the nested object OpenAI expects: %+v", parts[1]["image_url"])
	}
	if want := "data:image/png;base64," + tinyPNGBase64; imgURL["url"] != want {
		t.Fatalf("image_url.url = %v, want %v", imgURL["url"], want)
	}
}

// TestConvertAnthropicMessageToOpenAI_ImageBlockOrder verifies the block order
// is preserved when text follows the image, and that a leading text block is
// re-seeded before the image part.
func TestConvertAnthropicMessageToOpenAI_ImageBlockOrder(t *testing.T) {
	cases := []struct {
		name   string
		blocks []interface{}
		want   []string
	}{
		{
			name: "text then image",
			blocks: []interface{}{
				map[string]interface{}{"type": "text", "text": "before"},
				imageBlock("image/jpeg", tinyPNGBase64),
			},
			want: []string{"text", "image_url"},
		},
		{
			name: "image then text",
			blocks: []interface{}{
				imageBlock("image/jpeg", tinyPNGBase64),
				map[string]interface{}{"type": "text", "text": "after"},
			},
			want: []string{"image_url", "text"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := convertAnthropicMessageToOpenAI(map[string]interface{}{
				"role":    "user",
				"content": tc.blocks,
			})
			parts, ok := got[0]["content"].([]map[string]interface{})
			if !ok {
				t.Fatalf("content is not a part array: %+v", got[0]["content"])
			}
			var types []string
			for _, p := range parts {
				types = append(types, p["type"].(string))
			}
			if strings.Join(types, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("part order = %v, want %v", types, tc.want)
			}
		})
	}
}

// TestConvertAnthropicMessageToOpenAI_TextOnlyUnchanged is a regression guard:
// messages without attachments must keep the plain-string content form.
func TestConvertAnthropicMessageToOpenAI_TextOnlyUnchanged(t *testing.T) {
	got := convertAnthropicMessageToOpenAI(map[string]interface{}{
		"role": "user",
		"content": []interface{}{
			map[string]interface{}{"type": "text", "text": "plain question"},
		},
	})
	if len(got) != 1 {
		t.Fatalf("want 1 message, got %d", len(got))
	}
	if got[0]["content"] != "plain question" {
		t.Fatalf("text-only content changed shape: %T %+v", got[0]["content"], got[0]["content"])
	}

	// tool_result ordering must survive too: tool message first, then the user text.
	got = convertAnthropicMessageToOpenAI(map[string]interface{}{
		"role": "user",
		"content": []interface{}{
			map[string]interface{}{"type": "tool_result", "tool_use_id": "t1", "content": "output"},
			map[string]interface{}{"type": "text", "text": "keep going"},
		},
	})
	if len(got) != 2 || got[0]["role"] != "tool" || got[1]["content"] != "keep going" {
		t.Fatalf("tool_result handling regressed: %+v", got)
	}
}

// TestConvertAnthropicMessageToResponses_ImageBlock covers the Responses API
// conversion: a flat input_image part whose image_url is the data URL itself.
func TestConvertAnthropicMessageToResponses_ImageBlock(t *testing.T) {
	got := convertAnthropicMessageToResponses(map[string]interface{}{
		"role": "user",
		"content": []interface{}{
			map[string]interface{}{"type": "text", "text": "read this screenshot"},
			imageBlock("image/png", tinyPNGBase64),
		},
	})

	if len(got) != 1 {
		t.Fatalf("want 1 item, got %d: %+v", len(got), got)
	}
	if got[0]["role"] != "user" {
		t.Fatalf("want role user, got %v", got[0]["role"])
	}
	parts, ok := got[0]["content"].([]map[string]interface{})
	if !ok {
		t.Fatalf("content is not a part array: %+v", got[0]["content"])
	}
	if len(parts) != 2 {
		t.Fatalf("want 2 parts, got %d: %+v", len(parts), parts)
	}
	if parts[0]["type"] != "input_text" || parts[0]["text"] != "read this screenshot" {
		t.Fatalf("input_text part wrong: %+v", parts[0])
	}
	if parts[1]["type"] != "input_image" {
		t.Fatalf("want input_image part, got %+v", parts[1])
	}
	// Responses API takes the URL as a plain string, not a nested object.
	if want := "data:image/png;base64," + tinyPNGBase64; parts[1]["image_url"] != want {
		t.Fatalf("image_url = %v (%T), want plain string %v", parts[1]["image_url"], parts[1]["image_url"], want)
	}
}

// TestConvertAnthropicMessageToResponses_TextOnlyUnchanged is a regression guard.
func TestConvertAnthropicMessageToResponses_TextOnlyUnchanged(t *testing.T) {
	got := convertAnthropicMessageToResponses(map[string]interface{}{
		"role":    "user",
		"content": "plain question",
	})
	if len(got) != 1 || got[0]["role"] != "user" {
		t.Fatalf("string content regressed: %+v", got)
	}
	parts := got[0]["content"].([]map[string]interface{})
	if len(parts) != 1 || parts[0]["type"] != "input_text" {
		t.Fatalf("string content shape regressed: %+v", parts)
	}
}

// TestValidateRequestAttachments covers the size / format / count boundaries.
func TestValidateRequestAttachments(t *testing.T) {
	// A valid PNG whose header declares a huge frame: small in bytes, legal
	// base64, decodable — but far over the pixel cap. Built by hand so the
	// test does not depend on an image library.
	oversizedPixelsPNG, err := pngWithDimensions(AttachmentMaxImageSide+1, 100)
	if err != nil {
		t.Fatal(err)
	}
	overLimit := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x41}, AttachmentMaxImageBytes+1))

	reqWith := func(blocks ...interface{}) map[string]interface{} {
		return map[string]interface{}{
			"messages": []interface{}{
				map[string]interface{}{"role": "user", "content": blocks},
			},
		}
	}

	cases := []struct {
		name    string
		req     map[string]interface{}
		wantErr error
	}{
		{
			name:    "oversized bytes are rejected",
			req:     reqWith(imageBlock("image/png", overLimit)),
			wantErr: errAttachmentTooLarge,
		},
		{
			// gif and webp joined the whitelist alongside png/jpeg; the
			// registered image/gif and x/image/webp decoders must parse them.
			name:    "gif media type is accepted",
			req:     reqWith(imageBlock("image/gif", tinyGIFBase64)),
			wantErr: nil,
		},
		{
			name:    "webp media type is accepted",
			req:     reqWith(imageBlock("image/webp", tinyWebPBase64)),
			wantErr: nil,
		},
		{
			name:    "an image over the pixel cap is rejected",
			req:     reqWith(imageBlock("image/png", oversizedPixelsPNG)),
			wantErr: errAttachmentTooLargePixels,
		},
		{
			name:    "zero byte image is rejected",
			req:     reqWith(imageBlock("image/png", "")),
			wantErr: errAttachmentEmpty,
		},
		{
			name:    "non-whitelisted media type is rejected",
			req:     reqWith(imageBlock("image/bmp", tinyPNGBase64)),
			wantErr: errAttachmentUnsupportedType,
		},
		{
			name:    "malformed base64 is rejected",
			req:     reqWith(imageBlock("image/png", "not base64!!!")),
			wantErr: errAttachmentInvalid,
		},
		{
			name:    "undecodable payload is rejected",
			req:     reqWith(imageBlock("image/png", base64.StdEncoding.EncodeToString([]byte("not an image")))),
			wantErr: errAttachmentInvalid,
		},
		{
			name:    "url source is rejected",
			req:     reqWith(map[string]interface{}{"type": "image", "source": map[string]interface{}{"type": "url", "url": "https://example.com/a.png"}}),
			wantErr: errAttachmentUnsupportedType,
		},
		{
			// The product rule is four images per turn, but the store merges
			// consecutive user messages (a turn whose assistant reply failed
			// leaves nothing between them), so a message may legitimately
			// carry more. Anything up to the hard ceiling must pass.
			name: "images up to the hard ceiling are accepted",
			req: reqWith(
				imageBlock("image/png", tinyPNGBase64),
				imageBlock("image/png", tinyPNGBase64),
				imageBlock("image/png", tinyPNGBase64),
				imageBlock("image/png", tinyPNGBase64),
			),
		},
		{
			name: "images at exactly the ceiling are accepted",
			req: reqWith(
				imageBlock("image/png", tinyPNGBase64),
				imageBlock("image/png", tinyPNGBase64),
				imageBlock("image/png", tinyPNGBase64),
				imageBlock("image/png", tinyPNGBase64),
				imageBlock("image/png", tinyPNGBase64),
				imageBlock("image/png", tinyPNGBase64),
				imageBlock("image/png", tinyPNGBase64),
				imageBlock("image/png", tinyPNGBase64),
			),
		},
		{
			name: "one image over the ceiling is rejected",
			req: reqWith(
				imageBlock("image/png", tinyPNGBase64),
				imageBlock("image/png", tinyPNGBase64),
				imageBlock("image/png", tinyPNGBase64),
				imageBlock("image/png", tinyPNGBase64),
				imageBlock("image/png", tinyPNGBase64),
				imageBlock("image/png", tinyPNGBase64),
				imageBlock("image/png", tinyPNGBase64),
				imageBlock("image/png", tinyPNGBase64),
				imageBlock("image/png", tinyPNGBase64),
			),
			wantErr: errAttachmentTooMany,
		},
		{
			name: "text-only message is unaffected",
			req:  reqWith(map[string]interface{}{"type": "text", "text": "no attachments"}),
		},
		{
			name: "request without messages is unaffected",
			req:  map[string]interface{}{"model": "m"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRequestAttachments(tc.req)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want %v, got nil", tc.wantErr)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("want %v, got %v", tc.wantErr, err)
			}
			// The payload must never be echoed back in an error (log hygiene).
			if strings.Contains(err.Error(), tinyPNGBase64) {
				t.Fatalf("error message leaks attachment bytes: %v", err)
			}
		})
	}
}

// pngWithDimensions builds the base64 of a minimal PNG header declaring the
// given width/height. Only the IHDR is written — image.DecodeConfig reads
// exactly that far, and the upstream validators we mirror do the same.
func pngWithDimensions(width, height int) (string, error) {
	// PNG signature + IHDR chunk. Fields: width, height (big-endian uint32),
	// bit depth 8, color type 0 (grayscale), the rest zero.
	chunk := make([]byte, 0, 33)
	chunk = append(chunk, 0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n')
	ihdr := make([]byte, 13)
	ihdr[0] = byte(width >> 24)
	ihdr[1] = byte(width >> 16)
	ihdr[2] = byte(width >> 8)
	ihdr[3] = byte(width)
	ihdr[4] = byte(height >> 24)
	ihdr[5] = byte(height >> 16)
	ihdr[6] = byte(height >> 8)
	ihdr[7] = byte(height)
	ihdr[8] = 8 // bit depth
	ihdr[9] = 2 // color type: truecolor RGB
	// length 13, type "IHDR", data, CRC (computed below).
	buf := bytes.NewBuffer(chunk)
	buf.Write([]byte{0, 0, 0, 13})
	buf.WriteString("IHDR")
	buf.Write(ihdr)
	crc := crc32.ChecksumIEEE(append(append([]byte("IHDR"), ihdr...)))
	buf.Write([]byte{byte(crc >> 24), byte(crc >> 16), byte(crc >> 8), byte(crc)})
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// TestUpstreamError_VisionRejection verifies an image request rejected by a
// vision-less model surfaces a readable sentinel rather than the raw body,
// while every other failure keeps the pre-existing message.
func TestUpstreamError_VisionRejection(t *testing.T) {
	visionBody := []byte(`{"error":{"message":"This model does not support image input.","type":"invalid_request_error"}}`)

	err := upstreamError(400, visionBody, true)
	if !errors.Is(err, errAttachmentUnsupportedByModel) {
		t.Fatalf("want unsupported-by-model sentinel, got %v", err)
	}
	if strings.Contains(err.Error(), "{\"error\"") {
		t.Fatalf("raw upstream JSON leaked into the error: %v", err)
	}
	if !strings.Contains(err.Error(), "does not support image input") {
		t.Fatalf("human-readable upstream detail was dropped: %v", err)
	}

	// Same body, but the request had no images: unchanged legacy behaviour.
	err = upstreamError(400, visionBody, false)
	if !strings.HasPrefix(err.Error(), "HTTP 400: ") {
		t.Fatalf("non-attachment error changed: %v", err)
	}

	// An attachment request failing for an unrelated reason keeps the detail.
	err = upstreamError(400, []byte(`{"error":{"message":"max_tokens is too large"}}`), true)
	if !strings.HasPrefix(err.Error(), "HTTP 400: ") {
		t.Fatalf("unrelated 400 was misclassified: %v", err)
	}
}

// TestUpstreamError_PixelLimitRejection verifies a "frame too large" refusal
// from a provider with a tighter pixel cap than the local pre-flight gets the
// pixel sentinel (so the UI can suggest downscaling), not the no-vision one.
func TestUpstreamError_PixelLimitRejection(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			name: "dimension wording",
			body: `{"error":{"message":"image dimensions exceed the maximum allowed 4096x4096","type":"invalid_request_error"}}`,
		},
		{
			name: "resolution wording",
			body: `{"error":{"message":"The image resolution is too high","type":"invalid_request_error"}}`,
		},
		{
			name: "exceed wording",
			body: `{"error":{"message":"Image size exceeds maximum"}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := upstreamError(400, []byte(tc.body), true)
			if !errors.Is(err, errAttachmentTooLargePixels) {
				t.Fatalf("want pixel-limit sentinel, got %v", err)
			}
			if strings.HasPrefix(err.Error(), "HTTP 400") {
				t.Fatalf("raw upstream body leaked: %v", err)
			}
		})
	}

	// "too large" about something else entirely must not be classified as an
	// image problem just because the request carried an image.
	err := upstreamError(400, []byte(`{"error":{"message":"max_tokens is too large"}}`), true)
	if !strings.HasPrefix(err.Error(), "HTTP 400: ") {
		t.Fatalf("unrelated size error was misclassified: %v", err)
	}

	// No image in the request: legacy behaviour regardless of wording.
	err = upstreamError(400, []byte(cases[0].body), false)
	if !strings.HasPrefix(err.Error(), "HTTP 400: ") {
		t.Fatalf("text-only request was misclassified: %v", err)
	}
}

// TestChatCompletionWireFormats exercises all three protocol paths end to end
// against a stub upstream and inspects the JSON that actually goes on the wire.
func TestChatCompletionWireFormats(t *testing.T) {
	pngDataURL := "data:image/png;base64," + tinyPNGBase64
	textFileBody := "[Attached file: notes.log]\n----- begin notes.log -----\nboom\n----- end notes.log -----"

	reqBody := func() map[string]interface{} {
		return map[string]interface{}{
			"max_tokens": 1024,
			"messages": []interface{}{
				map[string]interface{}{
					"role": "user",
					"content": []interface{}{
						map[string]interface{}{"type": "text", "text": "why did it crash?"},
						imageBlock("image/png", tinyPNGBase64),
						map[string]interface{}{"type": "text", "text": textFileBody},
					},
				},
			},
		}
	}

	cases := []struct {
		name     string
		path     string
		protocol string
		call     func(a *App, base string, body map[string]interface{}, c *http.Client) (string, error)
		// assert inspects the decoded upstream request body.
		assert func(t *testing.T, sent map[string]interface{})
	}{
		{
			name:     "anthropic messages",
			path:     "/v1/messages",
			protocol: "anthropic",
			call: func(a *App, base string, body map[string]interface{}, c *http.Client) (string, error) {
				return a.chatCompletionAnthropic("k", base, "claude", body, "test", c)
			},
			assert: func(t *testing.T, sent map[string]interface{}) {
				block := firstUserBlock(t, sent, "messages")
				if block["type"] != "image" {
					t.Fatalf("anthropic block type = %v, want image", block["type"])
				}
				src, _ := block["source"].(map[string]interface{})
				if src["type"] != "base64" || src["media_type"] != "image/png" || src["data"] != tinyPNGBase64 {
					t.Fatalf("anthropic image source wrong: %+v", src)
				}
			},
		},
		{
			name:     "openai chat completions",
			path:     "/chat/completions",
			protocol: "openai",
			call: func(a *App, base string, body map[string]interface{}, c *http.Client) (string, error) {
				return a.chatCompletionOpenAI("k", base, "gpt", body, "test", "", c)
			},
			assert: func(t *testing.T, sent map[string]interface{}) {
				block := firstUserBlock(t, sent, "messages")
				if block["type"] != "image_url" {
					t.Fatalf("openai block type = %v, want image_url", block["type"])
				}
				inner, _ := block["image_url"].(map[string]interface{})
				if inner["url"] != pngDataURL {
					t.Fatalf("openai image_url.url = %v, want %v", inner["url"], pngDataURL)
				}
			},
		},
		{
			name:     "openai responses",
			path:     "/responses",
			protocol: "responses",
			call: func(a *App, base string, body map[string]interface{}, c *http.Client) (string, error) {
				return a.chatCompletionResponses("k", base, "gpt", body, "test", c)
			},
			assert: func(t *testing.T, sent map[string]interface{}) {
				block := firstUserBlock(t, sent, "input")
				if block["type"] != "input_image" {
					t.Fatalf("responses block type = %v, want input_image", block["type"])
				}
				if block["image_url"] != pngDataURL {
					t.Fatalf("responses image_url = %v, want flat data URL %v", block["image_url"], pngDataURL)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var captured []byte
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					t.Errorf("upstream path = %s, want %s", r.URL.Path, tc.path)
				}
				captured, _ = io.ReadAll(r.Body)
				w.Header().Set("Content-Type", sseContentType)
				fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer srv.Close()

			_, _ = tc.call(&App{}, srv.URL, reqBody(), srv.Client())

			var sent map[string]interface{}
			if err := json.Unmarshal(captured, &sent); err != nil {
				t.Fatalf("upstream body is not JSON: %v (%s)", err, captured)
			}
			tc.assert(t, sent)

			// Text attachments must ride along as plain text, never base64.
			if !strings.Contains(string(captured), "why did it crash?") {
				t.Fatalf("user text missing from upstream body")
			}
			if !strings.Contains(string(captured), "boom") {
				t.Fatalf("text attachment body missing from upstream payload")
			}
			if strings.Contains(string(captured), base64.StdEncoding.EncodeToString([]byte(textFileBody))) {
				t.Fatalf("text attachment was base64-encoded")
			}
		})
	}
}

// firstUserBlock returns the first non-text content part of the first user
// message in a protocol payload (Anthropic/OpenAI use `messages`, Responses
// uses `input`).
func firstUserBlock(t *testing.T, sent map[string]interface{}, field string) map[string]interface{} {
	t.Helper()
	items, ok := sent[field].([]interface{})
	if !ok || len(items) == 0 {
		t.Fatalf("payload has no %s array: %+v", field, sent)
	}
	for _, item := range items {
		msg, ok := item.(map[string]interface{})
		if !ok || msg["role"] != "user" {
			continue
		}
		parts, ok := msg["content"].([]interface{})
		if !ok {
			continue
		}
		for _, p := range parts {
			block, ok := p.(map[string]interface{})
			if !ok {
				continue
			}
			if block["type"] != "text" && block["type"] != "input_text" {
				return block
			}
		}
	}
	t.Fatalf("no image block found in %s: %+v", field, items)
	return nil
}

// TestChatCompletionRejectsOversizedAttachmentBeforeSending verifies the guard
// rail fires before any network call: the stub server must never be hit.
func TestChatCompletionRejectsOversizedAttachmentBeforeSending(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
	}))
	defer srv.Close()

	oversized := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x41}, AttachmentMaxImageBytes+1))
	_, err := (&App{}).ChatCompletion("k", srv.URL, "m", mustJSON(t, map[string]interface{}{
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": []interface{}{imageBlock("image/png", oversized)}},
		},
	}), "openai", "test", "", "")
	if !errors.Is(err, errAttachmentTooLarge) {
		t.Fatalf("want %v, got %v", errAttachmentTooLarge, err)
	}
	if hit {
		t.Fatal("oversized attachment reached the upstream API")
	}
}

func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestMCPClientUpdateRenamesWithoutCreatingDuplicate(t *testing.T) {
	dir := t.TempDir()
	a := &App{dataDir: dir, mcpClientStore: store.NewMCPClientStore(dir)}
	defer func() {
		if a.mcpClient != nil {
			a.mcpClient.CloseAll()
		}
	}()
	cfg := store.MCPClientConfig{Name: "before", Transport: store.MCPTransportStdio, Command: "npx", Args: []string{"-y", "example"}, Enabled: false}
	if err := a.SaveMCPClient(cfg); err != nil {
		t.Fatal(err)
	}
	listed, err := a.ListMCPClients()
	if err != nil || len(listed) != 1 {
		t.Fatalf("list: %v %v", listed, err)
	}
	cfg = listed[0]
	stale := cfg
	cfg.Name = "after"
	cfg.Args = []string{"-y", "updated", "path with spaces"}
	if err := a.UpdateMCPClient("before", cfg); err != nil {
		t.Fatal(err)
	}
	all, err := a.mcpClientStore.Load()
	if err != nil || len(all) != 1 || all[0].Name != "after" || all[0].Enabled || all[0].Args[2] != "path with spaces" {
		t.Fatalf("edit did not preserve config: %+v %v", all, err)
	}
	other := cfg
	other.Name = "other"
	other.Revision = ""
	if err := a.SaveMCPClient(other); err != nil {
		t.Fatal(err)
	}
	if err := a.UpdateMCPClient("other", cfg); err == nil {
		t.Fatal("rename overwrote another client")
	}
	if err := a.UpdateMCPClient("missing", cfg); err == nil {
		t.Fatal("stale edit created a client")
	}
	if err := a.SaveMCPClient(stale); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("stale edit resurrected renamed client: %v", err)
	}
	all, err = a.mcpClientStore.Load()
	if err != nil || len(all) != 2 {
		t.Fatalf("failed rename changed store: %+v %v", all, err)
	}
}
