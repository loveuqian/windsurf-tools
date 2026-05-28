package services

import "testing"

func TestProviderChatEndpoint(t *testing.T) {
	cases := []struct {
		name string
		base string
		want string
	}{
		// 已带版本段 → 不再补 /v1
		{"openai v1", "https://api.openai.com/v1", "https://api.openai.com/v1/chat/completions"},
		{"openai v1 trailing slash", "https://api.openai.com/v1/", "https://api.openai.com/v1/chat/completions"},
		{"moonshot v1", "https://api.moonshot.cn/v1", "https://api.moonshot.cn/v1/chat/completions"},
		{"qwen compatible-mode v1", "https://dashscope.aliyuncs.com/compatible-mode/v1", "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions"},
		{"zhipu v4", "https://open.bigmodel.cn/api/paas/v4", "https://open.bigmodel.cn/api/paas/v4/chat/completions"},
		{"doubao v3", "https://ark.cn-beijing.volces.com/api/v3", "https://ark.cn-beijing.volces.com/api/v3/chat/completions"},
		{"xai v1", "https://api.x.ai/v1", "https://api.x.ai/v1/chat/completions"},
		// 不带版本段 → 补 /v1
		{"deepseek no version", "https://api.deepseek.com", "https://api.deepseek.com/v1/chat/completions"},
		{"deepseek trailing slash", "https://api.deepseek.com/", "https://api.deepseek.com/v1/chat/completions"},
		{"oneapi host only", "https://my-relay.example.com", "https://my-relay.example.com/v1/chat/completions"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := providerChatEndpoint(c.base); got != c.want {
				t.Errorf("providerChatEndpoint(%q) = %q, want %q", c.base, got, c.want)
			}
		})
	}
}

func TestProviderModelsEndpoint(t *testing.T) {
	cases := []struct {
		base string
		want string
	}{
		{"https://api.openai.com/v1", "https://api.openai.com/v1/models"},
		{"https://api.deepseek.com", "https://api.deepseek.com/v1/models"},
		{"https://open.bigmodel.cn/api/paas/v4", "https://open.bigmodel.cn/api/paas/v4/models"},
	}
	for _, c := range cases {
		if got := providerModelsEndpoint(c.base); got != c.want {
			t.Errorf("providerModelsEndpoint(%q) = %q, want %q", c.base, got, c.want)
		}
	}
}

func TestAnthropicMessagesEndpoint(t *testing.T) {
	// Anthropic 官方 base 不带版本段 → 补 /v1
	if got := anthropicMessagesEndpoint("https://api.anthropic.com"); got != "https://api.anthropic.com/v1/messages" {
		t.Errorf("got %q", got)
	}
	// 若用户填了带 /v1 的中转 base,不应双 /v1
	if got := anthropicMessagesEndpoint("https://relay.example.com/v1"); got != "https://relay.example.com/v1/messages" {
		t.Errorf("got %q", got)
	}
}

func TestBaseHasVersionSegment(t *testing.T) {
	yes := []string{
		"https://api.openai.com/v1",
		"https://x/api/v3",
		"https://x/api/paas/v4",
		"https://x/v10",
	}
	no := []string{
		"https://api.deepseek.com",
		"https://api.anthropic.com",
		"https://x/compatible-mode", // 末段非 vN
		"https://x/v1beta",          // 不是纯 vN
	}
	for _, b := range yes {
		if !baseHasVersionSegment(b) {
			t.Errorf("baseHasVersionSegment(%q) = false, want true", b)
		}
	}
	for _, b := range no {
		if baseHasVersionSegment(b) {
			t.Errorf("baseHasVersionSegment(%q) = true, want false", b)
		}
	}
}
