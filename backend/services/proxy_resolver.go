package services

// proxy_resolver.go ── 上游代理选择。
//
// 优先级（高 → 低）：
//   1. clash + 节点：ClashRotateEnabled=true 且能从 controller /configs 探到
//      代理端口 → http://<host>:<port> 或 socks5://<host>:<port>
//   2. clash：ClashControllerURL 非空且能探到入口端口 → 同上
//   3. 系统代理：通过 httpproxy.FromEnvironment 读 HTTPS_PROXY/HTTP_PROXY/NO_PROXY
//   4. 默认：直连（返回 ""）
//
// scheme 选择：
//   - mixed-port：HTTP+SOCKS5 双协议 → http://（http.Transport 原生支持）
//   - port：HTTP → http://
//   - socks-port：SOCKS5 → socks5://
//
// http.Transport.Proxy 函数会根据 URL.Scheme 自动选协议（Go 1.10+ 支持 socks5）。
//
// 系统代理只看环境变量；要让 Windows 系统代理生效，请在启动 shell / 任务计划
// 里 export HTTPS_PROXY。这是跨平台最一致、零 syscall 的做法。

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http/httpproxy"
)

// probeClashCacheTTL 控制 probeClashProxyEntry 缓存有效期。
// 设短了可以让 clash 重启 / 端口变化更快被感知；设长了能挡住 UI 连点导致的
// 重复 3s 探活。2s 是经验值：UI 连击窗口（用户保存设置按钮 / Settings 多入口
// 同时触发）通常 <1s，clash 真正换端口属于罕见操作。
const probeClashCacheTTL = 2 * time.Second

var (
	probeClashMu   sync.Mutex
	probeClashKey  string    // 缓存 key = controllerURL|secret
	probeClashURL  string    // 上次探到的 proxy URL（含 scheme + host + port）
	probeClashTime time.Time // 上次探活完成时刻
)

// ProxySource 给日志/状态展示用。
type ProxySource string

const (
	ProxySourceNone      ProxySource = "direct"
	ProxySourceClashWith ProxySource = "clash+nodes"
	ProxySourceClashOnly ProxySource = "clash"
	ProxySourceSystem    ProxySource = "system"
)

// ResolveUpstreamProxy 按优先级返回上游代理 URL 与来源。任一档失败降级到下一档。
func ResolveUpstreamProxy(controllerURL, secret string, clashRotateEnabled bool) (string, ProxySource) {
	if proxyURL := probeClashProxyEntry(controllerURL, secret); proxyURL != "" {
		if clashRotateEnabled {
			return proxyURL, ProxySourceClashWith
		}
		return proxyURL, ProxySourceClashOnly
	}
	if proxyURL := readSystemProxy(); proxyURL != "" {
		return proxyURL, ProxySourceSystem
	}
	return "", ProxySourceNone
}

// probeClashProxyEntry 通过 Clash external-controller /configs 探出代理端口。
// 优先级：mixed-port > port > socks-port。host 取自 controllerURL，远程 Clash 也对。
//
// 加 probeClashCacheTTL (2s) 的内存缓存：连续 apply（UI 连点 / 多入口同时触发）
// 时复用上次结果，避免重复跑 3s HTTP 探活把 UI 卡住。失败结果（""）也会缓存，
// 防止 clash 没起时高频空打。
func probeClashProxyEntry(controllerURL, secret string) string {
	controllerURL = strings.TrimRight(strings.TrimSpace(controllerURL), "/")
	if controllerURL == "" {
		return ""
	}
	cacheKey := controllerURL + "|" + secret

	probeClashMu.Lock()
	if probeClashKey == cacheKey && time.Since(probeClashTime) < probeClashCacheTTL {
		v := probeClashURL
		probeClashMu.Unlock()
		return v
	}
	probeClashMu.Unlock()

	result := doProbeClashProxyEntry(controllerURL, secret)

	probeClashMu.Lock()
	probeClashKey = cacheKey
	probeClashURL = result
	probeClashTime = time.Now()
	probeClashMu.Unlock()

	return result
}

// doProbeClashProxyEntry 是 probeClashProxyEntry 不带缓存的内核实现。
func doProbeClashProxyEntry(controllerURL, secret string) string {
	parsed, err := url.Parse(controllerURL)
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	req, err := http.NewRequest(http.MethodGet, controllerURL+"/configs", nil)
	if err != nil {
		return ""
	}
	if secret = strings.TrimSpace(secret); secret != "" {
		// 同时设 Authorization header 和 ?secret= query：
		// - mihomo / clash-meta / 主线 Clash 标准是读 Authorization Bearer
		// - 部分 fork（老 clash-premium 等）只读 query 参数
		// 两个都加无副作用，提升兼容性。
		req.Header.Set("Authorization", "Bearer "+secret)
		q := req.URL.Query()
		q.Set("secret", secret)
		req.URL.RawQuery = q.Encode()
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return ""
	}
	var cfg struct {
		MixedPort int `json:"mixed-port"`
		Port      int `json:"port"`
		SocksPort int `json:"socks-port"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return ""
	}
	host := parsed.Hostname()
	switch {
	case cfg.MixedPort > 0:
		return fmt.Sprintf("http://%s:%d", host, cfg.MixedPort)
	case cfg.Port > 0:
		return fmt.Sprintf("http://%s:%d", host, cfg.Port)
	case cfg.SocksPort > 0:
		// 注：Go 标准库 net/http 的 socks5:// dialer 内部通过
		// golang.org/x/net/internal/socks 实现，默认就把目标 host 作为
		// AddrTypeFQDN 发给 SOCKS server（即等价 socks5h 远程 DNS 行为）。
		// 所以这里直接用 socks5:// 是安全的，不需要额外 socks5h scheme。
		return fmt.Sprintf("socks5://%s:%d", host, cfg.SocksPort)
	}
	// 探活通了但三个入口都关 —— 罕见但有人这么配，至少留条日志线索免得排障靠猜
	log.Printf("[proxy] clash controller %s 探活成功但 mixed-port/port/socks-port 全为 0，无可用入口端口", controllerURL)
	return ""
}

// InvalidateClashProbeCache 强制下一次 probeClashProxyEntry 重打 HTTP。
// 用于 AutoSetupClash 等"我明确知道 clash 状态变了"的入口，绕过 TTL。
func InvalidateClashProbeCache() {
	probeClashMu.Lock()
	probeClashKey = ""
	probeClashURL = ""
	probeClashTime = time.Time{}
	probeClashMu.Unlock()
}

// systemProxyProbeHosts 是 readSystemProxy 用于匹配 ProxyFunc 的目标域名。
// 任一域命中即返回代理 —— 主要应对 NO_PROXY 排除了 windsurf 但其它跨墙域名
// 仍走代理的情况（用户用 MITM 把流量从 windsurf 域转给 anthropic / openai 时）。
var systemProxyProbeHosts = []string{
	"server.self-serve.windsurf.com",
	"api.anthropic.com",
	"api.openai.com",
}

// readSystemProxy 读取系统代理。
//
// 用 httpproxy.FromEnvironment() 处理 HTTPS_PROXY/HTTP_PROXY/NO_PROXY 大小写、
// URL 归一化（http.ProxyFromEnvironment 的底层实现）。
//
// 用多个主流量 https URL 依次探活 —— httpproxy 按目标 URL 匹配代理，传 https
// 让 HTTPS_PROXY 优先于 HTTP_PROXY，与实际请求一致。任一命中即返回。
func readSystemProxy() string {
	cfg := httpproxy.FromEnvironment()
	proxyFunc := cfg.ProxyFunc()
	for _, host := range systemProxyProbeHosts {
		probe := &url.URL{Scheme: "https", Host: host}
		if u, err := proxyFunc(probe); err == nil && u != nil {
			return u.String()
		}
	}
	return ""
}
