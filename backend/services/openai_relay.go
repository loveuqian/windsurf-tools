package services

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http2"
)

// OpenAIRelay 本地 OpenAI/Anthropic 兼容 API 中转服务器
type OpenAIRelay struct {
	mu           sync.RWMutex
	server       *http.Server
	listener     net.Listener
	running      bool
	port         int
	secret       string     // Bearer token 鉴权
	proxy        *MitmProxy // 复用账号池
	logFn        func(string)
	onSuccess    func(apiKey string) // 请求成功后回调（用于触发额度刷新）
	proxyURL     string              // 出站代理
	upstream     http.RoundTripper   // 持久连接池
	maxRetry     int                 // 额度耗尽重试次数
	usageTracker *UsageTracker       // 用量追踪
}

// SetOnSuccess 设置请求成功回调（App 层用来触发额度刷新）
func (r *OpenAIRelay) SetOnSuccess(fn func(apiKey string)) {
	r.mu.Lock()
	r.onSuccess = fn
	r.mu.Unlock()
}

type OpenAIRelayStatus struct {
	Running bool   `json:"running"`
	Port    int    `json:"port"`
	URL     string `json:"url"`
}

func NewOpenAIRelay(proxy *MitmProxy, logFn func(string), proxyURL string, tracker *UsageTracker) *OpenAIRelay {
	return &OpenAIRelay{
		proxy:        proxy,
		logFn:        logFn,
		proxyURL:     proxyURL,
		maxRetry:     defaultReplayBudget,
		usageTracker: tracker,
	}
}

func (r *OpenAIRelay) log(format string, args ...interface{}) {
	if r.logFn != nil {
		r.logFn(fmt.Sprintf("[OpenAI Relay] "+format, args...))
	}
}

func (r *OpenAIRelay) Start(port int, secret string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return fmt.Errorf("relay already running")
	}

	if port <= 0 {
		port = 8787
	}
	r.port = port
	r.secret = secret

	// 构建持久 h2 transport（连接池复用）
	r.upstream = r.buildUpstreamTransport()

	mux := http.NewServeMux()
	// OpenAI 兼容
	mux.HandleFunc("/v1/chat/completions", r.handleChatCompletions)
	mux.HandleFunc("/v1/models", r.handleModels)
	// Anthropic Messages API 兼容（Claude Code）
	mux.HandleFunc("/v1/messages", r.handleAnthropicMessages)
	// 用量追踪 API
	mux.HandleFunc("/v1/usage", r.handleUsageAPI)
	mux.HandleFunc("/v1/usage/summary", r.handleUsageSummaryAPI)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"ok"}`))
	})

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("listen :%d: %w", port, err)
	}

	r.listener = ln
	r.server = &http.Server{Handler: r.withCORS(mux)}
	r.running = true

	go func() {
		r.log("started on http://127.0.0.1:%d", port)
		if err := r.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			r.log("server error: %v", err)
		}
		r.mu.Lock()
		r.running = false
		r.mu.Unlock()
	}()
	return nil
}

func (r *OpenAIRelay) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if origin := strings.TrimSpace(req.Header.Get("Origin")); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if req.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, req)
	})
}

func (r *OpenAIRelay) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running || r.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := r.server.Shutdown(ctx)
	r.running = false
	r.log("stopped")
	return err
}

func (r *OpenAIRelay) Status() OpenAIRelayStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s := OpenAIRelayStatus{Running: r.running, Port: r.port}
	if r.running {
		s.URL = fmt.Sprintf("http://127.0.0.1:%d", r.port)
	}
	return s
}

// RuntimeConfig 返回当前运行配置（线程安全），供 app 层判断是否需重启 server。
// 第一个返回值表示 server 是否在跑；后续 port/secret 仅 running=true 时有效。
func (r *OpenAIRelay) RuntimeConfig() (running bool, port int, secret string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.running, r.port, r.secret
}

// ── 鉴权 ──

func (r *OpenAIRelay) checkAuth(w http.ResponseWriter, req *http.Request) bool {
	if r.secret == "" {
		return true
	}
	auth := req.Header.Get("Authorization")
	if strings.TrimPrefix(auth, "Bearer ") == r.secret {
		return true
	}
	writeOpenAIError(w, 401, "invalid_api_key", "Invalid API key")
	return false
}

// ── /v1/models ──

func (r *OpenAIRelay) handleModels(w http.ResponseWriter, req *http.Request) {
	if !r.checkAuth(w, req) {
		return
	}
	var data []map[string]interface{}
	for _, m := range relayModelCatalog {
		data = append(data, m.toJSON())
	}
	resp := map[string]interface{}{"object": "list", "data": data}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// relayModelInfo 每个模型的元信息 + 定价
type relayModelInfo struct {
	ID             string  // 客户端用的名
	Name           string  // 显示名
	Provider       string  // openai / anthropic / google / deepseek / ...
	Family         string  // gpt / claude / gemini / ...
	ContextLength  int     // 上下文窗口
	MaxOutput      int     // 最大输出 token
	InputPrice     float64 // 输入定价 $/1M tokens
	OutputPrice    float64 // 输出定价 $/1M tokens
	SupportsVision bool
	SupportsTools  bool
	HiddenFree     bool // windsurf 隐藏免费模型
}

func (m relayModelInfo) toJSON() map[string]interface{} {
	obj := map[string]interface{}{
		"id":                 m.ID,
		"object":             "model",
		"owned_by":           m.Provider,
		"name":               m.Name,
		"provider":           m.Provider,
		"family":             m.Family,
		"context_length":     m.ContextLength,
		"max_output_tokens":  m.MaxOutput,
		"supports_vision":    m.SupportsVision,
		"supports_tools":     m.SupportsTools,
		"supports_streaming": true,
	}
	if m.InputPrice > 0 || m.OutputPrice > 0 {
		obj["pricing"] = map[string]interface{}{
			"input":    m.InputPrice,
			"output":   m.OutputPrice,
			"currency": "usd",
			"unit":     "1M tokens",
		}
	}
	if m.HiddenFree {
		obj["hidden_free"] = true
	}
	return obj
}

// 2026-05-18 全量模型目录(对齐 wsapi catalog 104 模型 + 定价)
// 定价参考 Windsurf 官方 + 上游 API 公开价
var relayModelCatalog = []relayModelInfo{
	// ── Claude ──────────────────────────────────────────
	{ID: "claude-opus-4-7-medium", Name: "Claude Opus 4.7 Medium", Provider: "anthropic", Family: "claude", ContextLength: 200000, MaxOutput: 32000, InputPrice: 15, OutputPrice: 75, SupportsVision: true, SupportsTools: true},
	{ID: "claude-opus-4.6", Name: "Claude Opus 4.6", Provider: "anthropic", Family: "claude", ContextLength: 200000, MaxOutput: 32000, InputPrice: 15, OutputPrice: 75, SupportsVision: true, SupportsTools: true},
	{ID: "claude-opus-4.6-thinking", Name: "Claude Opus 4.6 Thinking", Provider: "anthropic", Family: "claude", ContextLength: 200000, MaxOutput: 32000, InputPrice: 15, OutputPrice: 75, SupportsVision: true, SupportsTools: true},
	{ID: "claude-4.5-opus", Name: "Claude 4.5 Opus", Provider: "anthropic", Family: "claude", ContextLength: 200000, MaxOutput: 32000, InputPrice: 15, OutputPrice: 75, SupportsVision: true, SupportsTools: true},
	{ID: "claude-4.5-opus-thinking", Name: "Claude 4.5 Opus Thinking", Provider: "anthropic", Family: "claude", ContextLength: 200000, MaxOutput: 32000, InputPrice: 15, OutputPrice: 75, SupportsVision: true, SupportsTools: true},
	{ID: "claude-4.5-sonnet", Name: "Claude 4.5 Sonnet", Provider: "anthropic", Family: "claude", ContextLength: 200000, MaxOutput: 16000, InputPrice: 3, OutputPrice: 15, SupportsVision: true, SupportsTools: true},
	{ID: "claude-4.5-sonnet-thinking", Name: "Claude 4.5 Sonnet Thinking", Provider: "anthropic", Family: "claude", ContextLength: 200000, MaxOutput: 16000, InputPrice: 3, OutputPrice: 15, SupportsVision: true, SupportsTools: true},
	{ID: "claude-4.5-haiku", Name: "Claude 4.5 Haiku", Provider: "anthropic", Family: "claude", ContextLength: 200000, MaxOutput: 8192, InputPrice: 0.8, OutputPrice: 4, SupportsVision: true, SupportsTools: true},
	{ID: "claude-sonnet-4.6", Name: "Claude Sonnet 4.6", Provider: "anthropic", Family: "claude", ContextLength: 200000, MaxOutput: 16000, InputPrice: 3, OutputPrice: 15, SupportsVision: true, SupportsTools: true},
	{ID: "claude-sonnet-4.6-1m", Name: "Claude Sonnet 4.6 1M", Provider: "anthropic", Family: "claude", ContextLength: 1000000, MaxOutput: 16000, InputPrice: 3, OutputPrice: 15, SupportsVision: true, SupportsTools: true},
	{ID: "claude-sonnet-4.6-thinking", Name: "Claude Sonnet 4.6 Thinking", Provider: "anthropic", Family: "claude", ContextLength: 200000, MaxOutput: 16000, InputPrice: 3, OutputPrice: 15, SupportsVision: true, SupportsTools: true},
	{ID: "claude-sonnet-4.6-thinking-1m", Name: "Claude Sonnet 4.6 Thinking 1M", Provider: "anthropic", Family: "claude", ContextLength: 1000000, MaxOutput: 16000, InputPrice: 3, OutputPrice: 15, SupportsVision: true, SupportsTools: true},
	{ID: "claude-4.1-opus", Name: "Claude 4.1 Opus", Provider: "anthropic", Family: "claude", ContextLength: 200000, MaxOutput: 32000, InputPrice: 15, OutputPrice: 75, SupportsVision: true, SupportsTools: true},
	{ID: "claude-4.1-opus-thinking", Name: "Claude 4.1 Opus Thinking", Provider: "anthropic", Family: "claude", ContextLength: 200000, MaxOutput: 32000, InputPrice: 15, OutputPrice: 75, SupportsVision: true, SupportsTools: true},
	{ID: "claude-4-opus", Name: "Claude 4 Opus", Provider: "anthropic", Family: "claude", ContextLength: 200000, MaxOutput: 32000, InputPrice: 15, OutputPrice: 75, SupportsVision: true, SupportsTools: true},
	{ID: "claude-4-opus-thinking", Name: "Claude 4 Opus Thinking", Provider: "anthropic", Family: "claude", ContextLength: 200000, MaxOutput: 32000, InputPrice: 15, OutputPrice: 75, SupportsVision: true, SupportsTools: true},
	{ID: "claude-4-sonnet", Name: "Claude 4 Sonnet", Provider: "anthropic", Family: "claude", ContextLength: 200000, MaxOutput: 16000, InputPrice: 3, OutputPrice: 15, SupportsVision: true, SupportsTools: true},
	{ID: "claude-4-sonnet-thinking", Name: "Claude 4 Sonnet Thinking", Provider: "anthropic", Family: "claude", ContextLength: 200000, MaxOutput: 16000, InputPrice: 3, OutputPrice: 15, SupportsVision: true, SupportsTools: true},
	{ID: "claude-3.7-sonnet", Name: "Claude 3.7 Sonnet", Provider: "anthropic", Family: "claude", ContextLength: 200000, MaxOutput: 16000, InputPrice: 3, OutputPrice: 15, SupportsVision: true, SupportsTools: true},
	{ID: "claude-3.7-sonnet-thinking", Name: "Claude 3.7 Sonnet Thinking", Provider: "anthropic", Family: "claude", ContextLength: 200000, MaxOutput: 16000, InputPrice: 3, OutputPrice: 15, SupportsVision: true, SupportsTools: true},
	{ID: "claude-3.5-sonnet", Name: "Claude 3.5 Sonnet", Provider: "anthropic", Family: "claude", ContextLength: 200000, MaxOutput: 8192, InputPrice: 3, OutputPrice: 15, SupportsVision: true, SupportsTools: true},

	// ── GPT ─────────────────────────────────────────────
	{ID: "gpt-5", Name: "GPT-5", Provider: "openai", Family: "gpt", ContextLength: 272000, MaxOutput: 128000, InputPrice: 10, OutputPrice: 30, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-5-high", Name: "GPT-5 High", Provider: "openai", Family: "gpt", ContextLength: 272000, MaxOutput: 128000, InputPrice: 10, OutputPrice: 30, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-5-medium", Name: "GPT-5 Medium", Provider: "openai", Family: "gpt", ContextLength: 272000, MaxOutput: 128000, InputPrice: 5, OutputPrice: 15, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-5-nano", Name: "GPT-5 Nano", Provider: "openai", Family: "gpt", ContextLength: 272000, MaxOutput: 128000, InputPrice: 0.5, OutputPrice: 2, SupportsVision: true, SupportsTools: true, HiddenFree: true},
	{ID: "gpt-5-codex", Name: "GPT-5 Codex", Provider: "openai", Family: "gpt", ContextLength: 272000, MaxOutput: 128000, InputPrice: 10, OutputPrice: 30, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-5.1", Name: "GPT-5.1", Provider: "openai", Family: "gpt", ContextLength: 272000, MaxOutput: 128000, InputPrice: 10, OutputPrice: 30, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-5.1-fast", Name: "GPT-5.1 Fast", Provider: "openai", Family: "gpt", ContextLength: 272000, MaxOutput: 128000, InputPrice: 5, OutputPrice: 15, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-5.1-high", Name: "GPT-5.1 High", Provider: "openai", Family: "gpt", ContextLength: 272000, MaxOutput: 128000, InputPrice: 10, OutputPrice: 30, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-5.1-high-fast", Name: "GPT-5.1 High Fast", Provider: "openai", Family: "gpt", ContextLength: 272000, MaxOutput: 128000, InputPrice: 10, OutputPrice: 30, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-5.1-medium", Name: "GPT-5.1 Medium", Provider: "openai", Family: "gpt", ContextLength: 272000, MaxOutput: 128000, InputPrice: 5, OutputPrice: 15, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-5.1-medium-fast", Name: "GPT-5.1 Medium Fast", Provider: "openai", Family: "gpt", ContextLength: 272000, MaxOutput: 128000, InputPrice: 5, OutputPrice: 15, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-5.1-low", Name: "GPT-5.1 Low", Provider: "openai", Family: "gpt", ContextLength: 272000, MaxOutput: 128000, InputPrice: 2, OutputPrice: 8, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-5.1-low-fast", Name: "GPT-5.1 Low Fast", Provider: "openai", Family: "gpt", ContextLength: 272000, MaxOutput: 128000, InputPrice: 2, OutputPrice: 8, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-5.2", Name: "GPT-5.2", Provider: "openai", Family: "gpt", ContextLength: 272000, MaxOutput: 128000, InputPrice: 10, OutputPrice: 30, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-5.2-high", Name: "GPT-5.2 High", Provider: "openai", Family: "gpt", ContextLength: 272000, MaxOutput: 128000, InputPrice: 10, OutputPrice: 30, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-5.2-high-fast", Name: "GPT-5.2 High Fast", Provider: "openai", Family: "gpt", ContextLength: 272000, MaxOutput: 128000, InputPrice: 10, OutputPrice: 30, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-5.2-xhigh", Name: "GPT-5.2 XHigh", Provider: "openai", Family: "gpt", ContextLength: 272000, MaxOutput: 128000, InputPrice: 15, OutputPrice: 60, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-5.2-xhigh-fast", Name: "GPT-5.2 XHigh Fast", Provider: "openai", Family: "gpt", ContextLength: 272000, MaxOutput: 128000, InputPrice: 15, OutputPrice: 60, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-5.3-codex", Name: "GPT-5.3 Codex", Provider: "openai", Family: "gpt", ContextLength: 272000, MaxOutput: 128000, InputPrice: 10, OutputPrice: 30, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-5.4-xhigh", Name: "GPT-5.4 XHigh", Provider: "openai", Family: "gpt", ContextLength: 272000, MaxOutput: 128000, InputPrice: 15, OutputPrice: 60, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-5.4-high", Name: "GPT-5.4 High", Provider: "openai", Family: "gpt", ContextLength: 272000, MaxOutput: 128000, InputPrice: 10, OutputPrice: 30, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-5.4-medium", Name: "GPT-5.4 Medium", Provider: "openai", Family: "gpt", ContextLength: 272000, MaxOutput: 128000, InputPrice: 5, OutputPrice: 15, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-5.4-low", Name: "GPT-5.4 Low", Provider: "openai", Family: "gpt", ContextLength: 272000, MaxOutput: 128000, InputPrice: 2, OutputPrice: 8, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-4o", Name: "GPT-4o", Provider: "openai", Family: "gpt", ContextLength: 128000, MaxOutput: 16384, InputPrice: 2.5, OutputPrice: 10, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-4o-mini", Name: "GPT-4o Mini", Provider: "openai", Family: "gpt", ContextLength: 128000, MaxOutput: 16384, InputPrice: 0.15, OutputPrice: 0.6, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-4.1", Name: "GPT-4.1", Provider: "openai", Family: "gpt", ContextLength: 1047576, MaxOutput: 32768, InputPrice: 2, OutputPrice: 8, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-4.1-mini", Name: "GPT-4.1 Mini", Provider: "openai", Family: "gpt", ContextLength: 1047576, MaxOutput: 32768, InputPrice: 0.4, OutputPrice: 1.6, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-4.1-nano", Name: "GPT-4.1 Nano", Provider: "openai", Family: "gpt", ContextLength: 1047576, MaxOutput: 32768, InputPrice: 0.1, OutputPrice: 0.4, SupportsVision: true, SupportsTools: true},
	{ID: "gpt-oss-120b", Name: "GPT OSS 120B", Provider: "openai", Family: "gpt", ContextLength: 128000, MaxOutput: 16384, InputPrice: 1, OutputPrice: 4, SupportsVision: false, SupportsTools: true},

	// ── o 系列 ──────────────────────────────────────────
	{ID: "o3", Name: "O3", Provider: "openai", Family: "o", ContextLength: 200000, MaxOutput: 100000, InputPrice: 10, OutputPrice: 40, SupportsVision: true, SupportsTools: true},
	{ID: "o3-high", Name: "O3 High", Provider: "openai", Family: "o", ContextLength: 200000, MaxOutput: 100000, InputPrice: 10, OutputPrice: 40, SupportsVision: true, SupportsTools: true},
	{ID: "o3-mini", Name: "O3 Mini", Provider: "openai", Family: "o", ContextLength: 200000, MaxOutput: 100000, InputPrice: 1.1, OutputPrice: 4.4, SupportsVision: true, SupportsTools: true},
	{ID: "o3-pro", Name: "O3 Pro", Provider: "openai", Family: "o", ContextLength: 200000, MaxOutput: 100000, InputPrice: 20, OutputPrice: 80, SupportsVision: true, SupportsTools: true},
	{ID: "o4-mini", Name: "O4 Mini", Provider: "openai", Family: "o", ContextLength: 200000, MaxOutput: 100000, InputPrice: 1.1, OutputPrice: 4.4, SupportsVision: true, SupportsTools: true},

	// ── Gemini ──────────────────────────────────────────
	{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", Provider: "google", Family: "gemini", ContextLength: 1048576, MaxOutput: 65536, InputPrice: 0.15, OutputPrice: 0.6, SupportsVision: true, SupportsTools: true, HiddenFree: true},
	{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", Provider: "google", Family: "gemini", ContextLength: 1048576, MaxOutput: 65536, InputPrice: 1.25, OutputPrice: 10, SupportsVision: true, SupportsTools: true},
	{ID: "gemini-3.0-flash", Name: "Gemini 3.0 Flash", Provider: "google", Family: "gemini", ContextLength: 1048576, MaxOutput: 65536, InputPrice: 0.15, OutputPrice: 0.6, SupportsVision: true, SupportsTools: true},
	{ID: "gemini-3.0-flash-high", Name: "Gemini 3.0 Flash High", Provider: "google", Family: "gemini", ContextLength: 1048576, MaxOutput: 65536, InputPrice: 0.15, OutputPrice: 0.6, SupportsVision: true, SupportsTools: true},
	{ID: "gemini-3.0-flash-low", Name: "Gemini 3.0 Flash Low", Provider: "google", Family: "gemini", ContextLength: 1048576, MaxOutput: 65536, InputPrice: 0.075, OutputPrice: 0.3, SupportsVision: true, SupportsTools: true},
	{ID: "gemini-3.0-flash-minimal", Name: "Gemini 3.0 Flash Minimal", Provider: "google", Family: "gemini", ContextLength: 1048576, MaxOutput: 65536, InputPrice: 0.075, OutputPrice: 0.3, SupportsVision: true, SupportsTools: true, HiddenFree: true},
	{ID: "gemini-3.0-pro", Name: "Gemini 3.0 Pro", Provider: "google", Family: "gemini", ContextLength: 1048576, MaxOutput: 65536, InputPrice: 1.25, OutputPrice: 10, SupportsVision: true, SupportsTools: true},
	{ID: "gemini-3.1-pro-high", Name: "Gemini 3.1 Pro High", Provider: "google", Family: "gemini", ContextLength: 1048576, MaxOutput: 65536, InputPrice: 1.25, OutputPrice: 10, SupportsVision: true, SupportsTools: true},
	{ID: "gemini-3.1-pro-low", Name: "Gemini 3.1 Pro Low", Provider: "google", Family: "gemini", ContextLength: 1048576, MaxOutput: 65536, InputPrice: 0.3, OutputPrice: 2, SupportsVision: true, SupportsTools: true},

	// ── DeepSeek ────────────────────────────────────────
	{ID: "deepseek-3.2", Name: "DeepSeek 3.2", Provider: "deepseek", Family: "deepseek", ContextLength: 128000, MaxOutput: 32000, InputPrice: 0.27, OutputPrice: 1.1, SupportsVision: false, SupportsTools: true},
	{ID: "deepseek-v3", Name: "DeepSeek V3", Provider: "deepseek", Family: "deepseek", ContextLength: 128000, MaxOutput: 32000, InputPrice: 0.27, OutputPrice: 1.1, SupportsVision: false, SupportsTools: true},
	{ID: "deepseek-r1", Name: "DeepSeek R1", Provider: "deepseek", Family: "deepseek", ContextLength: 128000, MaxOutput: 32000, InputPrice: 0.55, OutputPrice: 2.19, SupportsVision: false, SupportsTools: true},

	// ── GLM ─────────────────────────────────────────────
	{ID: "glm-4.7", Name: "GLM 4.7", Provider: "zhipu", Family: "glm", ContextLength: 128000, MaxOutput: 16000, InputPrice: 0, OutputPrice: 0, SupportsVision: false, SupportsTools: true, HiddenFree: true},
	{ID: "glm-5", Name: "GLM 5", Provider: "zhipu", Family: "glm", ContextLength: 128000, MaxOutput: 16000, InputPrice: 0, OutputPrice: 0, SupportsVision: false, SupportsTools: true, HiddenFree: true},
	{ID: "glm-5.1", Name: "GLM 5.1", Provider: "zhipu", Family: "glm", ContextLength: 128000, MaxOutput: 16000, InputPrice: 0, OutputPrice: 0, SupportsVision: false, SupportsTools: true, HiddenFree: true},

	// ── Kimi ────────────────────────────────────────────
	{ID: "kimi-k2", Name: "Kimi K2", Provider: "moonshot", Family: "kimi", ContextLength: 128000, MaxOutput: 16000, InputPrice: 0, OutputPrice: 0, SupportsVision: false, SupportsTools: true, HiddenFree: true},
	{ID: "kimi-k2-6", Name: "Kimi K2.6", Provider: "moonshot", Family: "kimi", ContextLength: 128000, MaxOutput: 16000, InputPrice: 0, OutputPrice: 0, SupportsVision: false, SupportsTools: true, HiddenFree: true},
	{ID: "kimi-k2.5", Name: "Kimi K2.5", Provider: "moonshot", Family: "kimi", ContextLength: 128000, MaxOutput: 16000, InputPrice: 0, OutputPrice: 0, SupportsVision: false, SupportsTools: true, HiddenFree: true},

	// ── Grok ────────────────────────────────────────────
	{ID: "grok-3", Name: "Grok 3", Provider: "xai", Family: "grok", ContextLength: 131072, MaxOutput: 16384, InputPrice: 3, OutputPrice: 15, SupportsVision: true, SupportsTools: true},
	{ID: "grok-3-mini-thinking", Name: "Grok 3 Mini Thinking", Provider: "xai", Family: "grok", ContextLength: 131072, MaxOutput: 16384, InputPrice: 0.3, OutputPrice: 0.5, SupportsVision: true, SupportsTools: true},
	{ID: "grok-code-fast-1", Name: "Grok Code Fast 1", Provider: "xai", Family: "grok", ContextLength: 131072, MaxOutput: 16384, InputPrice: 0.3, OutputPrice: 0.5, SupportsVision: false, SupportsTools: true},

	// ── MiniMax ─────────────────────────────────────────
	{ID: "minimax-m2.5", Name: "MiniMax M2.5", Provider: "minimax", Family: "minimax", ContextLength: 128000, MaxOutput: 16000, InputPrice: 0, OutputPrice: 0, SupportsVision: false, SupportsTools: true, HiddenFree: true},

	// ── SWE ─────────────────────────────────────────────
	{ID: "swe-1.5", Name: "SWE 1.5", Provider: "windsurf", Family: "swe", ContextLength: 272000, MaxOutput: 128000, InputPrice: 0, OutputPrice: 0, SupportsVision: false, SupportsTools: true},
	{ID: "swe-1.5-fast", Name: "SWE 1.5 Fast", Provider: "windsurf", Family: "swe", ContextLength: 272000, MaxOutput: 128000, InputPrice: 0, OutputPrice: 0, SupportsVision: false, SupportsTools: true},
	{ID: "swe-1.6", Name: "SWE 1.6", Provider: "windsurf", Family: "swe", ContextLength: 272000, MaxOutput: 128000, InputPrice: 0, OutputPrice: 0, SupportsVision: false, SupportsTools: true},
	{ID: "swe-1.6-fast", Name: "SWE 1.6 Fast", Provider: "windsurf", Family: "swe", ContextLength: 272000, MaxOutput: 128000, InputPrice: 0, OutputPrice: 0, SupportsVision: false, SupportsTools: true},

	// ── Arena ───────────────────────────────────────────
	{ID: "arena-fast", Name: "Arena Fast", Provider: "windsurf", Family: "arena", ContextLength: 200000, MaxOutput: 32000, InputPrice: 0, OutputPrice: 0, SupportsVision: false, SupportsTools: true},
	{ID: "arena-smart", Name: "Arena Smart", Provider: "windsurf", Family: "arena", ContextLength: 200000, MaxOutput: 32000, InputPrice: 0, OutputPrice: 0, SupportsVision: false, SupportsTools: true},

	// ── Cascade (default) ──────────────────────────────
	{ID: "cascade", Name: "Cascade (Default)", Provider: "windsurf", Family: "cascade", ContextLength: 200000, MaxOutput: 32000, InputPrice: 0, OutputPrice: 0, SupportsVision: false, SupportsTools: true},
}

// ── /v1/chat/completions ──

type openAIChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   *bool         `json:"stream,omitempty"`
}

func (r *OpenAIRelay) handleChatCompletions(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		writeOpenAIError(w, 405, "method_not_allowed", "POST only")
		return
	}
	if !r.checkAuth(w, req) {
		return
	}

	var chatReq openAIChatRequest
	if err := json.NewDecoder(req.Body).Decode(&chatReq); err != nil {
		writeOpenAIError(w, 400, "invalid_request", err.Error())
		return
	}
	if len(chatReq.Messages) == 0 {
		writeOpenAIError(w, 400, "invalid_request", "messages is required")
		return
	}

	stream := chatReq.Stream != nil && *chatReq.Stream
	startTime := time.Now()

	// 估算 prompt tokens
	promptTokens := 0
	for _, m := range chatReq.Messages {
		promptTokens += estimateTokens(m.Content)
	}

	// 从账号池拿 key + JWT（支持额度耗尽 / 认证失败自动轮转重试）
	var upstreamResp *http.Response
	var usedKey string
	for attempt := 0; attempt <= r.maxRetry; attempt++ {
		apiKey, jwtBytes := r.proxy.pickPoolKeyAndJWT()
		if apiKey == "" || len(jwtBytes) == 0 {
			writeOpenAIError(w, 503, "no_accounts", "No available accounts in pool")
			return
		}
		jwtStr := string(jwtBytes)
		usedKey = apiKey

		if attempt == 0 {
			r.log("chat request: model=%s messages=%d stream=%v key=%s...", chatReq.Model, len(chatReq.Messages), stream, truncKey(apiKey))
		}

		r.proxy.mu.RLock()
		chatFP := r.proxy.keyFingerprint(apiKey)
		// F7-REMOVAL: 下一行 smartFriend 读取删除；下面调用改回 BuildChatRequestWithModel(...) 不传 smartFriend
		smartFriend := r.proxy.smartFriendEnabled
		r.proxy.mu.RUnlock()
		protoBody := buildChatRequestWithModelMode(chatReq.Messages, apiKey, jwtStr, "", chatReq.Model, chatFP, smartFriend)
		// Connect 协议：直接发送 protobuf body（无 envelope）
		// 有 envelope 返回 invalid_argument，无 envelope 返回 resource_exhausted（更接近成功）
		resp, kind, err := r.sendGRPC(protoBody, apiKey, jwtStr)
		if err != nil {
			if kind == upstreamFailureQuota {
				r.log("额度耗尽 key=%s... 自动轮转重试(%d/%d)", truncKey(apiKey), attempt+1, r.maxRetry)
				r.proxy.markRuntimeExhaustedAndRotate(apiKey, "relay-quota")
				continue
			}
			if kind == upstreamFailureGlobalRateLimit {
				r.log("全局限速命中 key=%s..., 放弃重试", truncKey(apiKey))
				writeRelayUpstreamFailure(w, kind, err.Error())
				return
			}
			if kind == upstreamFailureRateLimit {
				r.log("限速命中 key=%s... 自动轮转重试(%d/%d)", truncKey(apiKey), attempt+1, r.maxRetry)
				if rotatedKey := r.proxy.markRateLimitedAndRotate(apiKey, "relay-rate-limit="+err.Error()); rotatedKey != "" {
					continue
				}
				writeRelayUpstreamFailure(w, kind, err.Error())
				return
			}
			if kind == upstreamFailureAuth {
				r.log("认证失败 key=%s... 优先切换到下一把 key(%d/%d)", truncKey(apiKey), attempt+1, r.maxRetry)
				if rotatedKey := r.proxy.rotateAfterAuthFailure(apiKey, "relay-auth="+err.Error()); rotatedKey != "" {
					continue
				}
				r.log("无可用备用 key，回退刷新当前 JWT: %s...", truncKey(apiKey))
				refreshed := r.proxy.refreshJWTForKey(apiKey)
				if len(refreshed) > 0 {
					continue // 用刷新后的 JWT 重试（pickPoolKeyAndJWT 会拿到新 JWT）
				}
				r.log("JWT 刷新失败，保留当前认证错误")
				writeRelayUpstreamFailure(w, kind, err.Error())
				return
			}
			r.log("gRPC error (kind=%s): %v", string(kind), err)
			writeOpenAIError(w, 502, "upstream_error", err.Error())
			return
		}
		upstreamResp = resp
		break
	}
	if upstreamResp == nil {
		writeOpenAIError(w, 503, "all_exhausted", "All accounts in pool are exhausted")
		return
	}
	defer upstreamResp.Body.Close()

	chatID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	model := chatReq.Model
	if model == "" {
		model = "cascade"
	}

	var finalKind upstreamFailureKind
	var finalDetail string
	var completionTokens int
	if stream {
		completionTokens, finalKind, finalDetail = r.streamResponse(w, upstreamResp, chatID, model)
	} else {
		completionTokens, finalKind, finalDetail = r.blockingResponse(w, upstreamResp, chatID, model, promptTokens)
	}
	r.finalizeRelayOutcome(usedKey, finalKind, finalDetail)

	// 记录用量
	status := "ok"
	if finalKind != upstreamFailureNone {
		status = "error"
	}
	r.recordUsage(UsageRecord{
		Model:            model,
		RequestModel:     chatReq.Model,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
		DurationMs:       time.Since(startTime).Milliseconds(),
		APIKeyShort:      truncKey(usedKey),
		Status:           status,
		ErrorDetail:      finalDetail,
		Format:           "openai",
	})
}

// buildUpstreamTransport 构建持久化 transport（与 MITM 上游一致，http.Transport + ForceAttemptHTTP2）
func (r *OpenAIRelay) buildUpstreamTransport() http.RoundTripper {
	t := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         GRPCUpstreamHost,
			NextProtos:         []string{"h2"},
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          50,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 120 * time.Second,
	}
	if r.proxyURL != "" {
		if u, err := url.Parse(r.proxyURL); err == nil {
			t.Proxy = http.ProxyURL(u)
			r.log("出站代理: %s", r.proxyURL)
		}
	}
	// 显式配置 HTTP/2（gRPC 必须 h2）
	if err := http2.ConfigureTransport(t); err != nil {
		r.log("http2.ConfigureTransport 失败: %v (回退 ForceAttemptHTTP2)", err)
	}
	r.log("transport built: ServerName=%s h2=explicit proxy=%s", GRPCUpstreamHost, r.proxyURL)
	return t
}

func isTransientRelayRoundTripError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(text, "eof") ||
		strings.Contains(text, "connection reset by peer") ||
		strings.Contains(text, "server closed idle connection") ||
		strings.Contains(text, "use of closed network connection") ||
		strings.Contains(text, "client connection lost")
}

func (r *OpenAIRelay) rebuildUpstreamTransport() http.RoundTripper {
	transport := r.buildUpstreamTransport()
	r.mu.Lock()
	r.upstream = transport
	r.mu.Unlock()
	return transport
}

func (r *OpenAIRelay) currentUpstreamTransport() http.RoundTripper {
	r.mu.RLock()
	transport := r.upstream
	r.mu.RUnlock()
	if transport != nil {
		return transport
	}
	return r.rebuildUpstreamTransport()
}

func buildGetChatMessageRequest(upIP string, payload []byte, jwt string) (*http.Request, error) {
	connectURL := fmt.Sprintf("https://%s/exa.api_server_pb.ApiServerService/GetChatMessage", upIP)
	// ★ IDE 实际发送 gzip-compressed Connect envelope（flag=0x01 + gzip(payload)）。
	// 之前裸发 resource_exhausted，加普通 envelope 返回 invalid_argument——必须 gzip envelope。
	wrapped := WrapGRPCEnvelopeGzip(payload)
	req, err := http.NewRequest(http.MethodPost, connectURL, bytes.NewReader(wrapped))
	if err != nil {
		return nil, err
	}

	req.Host = GRPCUpstreamHost
	req.Header.Set("Content-Type", "application/connect+proto")
	req.Header.Set("Connect-Protocol-Version", "1")
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("User-Agent", "connect-go/1.18.1 (go1.26.1)")
	req.Header.Set("Accept-Encoding", "identity")
	// Connect 流式帧级压缩协商：每帧内部 gzip
	req.Header.Set("Connect-Content-Encoding", "gzip")
	req.Header.Set("Connect-Accept-Encoding", "gzip")
	return req, nil
}

// sendGRPC 向 Windsurf 上游发送 Connect 流式请求，返回完整响应与失败分类。
// 同时检测 trailers-only 模式（HTTP 200 但 grpc-status 头非零）。
func (r *OpenAIRelay) sendGRPC(payload []byte, apiKey, jwt string) (*http.Response, upstreamFailureKind, error) {
	upIP := ResolveUpstreamIP()

	var resp *http.Response
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		httpReq, reqErr := buildGetChatMessageRequest(upIP, payload, jwt)
		if reqErr != nil {
			return nil, upstreamFailureNone, reqErr
		}

		transport := r.currentUpstreamTransport()
		r.log("sendGRPC → %s (host=%s) payload=%dB attempt=%d", upIP, GRPCUpstreamHost, len(payload), attempt+1)
		resp, err = transport.RoundTrip(httpReq)
		if err == nil {
			break
		}
		if !isTransientRelayRoundTripError(err) || attempt == 1 {
			return nil, upstreamFailureNone, fmt.Errorf("grpc roundtrip to %s: %w", upIP, err)
		}
		r.log("sendGRPC transient error: %v; rebuild transport and retry", err)
		if _, ok := transport.(*http.Transport); ok {
			r.rebuildUpstreamTransport()
		}
	}

	grpcStatus := resp.Header.Get("grpc-status")
	grpcMsg := resp.Header.Get("grpc-message")

	// 非 200 或 Trailers-Only 错误（HTTP 200 + grpc-status 头非空非 0）
	isHTTPErr := resp.StatusCode != 200
	isTrailersOnlyErr := grpcStatus != "" && grpcStatus != "0"
	if isHTTPErr || isTrailersOnlyErr {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		kind, detail := classifyUpstreamFailure(grpcStatus, grpcMsg, string(body))
		r.log("sendGRPC error: ip=%s status=%d proto=%s grpc-status=%s kind=%s detail=%s body=%s",
			upIP, resp.StatusCode, resp.Proto, grpcStatus, string(kind), detail, truncate(string(body), 200))
		if detail == "" {
			detail = fmt.Sprintf("upstream HTTP %d (proto=%s), grpc-status=%s, grpc-message=%s", resp.StatusCode, resp.Proto, grpcStatus, grpcMsg)
		}
		return nil, kind, fmt.Errorf("%s", detail)
	}
	r.log("sendGRPC ok: proto=%s status=%d", resp.Proto, resp.StatusCode)

	// ★ gRPC streaming: 检查 trailers 中的错误（grpc-status 在 trailers 里，不在 headers 里）
	// 先 peek body：如果 body 为空且 trailers 有错误，提前返回
	// 注意：不能 io.ReadAll 因为 streamResponse 需要读 body
	// 改为在 streamResponse 末尾检查 trailers
	return resp, upstreamFailureNone, nil
}

// streamResponse 将 gRPC 流式响应转为 SSE。
// 返回值用于调用方判断这次流是正常完成，还是在流尾 / trailer 处以 quota/auth/grpc 失败收尾。
func (r *OpenAIRelay) streamResponse(w http.ResponseWriter, resp *http.Response, chatID, model string) (int, upstreamFailureKind, string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeOpenAIError(w, 500, "internal", "streaming not supported")
		return 0, upstreamFailureGRPC, "streaming not supported"
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(200)

	body := resp.Body
	reader := bufio.NewReaderSize(body, 32768)
	buf := make([]byte, 0, 65536)
	// ★ 性能：tmp 移到循环外，避免每次 Read 都 make 8KB 临时 slice。
	// 一次 chat 流式响应可能 100+ 次 Read，旧实现每次新建 → 800KB+ 临时分配 + GC 抖动。
	tmp := make([]byte, 8192)
	sawTerminalFrame := false
	completionTokens := 0

	for {
		n, readErr := reader.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}

		// 尝试从 buf 中提取完整的 gRPC 帧
		for len(buf) >= 5 {
			flags := buf[0]
			envelopeLen := int(buf[1])<<24 | int(buf[2])<<16 | int(buf[3])<<8 | int(buf[4])
			totalLen := 5 + envelopeLen
			if len(buf) < totalLen {
				break
			}
			// ★ 性能：直接用 buf[5:totalLen] 切片，不复制。
			// decodeStreamEnvelopePayload 解压后返回新 slice / 不压缩时显式 append nil 复制，
			// 都不会保留对 buf 底层数组的引用 → 后面 buf = buf[totalLen:] 切片安全。
			framePayload := buf[5:totalLen]

			decodedPayload, err := decodeStreamEnvelopePayload(flags, framePayload)
			// 在 decodeStreamEnvelopePayload 之后再 advance buf，避免 framePayload 在
			// decodeStreamEnvelopePayload 调用期间被 buf 重用覆盖。
			buf = buf[totalLen:]
			if err != nil {
				continue
			}
			if flags&streamEnvelopeEndStream != 0 {
				if kind, detail := classifyUpstreamFailure("", "", string(decodedPayload)); kind != upstreamFailureNone {
					return completionTokens, kind, detail
				}
				sawTerminalFrame = true
				continue
			}

			text, isDone, err := ParseChatResponseChunk(decodedPayload)
			if err != nil {
				continue
			}
			if text != "" {
				completionTokens += estimateTokens(text)
				chunk := buildSSEChunk(chatID, model, text, false)
				fmt.Fprintf(w, "data: %s\n\n", chunk)
				flusher.Flush()
			}
			if isDone {
				sawTerminalFrame = true
			}
		}

		if readErr != nil {
			if readErr != io.EOF {
				return completionTokens, upstreamFailureGRPC, readErr.Error()
			}
			if len(buf) > 0 {
				return completionTokens, upstreamFailureGRPC, "stream ended with incomplete grpc frame"
			}
			if kind, detail := classifyUpstreamFailure(resp.Trailer.Get("grpc-status"), resp.Trailer.Get("grpc-message"), ""); kind != upstreamFailureNone {
				return completionTokens, kind, detail
			}
			// 正常结束时才向 OpenAI SSE 客户端发 stop + [DONE]。
			// 这样 quota/auth/trailer 失败不会再伪装成一次成功完成的响应。
			_ = sawTerminalFrame // EOF without trailer failure也按正常结束处理，避免客户端悬挂。
			chunk := buildSSEChunk(chatID, model, "", true)
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			return completionTokens, upstreamFailureNone, ""
		}
	}
}

// blockingResponse 收集所有响应后一次性返回
func (r *OpenAIRelay) blockingResponse(w http.ResponseWriter, resp *http.Response, chatID, model string, promptTokens int) (int, upstreamFailureKind, string) {
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		writeOpenAIError(w, 502, "upstream_error", err.Error())
		return 0, upstreamFailureGRPC, err.Error()
	}
	if kind, detail := classifyUpstreamFailure(resp.Trailer.Get("grpc-status"), resp.Trailer.Get("grpc-message"), string(data)); kind != upstreamFailureNone {
		writeRelayUpstreamFailure(w, kind, detail)
		return 0, kind, detail
	}

	frames := ExtractGRPCFrames(data)
	var fullText strings.Builder
	for _, frame := range frames {
		text, _, _ := ParseChatResponseChunk(frame)
		fullText.WriteString(text)
	}

	completionTokens := estimateTokens(fullText.String())

	reply := map[string]interface{}{
		"id":      chatID,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]interface{}{
			{
				"index":         0,
				"message":       map[string]string{"role": "assistant", "content": fullText.String()},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]int{"prompt_tokens": promptTokens, "completion_tokens": completionTokens, "total_tokens": promptTokens + completionTokens},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reply)
	return completionTokens, upstreamFailureNone, ""
}

// ── 辅助 ──

func buildSSEChunk(id, model, content string, isStop bool) string {
	delta := map[string]string{}
	if content != "" {
		delta["content"] = content
	}
	finishReason := interface{}(nil)
	if isStop {
		finishReason = "stop"
	}
	chunk := map[string]interface{}{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]interface{}{
			{"index": 0, "delta": delta, "finish_reason": finishReason},
		},
	}
	b, _ := json.Marshal(chunk)
	return string(b)
}

func writeOpenAIError(w http.ResponseWriter, status int, errType, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := map[string]interface{}{
		"error": map[string]interface{}{
			"message": msg,
			"type":    errType,
			"code":    errType,
		},
	}
	json.NewEncoder(w).Encode(resp)
}

func writeRelayUpstreamFailure(w http.ResponseWriter, kind upstreamFailureKind, detail string) {
	status := 502
	errType := "upstream_error"
	switch kind {
	case upstreamFailureQuota:
		status = 429
		errType = "quota_exhausted"
	case upstreamFailureRateLimit:
		status = 429
		errType = "rate_limit"
	case upstreamFailureGlobalRateLimit:
		status = 429
		errType = "global_rate_limit"
	case upstreamFailureAuth:
		status = 401
		errType = "authentication_error"
	case upstreamFailurePermission:
		status = 403
		errType = "permission_denied"
	}
	if strings.TrimSpace(detail) == "" {
		detail = "upstream request failed"
	}
	writeOpenAIError(w, status, errType, detail)
}

func (r *OpenAIRelay) finalizeRelayOutcome(apiKey string, kind upstreamFailureKind, detail string) {
	if strings.TrimSpace(apiKey) == "" {
		return
	}
	if kind == upstreamFailureNone {
		r.proxy.RecordKeySuccess(apiKey)
		r.mu.RLock()
		cb := r.onSuccess
		r.mu.RUnlock()
		if cb != nil {
			go cb(apiKey)
		}
		return
	}

	detail = strings.TrimSpace(detail)
	switch kind {
	case upstreamFailureQuota:
		r.log("relay 结束为额度失败: key=%s... detail=%s", truncKey(apiKey), truncate(detail, 180))
		r.proxy.markRuntimeExhaustedAndRotate(apiKey, "relay-finished="+detail)
	case upstreamFailureRateLimit:
		r.log("relay 结束为限速失败: key=%s... detail=%s", truncKey(apiKey), truncate(detail, 180))
		r.proxy.markRateLimitedAndRotate(apiKey, "relay-rate-limit="+detail)
	case upstreamFailureAuth:
		r.log("relay 结束为认证失败: key=%s... detail=%s", truncKey(apiKey), truncate(detail, 180))
		// rotateAfterAuthFailure 已经在后台 goroutine 里调 refreshJWTForKey；
		// 这里再 sync 调一次会造成双 leader 竞争 + 异步 goroutine 可能在
		// test cleanup 之后才执行 → race。下次请求若 JWT 仍未刷新成功，会
		// 自然走 ensureJWTForKey 重新触发，无需在这里冗余刷新。
		_ = r.proxy.rotateAfterAuthFailure(apiKey, "relay-auth="+detail)
	default:
		r.log("relay 结束为上游失败: key=%s... kind=%s detail=%s", truncKey(apiKey), kind, truncate(detail, 180))
	}
}

func truncKey(key string) string {
	if len(key) > 12 {
		return key[:12]
	}
	return key
}

// ── 用量追踪 ──

func (r *OpenAIRelay) recordUsage(rec UsageRecord) {
	if r.usageTracker != nil {
		r.usageTracker.Record(rec)
	}
}

// GetUsageRecords 返回用量记录（App 层调用）
func (r *OpenAIRelay) GetUsageRecords(limit int) []UsageRecord {
	if r.usageTracker == nil {
		return nil
	}
	return r.usageTracker.GetRecords(limit)
}

// GetUsageSummary 返回用量汇总（App 层调用）
func (r *OpenAIRelay) GetUsageSummary() UsageSummary {
	if r.usageTracker == nil {
		return UsageSummary{}
	}
	return r.usageTracker.GetSummary()
}

// DeleteAllUsage 清空所有用量记录
func (r *OpenAIRelay) DeleteAllUsage() int {
	if r.usageTracker == nil {
		return 0
	}
	return r.usageTracker.DeleteAll()
}

// DeleteUsageBefore 删除指定天数之前的记录
func (r *OpenAIRelay) DeleteUsageBefore(days int) int {
	if r.usageTracker == nil {
		return 0
	}
	before := time.Now().AddDate(0, 0, -days)
	return r.usageTracker.DeleteBefore(before)
}

// handleUsageAPI 用量记录 API: GET=查询, DELETE=清除
func (r *OpenAIRelay) handleUsageAPI(w http.ResponseWriter, req *http.Request) {
	if !r.checkAuth(w, req) {
		return
	}
	switch req.Method {
	case http.MethodGet:
		limit := 100
		if q := req.URL.Query().Get("limit"); q != "" {
			fmt.Sscanf(q, "%d", &limit)
		}
		records := r.GetUsageRecords(limit)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(records)
	case http.MethodDelete:
		n := r.DeleteAllUsage()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"deleted": n})
	default:
		writeOpenAIError(w, 405, "method_not_allowed", "GET or DELETE only")
	}
}

// handleUsageSummaryAPI 用量汇总 API
func (r *OpenAIRelay) handleUsageSummaryAPI(w http.ResponseWriter, req *http.Request) {
	if !r.checkAuth(w, req) {
		return
	}
	summary := r.GetUsageSummary()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}
