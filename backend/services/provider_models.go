package services

// provider_models.go ── 拉取 ProviderAccount 的可用 model 列表。
//
// 三家协议各不一样:
//   - OpenAI 兼容(deepseek/moonshot/qwen/openai/xai/zhipu/minimax/doubao):
//       GET {base_url}/v1/models  Authorization: Bearer <token>
//       响应: { "data": [ {"id": "..."} , ... ] }
//   - Anthropic:
//       GET {base_url}/v1/models  x-api-key: <token>  anthropic-version: 2023-06-01
//       响应: { "data": [ {"id": "..."} , ... ] }   (与 OpenAI 巧合同形)
//   - Google Gemini:
//       GET {base_url}/v1beta/models?key=<token>
//       响应: { "models": [ {"name": "models/gemini-..."} , ... ] }
//       — name 字段含 "models/" 前缀，需要 strip。
//
// 失败原因(网络 / 鉴权 / JSON 形不对) 都返回 error，让上层 UI 写到
// LastDiscoveryError 字段供用户排障，而不是吞掉静默失败。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// FetchProviderModels 拉取一个 ProviderAccount 的可用 model 列表。
//
// httpClient 可以传 nil 走默认 30s 超时。生产环境建议复用 MitmProxy.upstreamBase
// 这样能继承用户配置的 Clash/系统代理。
func FetchProviderModels(ctx context.Context, httpClient *http.Client, provider, baseURL, authToken string) ([]string, error) {
	provider = strings.TrimSpace(strings.ToLower(provider))
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	authToken = strings.TrimSpace(authToken)
	if provider == "" || baseURL == "" || authToken == "" {
		return nil, fmt.Errorf("provider / base_url / auth_token 任一为空")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	switch provider {
	case "anthropic":
		return fetchAnthropicModels(ctx, httpClient, baseURL, authToken)
	case "google":
		return fetchGeminiModels(ctx, httpClient, baseURL, authToken)
	default:
		// openai / deepseek / moonshot / qwen / xai / zhipu / minimax / doubao
		// 全部走 OpenAI 兼容协议
		return fetchOpenAICompatModels(ctx, httpClient, baseURL, authToken)
	}
}

func fetchOpenAICompatModels(ctx context.Context, c *http.Client, baseURL, token string) ([]string, error) {
	endpoint := providerModelsEndpoint(baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body := readLimitedBody(resp, 512)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	out := make([]string, 0, len(payload.Data))
	for _, m := range payload.Data {
		id := strings.TrimSpace(m.ID)
		if id != "" {
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("响应里 data[] 为空")
	}
	return out, nil
}

func fetchAnthropicModels(ctx context.Context, c *http.Client, baseURL, token string) ([]string, error) {
	endpoint := providerModelsEndpoint(baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("x-api-key", token)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Accept", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body := readLimitedBody(resp, 512)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	out := make([]string, 0, len(payload.Data))
	for _, m := range payload.Data {
		id := strings.TrimSpace(m.ID)
		if id != "" {
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("响应里 data[] 为空")
	}
	return out, nil
}

func fetchGeminiModels(ctx context.Context, c *http.Client, baseURL, token string) ([]string, error) {
	// Gemini 默认 base_url 用户应填 https://generativelanguage.googleapis.com
	// 列表接口走 /v1beta/models?key=<token>
	endpoint := baseURL + "/v1beta/models?key=" + url.QueryEscape(token)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body := readLimitedBody(resp, 512)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}
	var payload struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	out := make([]string, 0, len(payload.Models))
	for _, m := range payload.Models {
		// Gemini 返回 "models/gemini-1.5-flash" 这种带前缀的名字，剥掉 models/
		// 让用户在 UI 上看到 "gemini-1.5-flash" 与 OpenAI/Anthropic 风格一致。
		name := strings.TrimSpace(strings.TrimPrefix(m.Name, "models/"))
		if name != "" {
			out = append(out, name)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("响应里 models[] 为空")
	}
	return out, nil
}

func readLimitedBody(resp *http.Response, max int) string {
	if resp.Body == nil {
		return ""
	}
	// io.Reader.Read 不保证一次填满 buffer,用 LimitReader+ReadAll 读全(上限 max),
	// 避免错误响应体只读到几十字节导致排障信息被截断。
	data, _ := io.ReadAll(io.LimitReader(resp.Body, int64(max)))
	return strings.TrimSpace(string(data))
}
