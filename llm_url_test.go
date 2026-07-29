package main

import "testing"

// TestBuildURL is a table-driven test covering every recognized protocol
// across the base-URL conventions users actually paste from Claude Code,
// OpenAI, OneAPI, DeepSeek, Qwen DashScope, Kimi, OpenRouter, and Zhipu GLM.
func TestBuildURL(t *testing.T) {
	cases := []struct {
		name     string
		baseURL  string
		protocol string
		want     string
		wantErr  bool
	}{
		// --- Anthropic ---
		{"anthropic bare domain", "https://api.anthropic.com", "anthropic", "https://api.anthropic.com/v1/messages", false},
		{"anthropic with /v1", "https://api.anthropic.com/v1", "anthropic", "https://api.anthropic.com/v1/messages", false},
		{"anthropic trailing slash", "https://api.anthropic.com/", "anthropic", "https://api.anthropic.com/v1/messages", false},
		{"anthropic /v1/ trailing slash", "https://api.anthropic.com/v1/", "anthropic", "https://api.anthropic.com/v1/messages", false},
		{"anthropic with custom path /api", "https://proxy.example.com/api", "anthropic", "https://proxy.example.com/api/v1/messages", false},
		{"anthropic with /api/v1", "https://proxy.example.com/api/v1", "anthropic", "https://proxy.example.com/api/v1/messages", false},

		// --- OpenAI ---
		{"openai with /v1", "https://api.openai.com/v1", "openai", "https://api.openai.com/v1/chat/completions", false},
		{"openai bare domain", "https://api.openai.com", "openai", "https://api.openai.com/v1/chat/completions", false},
		{"openai trailing slash", "https://api.openai.com/v1/", "openai", "https://api.openai.com/v1/chat/completions", false},

		// --- OpenAI-compatible (国产大模型 + 第三方代理) ---
		{"deepseek /v1", "https://api.deepseek.com/v1", "openai-compatible", "https://api.deepseek.com/v1/chat/completions", false},
		{"qwen dashscope compatible-mode", "https://dashscope.aliyuncs.com/compatible-mode/v1", "openai-compatible", "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions", false},
		{"kimi /v1", "https://api.moonshot.cn/v1", "openai-compatible", "https://api.moonshot.cn/v1/chat/completions", false},
		{"openrouter /api/v1", "https://openrouter.ai/api/v1", "openai-compatible", "https://openrouter.ai/api/v1/chat/completions", false},
		{"oneapi bare", "https://oneapi.example.com", "openai-compatible", "https://oneapi.example.com/v1/chat/completions", false},
		{"oneapi /v1", "https://oneapi.example.com/v1", "openai-compatible", "https://oneapi.example.com/v1/chat/completions", false},
		{"ollama bare (treated as openai-compatible)", "http://localhost:11434", "openai-compatible", "http://localhost:11434/v1/chat/completions", false},
		{"ollama with /v1", "http://localhost:11434/v1", "openai-compatible", "http://localhost:11434/v1/chat/completions", false},

		// --- OpenAI Responses API ---
		{"responses with /v1", "https://api.openai.com/v1", "responses", "https://api.openai.com/v1/responses", false},
		{"responses bare domain", "https://api.openai.com", "responses", "https://api.openai.com/v1/responses", false},

		// --- GLM native (verbatim passthrough) ---
		{"glm full endpoint", "https://open.bigmodel.cn/api/paas/v4/chat/completions", "glm-native", "https://open.bigmodel.cn/api/paas/v4/chat/completions", false},

		// --- Custom (verbatim passthrough) ---
		{"custom full URL", "https://my-proxy.example.com/api/v1/chat/completions", "custom", "https://my-proxy.example.com/api/v1/chat/completions", false},
		{"custom local proxy", "http://localhost:8080/v1/messages", "custom", "http://localhost:8080/v1/messages", false},

		// --- Error cases ---
		{"empty base", "", "anthropic", "", true},
		{"whitespace base", "   ", "anthropic", "", true},
		{"unknown protocol", "https://api.example.com", "banana", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildURL(tc.baseURL, tc.protocol)
			if (err != nil) != tc.wantErr {
				t.Fatalf("buildURL(%q, %q): err = %v, wantErr = %v", tc.baseURL, tc.protocol, err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("buildURL(%q, %q) = %q, want %q", tc.baseURL, tc.protocol, got, tc.want)
			}
		})
	}
}