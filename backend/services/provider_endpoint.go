package services

// provider_endpoint.go ── 统一 provider base_url → 完整上游 endpoint 的拼接。
//
// 背景:不同 provider 的 base_url 习惯不一致 ——
//   - 带版本段: OpenAI(/v1)、Moonshot(/v1)、qwen(/compatible-mode/v1)、
//     zhipu(/api/paas/v4)、doubao(/api/v3)、minimax(/v1)、xai(/v1)
//   - 不带版本段: DeepSeek、Anthropic、Google(generativelanguage.googleapis.com)
//
// 历史 bug:旧代码无脑 `base + "/v1/chat/completions"`,对已带 /v1 的 base 拼出
// `.../v1/v1/chat/completions` → 404;对 /api/paas/v4 这种又强加 /v1 也错。
// 本文件按"base path 末段是否已是 vN(v1/v2/v3/v4...)"决定补不补默认版本段。

import (
	"net/url"
	"regexp"
	"strings"
)

var versionSegmentRE = regexp.MustCompile(`^v\d+$`)

// baseHasVersionSegment 判断 base_url 的 path 末段是否已是版本段(v1/v3/v4...)。
// 解析失败时退回字符串末段判断,保证不 panic。
func baseHasVersionSegment(baseURL string) bool {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return false
	}
	path := trimmed
	if u, err := url.Parse(trimmed); err == nil && u.Path != "" {
		path = u.Path
	}
	path = strings.TrimRight(path, "/")
	if path == "" {
		return false
	}
	segs := strings.Split(path, "/")
	last := segs[len(segs)-1]
	return versionSegmentRE.MatchString(strings.ToLower(last))
}

// joinProviderPath 把 base_url 与相对路径拼成完整 endpoint。
// base 末段已是版本段 → 直接拼 relPath;否则在 relPath 前补默认版本 /v1。
//
//	relPath 形如 "/chat/completions" / "/models" / "/messages"。
func joinProviderPath(baseURL, relPath string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	relPath = "/" + strings.TrimLeft(relPath, "/")
	if baseHasVersionSegment(base) {
		return base + relPath
	}
	return base + "/v1" + relPath
}

// providerChatEndpoint OpenAI 兼容 chat completions endpoint。
func providerChatEndpoint(baseURL string) string {
	return joinProviderPath(baseURL, "/chat/completions")
}

// providerModelsEndpoint OpenAI 兼容 / Anthropic model 列表 endpoint。
// (两家恰好都是 .../v1/models)
func providerModelsEndpoint(baseURL string) string {
	return joinProviderPath(baseURL, "/models")
}

// anthropicMessagesEndpoint Anthropic messages endpoint。
func anthropicMessagesEndpoint(baseURL string) string {
	return joinProviderPath(baseURL, "/messages")
}
