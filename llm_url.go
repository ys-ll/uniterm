package main

import (
	"fmt"
	"strings"
)

// buildURL returns the full upstream URL for a chat completion request given
// the user's base URL and the configured protocol. The base URL is trimmed
// of any trailing slash; the protocol decides what suffix to append.
//
// Recognized protocols:
//
//   - "anthropic":          <base>/v1/messages  (or /messages when /v1 already on base)
//   - "openai":             <base>/v1/chat/completions  (or /chat/completions when /v1 already on base)
//   - "openai-compatible":  same as openai — for domestic Chinese LLMs and 3rd-party OpenAI-compatible proxies
//                            (DeepSeek, Qwen DashScope, Kimi, OpenRouter, OneAPI, …)
//   - "responses":          <base>/v1/responses  (or /responses when /v1 already on base)
//   - "glm-native":         <base> returned verbatim — Zhipu GLM uses a non-standard path the user must supply whole
//   - "custom":             <base> returned verbatim — any URL the user provides is used as-is
//
// Unknown protocols return an error.
func buildURL(baseURL, protocol string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return "", fmt.Errorf("base URL is empty")
	}
	switch protocol {
	case "anthropic":
		if strings.HasSuffix(base, "/v1") {
			return base + "/messages", nil
		}
		return base + "/v1/messages", nil
	case "openai", "openai-compatible":
		if strings.HasSuffix(base, "/v1") {
			return base + "/chat/completions", nil
		}
		return base + "/v1/chat/completions", nil
	case "responses":
		if strings.HasSuffix(base, "/v1") {
			return base + "/responses", nil
		}
		return base + "/v1/responses", nil
	case "glm-native", "custom":
		return base, nil
	}
	return "", fmt.Errorf("unknown protocol: %q", protocol)
}