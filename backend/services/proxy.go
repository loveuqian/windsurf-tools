package services

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"windsurf-tools-wails/backend/utils"

	"golang.org/x/net/http2"
)

// ── 动态 DNS 解析（兼容 VPN / IP 漂移） ──

var (
	resolvedIP string
	resolvedAt time.Time
	resolveMu  sync.RWMutex
	resolveTTL = 5 * time.Minute
)

// ResolveUpstreamIP 动态解析上游 IP，带缓存（TTL 5 分钟），失败时回退硬编码。
func ResolveUpstreamIP() string {
	resolveMu.RLock()
	if resolvedIP != "" && time.Since(resolvedAt) < resolveTTL {
		ip := resolvedIP
		resolveMu.RUnlock()
		return ip
	}
	resolveMu.RUnlock()

	resolveMu.Lock()
	defer resolveMu.Unlock()
	// double-check after acquiring write lock
	if resolvedIP != "" && time.Since(resolvedAt) < resolveTTL {
		return resolvedIP
	}

	ips, err := net.LookupHost(UpstreamHost)
	if err == nil {
		for _, ip := range ips {
			if !strings.HasPrefix(ip, "127.") && !strings.Contains(ip, ":") {
				resolvedIP = ip
				resolvedAt = time.Now()
				log.Printf("[DNS] %s → %s", UpstreamHost, ip)
				return ip
			}
		}
	}
	// DNS 失败或返回 127.x（已被 hosts 劫持），回退硬编码
	if resolvedIP != "" {
		return resolvedIP // 用上次缓存
	}
	if err != nil {
		log.Printf("[DNS] 解析 %s 失败(%v)，回退 %s", UpstreamHost, err, UpstreamIP)
	}
	return UpstreamIP
}

const (
	TargetDomain = "server.self-serve.windsurf.com"
	UpstreamIP   = "34.49.14.144"
	UpstreamHost = "server.self-serve.windsurf.com"

	defaultProxyPort = 443
	// 允许两次重放: 切号重试 + 可能的 Invalid Cascade session 剥离 conv_id 重试
	defaultReplayBudget  = 2
	jwtRefreshMinutes    = 4
	maxConsecErrors      = 1
	keyCooldownSec       = 600
	rateLimitCooldownSec = 120
	recentEventLimit     = 12
	streamQuotaWindow    = 4096
)

// PoolKeyState tracks the runtime state of each pool key.
type PoolKeyState struct {
	APIKey           string
	Plan             string // "Pro", "Trial", "Free", "Team", etc. 空串=未知
	JWT              []byte
	JWTUpdatedAt     time.Time
	Healthy          bool
	Disabled         bool
	RuntimeExhausted bool
	CooldownUntil    time.Time
	ConsecutiveErrs  int
	RequestCount     int
	SuccessCount     int
	TotalExhausted   int
	// ★ Per-key 稳定设备/session 指纹 — 每个 key 拥有独立的 session ID 和设备指纹
	// 服务端通过 F32(session UUID) 做 session 级限速，必须每 key 独立
	SessionID  string // F32: 稳定 UUID v4，创建时生成
	DeviceHash string // F31/F27: 稳定 hex hash，创建时生成
}

type jwtFetchCall struct {
	done chan struct{}
	jwt  []byte
	err  error
}

func newPoolKeyState(apiKey string) *PoolKeyState {
	return &PoolKeyState{
		APIKey:     apiKey,
		Healthy:    true,
		SessionID:  generateStableUUID(),
		DeviceHash: generateStableHexHash(),
	}
}

func (s *PoolKeyState) markExhausted() {
	s.Healthy = false
	s.RuntimeExhausted = true
	s.CooldownUntil = time.Now().Add(keyCooldownSec * time.Second)
	s.ConsecutiveErrs = 0
	s.TotalExhausted++
}

func (s *PoolKeyState) markRateLimited(detail string) {
	cooldown := time.Duration(rateLimitCooldownSec) * time.Second
	// 服务端有时返回 "Resets in: 16m0s" / "Resets in: 1h2m" → 用真实时长
	// 钳位 60s..30min，避免极端值
	if d := parseResetsIn(detail); d > 0 {
		if d < 60*time.Second {
			d = 60 * time.Second
		}
		if d > 30*time.Minute {
			d = 30 * time.Minute
		}
		cooldown = d
	}
	s.Healthy = false
	s.Disabled = false
	s.RuntimeExhausted = false
	s.CooldownUntil = time.Now().Add(cooldown)
	s.ConsecutiveErrs = 0
}

// parseResetsIn 从错误详情解析 "Resets in: 1h2m3s"（h/m/s 任意组合）。
var resetsInRE = regexp.MustCompile(`(?i)resets in:?\s*(?:(\d+)\s*h)?\s*(?:(\d+)\s*m)?\s*(?:(\d+)\s*s)?`)

func parseResetsIn(detail string) time.Duration {
	if detail == "" {
		return 0
	}
	m := resetsInRE.FindStringSubmatch(detail)
	if len(m) < 4 || (m[1] == "" && m[2] == "" && m[3] == "") {
		return 0
	}
	var dur time.Duration
	if m[1] != "" {
		if v, err := strconv.Atoi(m[1]); err == nil {
			dur += time.Duration(v) * time.Hour
		}
	}
	if m[2] != "" {
		if v, err := strconv.Atoi(m[2]); err == nil {
			dur += time.Duration(v) * time.Minute
		}
	}
	if m[3] != "" {
		if v, err := strconv.Atoi(m[3]); err == nil {
			dur += time.Duration(v) * time.Second
		}
	}
	return dur
}

func (s *PoolKeyState) markDisabled() {
	s.Healthy = false
	s.Disabled = true
	s.RuntimeExhausted = false
	s.CooldownUntil = time.Time{}
	s.ConsecutiveErrs = 0
}

func (s *PoolKeyState) isAvailable() bool {
	if s.Disabled {
		return false
	}
	if s.Healthy {
		return true
	}
	// RuntimeExhausted 的 key 不靠冷却自动恢复；只有 recordSuccess / ClearKeyExhausted 能解除。
	// 这确保额度真正耗尽的 key 不会 10 分钟后被回收重试。
	if s.RuntimeExhausted {
		return false
	}
	// 非额度耗尽的瞬态错误冷却：到期后视为可用。
	// ★ 纯只读判断 —— 不在这里写 s.Healthy/s.ConsecutiveErrs:本函数会在仅持
	//   p.mu.RLock() 的选号路径(pickStickyKey / leastConnections* / pickPoolKeyForSession)
	//   被并发调用,写共享字段是数据竞争。字段的真正复位交给写锁路径下的 recordSuccess。
	return time.Now().After(s.CooldownUntil)
}

// clearExhausted 解除「额度耗尽」锁定。仅清除 RuntimeExhausted + 冷却 + 健康标志，
// 不影响 Disabled（用户主动停用的号不会被这里清除）。
// 用于「自动切下一席」关闭后批量解锁、或用户手动从 UI 解锁单个号。
func (s *PoolKeyState) clearExhausted() {
	if !s.RuntimeExhausted {
		return
	}
	s.RuntimeExhausted = false
	s.CooldownUntil = time.Time{}
	s.ConsecutiveErrs = 0
	if !s.Disabled {
		s.Healthy = true
	}
}

func (s *PoolKeyState) recordSuccess() {
	s.RequestCount++
	s.SuccessCount++
	s.Disabled = false
	s.RuntimeExhausted = false
	s.ConsecutiveErrs = 0
}

// keyFingerprint returns the per-key stable fingerprint for identity replacement.
// Caller should hold p.mu.RLock() or p.mu.Lock().
func (p *MitmProxy) keyFingerprint(apiKey string) *KeyFingerprint {
	state := p.keyStates[apiKey]
	if state == nil || (state.SessionID == "" && state.DeviceHash == "") {
		return nil
	}
	return &KeyFingerprint{
		SessionID:  state.SessionID,
		DeviceHash: state.DeviceHash,
	}
}

// RecordKeySuccess 外部（如 Relay）通知号池某个 key 请求成功
func (p *MitmProxy) RecordKeySuccess(apiKey string) {
	p.mu.Lock()
	if state := p.keyStates[apiKey]; state != nil {
		state.recordSuccess()
	}
	p.mu.Unlock()
}

func (s *PoolKeyState) recordError() bool {
	s.RequestCount++
	s.ConsecutiveErrs++
	return s.ConsecutiveErrs >= maxConsecErrors
}

// MitmProxy is the core MITM reverse proxy that handles identity replacement.
type MitmProxy struct {
	mu       sync.RWMutex
	listener net.Listener
	running  bool
	port     int
	proxyURL string // 出站代理 (如 http://127.0.0.1:7890)

	poolKeys   []string // ordered list of api keys
	keyStates  map[string]*PoolKeyState
	currentIdx int
	jwtLock    sync.RWMutex
	jwtFetchMu sync.Mutex
	jwtFetches map[string]*jwtFetchCall

	windsurfSvc         *WindsurfService            // for JWT refresh
	logFn               func(string)                // log callback for UI
	onKeyExhausted      func(apiKey string)         // 额度耗尽回调（App 层刷新额度+同步号池）
	onKeyAccessDenied   func(apiKey, detail string) // 权限拒绝回调（App 层持久化降权/禁用）
	onCurrentKeyChanged func(apiKey, reason string) // 当前 key 变化回调（App 层同步本地会话）
	eventsMu            sync.RWMutex
	recentEvents        []MitmProxyEvent

	jwtReady chan struct{} // closed when at least one JWT is available
	jwtOnce  sync.Once
	stopCh   chan struct{}

	// bgWg 跟踪 fire-and-forget 后台 goroutine（如认证失败后的 JWT 异步刷新）。
	// Stop() 不强等（HTTP refresh 自带 30s timeout，等不值），但测试可调
	// WaitBackgroundForTests() 确保 goroutine 收尾后再断言，避免 race detector
	// 报 "Test/goroutine teardown" 误警。
	bgWg sync.WaitGroup

	lastErrorKind    string
	lastErrorSummary string
	lastErrorAt      string
	lastErrorKey     string

	debugDump   bool // 开启后 dump GetChatMessage 请求/响应的 protobuf 字段树
	fullCapture bool // 全量抓包：记录所有请求/响应到 JSONL + body 文件

	forgeConfig       ForgeConfig
	staticCacheConfig StaticCacheConfig
	jailbreakConfig   JailbreakConfig // chat system prompt 末尾注入「破限」覆盖文本
	jailbreakStats    JailbreakStats  // 注入计数 / 上次时间，供 UI 状态面板

	// F7-REMOVAL: 本行字段删除。同步去掉 SetSmartFriendEnabled / GetSmartFriendEnabled
	// (定义在 proxy_smartfriend.go) 以及下面 markRuntimeExhaustedAndRotate / handleRequest 两处分支。
	smartFriendEnabled bool // F7 patch: CASCADE(5) → SMART_FRIEND(13)

	// autoSwitchOnQuotaExhausted 镜像 settings.AutoSwitchOnQuotaExhausted。
	// 关闭时 MITM 收到上游 quota_exhausted 错误：
	//   - 不调用 markExhausted（不锁号、不进冷却、不打 RuntimeExhausted 标志）
	//   - 不调用 rotateKey（保持当前 key）
	//   - 错误透传给 IDE，由 IDE 自行处理（弹 quota 提示）
	// 默认 true（保持现有行为）。app_settings.syncAutoSwitchOnQuotaExhausted 同步。
	autoSwitchOnQuotaExhausted bool

	usageTracker *UsageTracker

	// ── 阶段 2 提供商路由(可选注入) ──
	// router 非 nil 且 RouteMode()=="providers" 时, chat path 走
	// 提供商上游(cascade 翻译 → OpenAI/Anthropic/Gemini → 翻回 cascade frame)。
	// 未注入或胶囊处于 pool 模式时永远走号池(向后兼容)。
	router Router
	// transportPool 全局出站 transport 池(可选注入)。provider Route 用它走代理。
	transportPool *TransportPool

	// ── Session binding (per-conversation sticky routing) ──
	sessionsMu sync.RWMutex
	sessionMap map[string]*SessionBinding // conversation_id → binding

	// ── 新对话首条消息追踪 ──
	// 首条消息(无 convID)使用的 pool key 会推入此队列；
	// 当第二条消息(有 convID)到达且 convID 未绑定时，从队列弹出匹配的 key。
	pendingNewConvMu   sync.Mutex
	pendingNewConvKeys []pendingKeyEntry

	// ── 全局 Trial 限速退避 ──
	// 检测到 "global rate limit for trial users" 时设置退避截止时间，
	// 退避期间 key 选择自动跳过 Trial/Free key，优先使用 Pro/Team key。
	globalTrialRateLimitUntil time.Time

	// ── 限速轮转 debounce ──
	// 上一次因 rate limit 触发轮转的时间；短时间内并发命中限速只切一次号，
	// 避免连锁烧 key。受 mu 保护。
	lastRateLimitRotateAt time.Time

	// ── ManualPin 粘性 ──
	// 当用户手动切到某账号 + 启用 ManualPin 时，App 层把该账号的 apiKey 推到
	// 这里。pickKeyForNewConversation / pickPoolKeyForSession 优先用此 key，
	// 而不是 leastConnectionsKey 的负载均衡分配。空字符串 = 关闭粘性。
	// 设计动机：用户「手动切到 X = 接下来的新对话也用 X」是明确意图，
	// 旧实现按 leastConnections 把新对话分散给"会话最少的"号，与意图冲突。
	stickyKey string

	// ── Clash IP 轮换钩子 ──
	// inFlightChatStreams: 当前进行中的 chat-path 请求数（atomic），
	// ClashRotator 据此判断是否空闲以避免切换瞬间断流。
	inFlightChatStreams int64
	// onUpstreamRateLimit: 检测到上游 rate-limit / global rate-limit 时的回调。
	// App 层用来通知 ClashRotator 立即换 IP。
	onUpstreamRateLimit func(detail string)
	// upstreamBase: 出站 transport（持有以便 CloseIdleConnections 后强制重建连接）
	upstreamBase *http.Transport
}

var injectCodeiumConfigFn = InjectCodeiumConfig
var getJWTByAPIKeyFn = func(s *WindsurfService, apiKey string) (string, error) {
	return s.GetJWTByAPIKey(apiKey)
}

const (
	MitmCurrentKeyChangeReasonQuotaRotate       = "quota_rotate"
	MitmCurrentKeyChangeReasonRateLimitRotate   = "rate_limit_rotate"
	MitmCurrentKeyChangeReasonAuthRotate        = "auth_rotate"
	MitmCurrentKeyChangeReasonPoolSync          = "pool_sync"
	MitmCurrentKeyChangeReasonUnavailableRotate = "unavailable_rotate"
	MitmCurrentKeyChangeReasonJWTFallback       = "jwt_fallback"
	MitmCurrentKeyChangeReasonManualSwitch      = "manual_switch"
	MitmCurrentKeyChangeReasonManualNext        = "manual_next"
)

// MitmProxyStatus is exposed to the frontend.
type MitmProxyStatus struct {
	Running          bool                 `json:"running"`
	Port             int                  `json:"port"`
	HostsMapped      bool                 `json:"hosts_mapped"`
	CAInstalled      bool                 `json:"ca_installed"`
	CurrentKey       string               `json:"current_key"`
	PoolStatus       []PoolKeyInfo        `json:"pool_status"`
	TotalReqs        int                  `json:"total_requests"`
	ActiveSessions   []SessionBindingInfo `json:"active_sessions"`
	SessionCount     int                  `json:"session_count"`
	LastErrorKind    string               `json:"last_error_kind"`
	LastErrorSummary string               `json:"last_error_summary"`
	LastErrorAt      string               `json:"last_error_at"`
	LastErrorKey     string               `json:"last_error_key"`
	RecentEvents     []MitmProxyEvent     `json:"recent_events"`
}

type PoolKeyInfo struct {
	KeyShort string `json:"key_short"`
	// KeyHash 是 full api_key 的稳定唯一指纹（sha256 前 12 个 hex 字符）。
	// 用于 App 层 / 前端把 PoolKeyInfo 精确对回 Account；之前用 KeyShort
	// 做前缀匹配，对 devin-session-token$<JWT> 这种共享前缀的 key 完全失效，
	// 导致所有账号被误标"当前活跃"。永远不要把 KeyShort 当唯一 ID 用。
	KeyHash           string `json:"key_hash"`
	Plan              string `json:"plan"`
	Healthy           bool   `json:"healthy"`
	Disabled          bool   `json:"disabled"`
	RuntimeExhausted  bool   `json:"runtime_exhausted"`
	CooldownUntil     string `json:"cooldown_until"`
	HasJWT            bool   `json:"has_jwt"`
	RequestCount      int    `json:"request_count"`
	SuccessCount      int    `json:"success_count"`
	TotalExhausted    int    `json:"total_exhausted"`
	IsCurrent         bool   `json:"is_current"`
	BoundSessionCount int    `json:"bound_session_count"`
	// App 层填充（MitmProxy 本身不知道账号信息）
	Email    string `json:"email,omitempty"`
	Nickname string `json:"nickname,omitempty"`
}

// HashPoolKey 返回 api_key 的稳定唯一指纹（sha256 前 12 hex），供 App / 前端
// 精确匹配 PoolKeyInfo ↔ Account 使用。devin-session-token$<JWT> 这类账号
// 用 KeyShort 前缀匹配会全部撞车，这个 hash 不会。
func HashPoolKey(apiKey string) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(apiKey)))
	return hex.EncodeToString(h[:6]) // 12 hex chars
}

type MitmProxyEvent struct {
	At      string `json:"at"`
	Message string `json:"message"`
	Tone    string `json:"tone"`
}

// ── Session binding (per-conversation sticky routing) ──

const (
	sessionMapMaxEntries  = 500
	sessionExpireMinutes  = 30
	pendingNewConvMaxAge  = 60 * time.Second // 首条消息 pending key 最长保留时间
	pendingNewConvMaxSize = 20               // pending 队列上限
)

// pendingKeyEntry 记录首条消息(无 convID)选用的 pool key，用于与后续带 convID 的第二条消息匹配。
type pendingKeyEntry struct {
	PoolKey string
	At      time.Time
}

// SessionBinding maps a conversation (by conversation_id) to a specific pool key.
type SessionBinding struct {
	ConversationID string
	PoolKey        string
	BoundAt        time.Time
	LastSeenAt     time.Time
	RequestCount   int
	Migrated       bool   // key 变更后需要主动剥离 conv_id
	Title          string // 从请求中提取的会话标题摘要（最后一条 user 消息片段）
}

// SessionBindingInfo is the frontend-safe DTO for session bindings.
type SessionBindingInfo struct {
	ConvID      string `json:"conv_id"`
	ConvIDShort string `json:"conv_id_short"`
	// PoolKeyShort 是前 16 字符 + "..."，仅用于显示。
	// 长 token 类账号(devin-session-token$<JWT>) 共享前缀会让多个账号截到相同
	// 字符串 —— 不要用它做唯一匹配，会跨账号撞车。
	PoolKeyShort string `json:"pool_key_short"`
	// PoolKeyHash 是 sha256 前 12 hex，全局唯一指纹。前端用此过滤"哪些会话归属
	// 当前 active key"，避免 PoolKeyShort 同前缀撞车把别账号会话误算。
	PoolKeyHash  string `json:"pool_key_hash"`
	BoundAt      string `json:"bound_at"`
	LastSeenAt   string `json:"last_seen_at"`
	RequestCount int    `json:"request_count"`
	Title        string `json:"title"`
}

type quotaStreamWatchBody struct {
	inner      io.ReadCloser
	onQuota    func(detail string)
	onSuccess  func(completionTokens int)
	recentText string
	sawQuota   bool
	finalized  bool

	// gRPC parser stream states
	grpcBuf          []byte
	completionTokens int
}

// NewMitmProxy creates a new proxy instance.
func NewMitmProxy(windsurfSvc *WindsurfService, logFn func(string), proxyURL string, usageTracker *UsageTracker) *MitmProxy {
	return &MitmProxy{
		port:                       defaultProxyPort,
		keyStates:                  make(map[string]*PoolKeyState),
		windsurfSvc:                windsurfSvc,
		logFn:                      logFn,
		proxyURL:                   proxyURL,
		jwtReady:                   make(chan struct{}),
		jwtFetches:                 make(map[string]*jwtFetchCall),
		stopCh:                     make(chan struct{}),
		sessionMap:                 make(map[string]*SessionBinding),
		usageTracker:               usageTracker,
		autoSwitchOnQuotaExhausted: true, // 默认开启，保持现有行为
	}
}

func (p *MitmProxy) syncCurrentAPIKeyToClient(apiKey, reason string) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return
	}
	// ★ 不再注入 codeium config — MITM 按 conversation_id 路由，IDE 保持原始 Pro key 身份
	p.log("号池活跃 key 切换: %s... (reason=%s)", apiKey[:minStr(12, len(apiKey))], reason)
	p.mu.RLock()
	cb := p.onCurrentKeyChanged
	p.mu.RUnlock()
	if cb != nil {
		go cb(apiKey, strings.TrimSpace(reason))
	}
}

// SetOnKeyExhausted 设置额度耗尽回调（App 层用来触发额度刷新 + 同步号池）
func (p *MitmProxy) SetOnKeyExhausted(fn func(apiKey string)) {
	p.mu.Lock()
	p.onKeyExhausted = fn
	p.mu.Unlock()
}

// SetOnKeyAccessDenied 设置权限拒绝回调（App 层用来持久化账号降权/禁用并重同步号池）
func (p *MitmProxy) SetOnKeyAccessDenied(fn func(apiKey, detail string)) {
	p.mu.Lock()
	p.onKeyAccessDenied = fn
	p.mu.Unlock()
}

// SetOnCurrentKeyChanged 设置当前 key 变化回调（App 层用来同步本地登录态）。
func (p *MitmProxy) SetOnCurrentKeyChanged(fn func(apiKey, reason string)) {
	p.mu.Lock()
	p.onCurrentKeyChanged = fn
	p.mu.Unlock()
}

// SetOnUpstreamRateLimit 设置上游 rate-limit 回调（App 层用来通知 ClashRotator 立即换 IP）。
func (p *MitmProxy) SetOnUpstreamRateLimit(fn func(detail string)) {
	p.mu.Lock()
	p.onUpstreamRateLimit = fn
	p.mu.Unlock()
}

// SetStickyKey 设置 ManualPin 粘性 key —— 启用后所有新对话强制走该 key，
// 不再按 leastConnections 在号池间分散。空字符串清除粘性。
//
// 由 App 层 setManualPin / UnpinManualAccount / 启动时根据 settings 推送。
// 与 SwitchToKey 分工：
//   - SwitchToKey 改 currentIdx（影响"当前活跃"显示 + auto-rotate 起点）
//   - SetStickyKey 改 stickyKey（影响新对话路由 → 不会被 leastConnections 分散）
func (p *MitmProxy) SetStickyKey(apiKey string) {
	p.mu.Lock()
	p.stickyKey = strings.TrimSpace(apiKey)
	p.mu.Unlock()
	if p.stickyKey != "" {
		p.log("ManualPin 粘性已启用: 新对话强制走 %s...", p.stickyKey[:minStr(12, len(p.stickyKey))])
	} else {
		p.log("ManualPin 粘性已清除: 新对话恢复 leastConnections 分散")
	}
}

// pickStickyKey 内部使用：如果启用了 stickyKey 且该 key 健康可用，返回 (key, jwt)。
// 不可用（额度耗尽/无 JWT/不在号池）则返回 ("", nil)，调用方继续 fallback 流程。
//
// Caller 不持有 p.mu / p.sessionsMu / p.pendingNewConvMu。
func (p *MitmProxy) pickStickyKey() (string, []byte) {
	p.mu.RLock()
	sticky := p.stickyKey
	state := p.keyStates[sticky]
	available := sticky != "" && state != nil && state.isAvailable()
	p.mu.RUnlock()
	if sticky == "" || !available {
		return "", nil
	}
	jwt := p.usableJWTForKey(sticky)
	if len(jwt) == 0 {
		return "", nil
	}
	return sticky, jwt
}

// InFlightChatStreams 返回当前进行中的 chat-path 请求数（用于 Clash 切换前空闲判定）。
func (p *MitmProxy) InFlightChatStreams() int64 {
	return atomic.LoadInt64(&p.inFlightChatStreams)
}

// CloseUpstreamIdleConnections 关闭出站 transport 的空闲连接池。
// Clash 切换节点后调用，强制下一请求重建连接走新 IP（避免 HTTP/2 复用旧链路）。
func (p *MitmProxy) CloseUpstreamIdleConnections() {
	p.mu.RLock()
	t := p.upstreamBase
	p.mu.RUnlock()
	if t != nil {
		t.CloseIdleConnections()
	}
}

func (p *MitmProxy) markRuntimeExhaustedAndRotate(usedKey, detail string) string {
	p.log("★ markRuntimeExhaustedAndRotate: key=%s... detail=%s", usedKey[:minStr(12, len(usedKey))], detail)
	rotatedKey := ""
	p.mu.Lock()
	// F7-REMOVAL: 整个 if p.smartFriendEnabled 分支删除
	if p.smartFriendEnabled {
		p.mu.Unlock()
		p.recordUpstreamFailure(upstreamFailureQuota, "smartfriend-bypass="+detail, usedKey)
		p.log("★ SmartFriend 已开启，忽略额度耗尽判定: %s...", usedKey[:minStr(12, len(usedKey))])
		return ""
	}
	// ★ 用户在 Settings「额度耗尽时自动切下一席」关闭：不锁号、不切号、不打 RuntimeExhausted。
	//    错误依旧透传给 IDE 处理（IDE 会显示 quota exceeded）。
	//    仅记录失败计数用于统计 / 通知中心。
	if !p.autoSwitchOnQuotaExhausted {
		p.mu.Unlock()
		p.recordUpstreamFailure(upstreamFailureQuota, "auto-switch-off="+detail, usedKey)
		p.log("★ 自动切下一席已关闭，忽略额度耗尽锁定: %s... (透传给IDE)", usedKey[:minStr(12, len(usedKey))])
		return ""
	}
	if state := p.keyStates[usedKey]; state != nil {
		state.markExhausted()
	}
	rotatedKey = p.rotateKey()
	cb := p.onKeyExhausted
	poolSize := len(p.poolKeys)
	p.mu.Unlock()
	p.recordUpstreamFailure(upstreamFailureQuota, detail, usedKey)
	if rotatedKey != "" {
		p.log("★ 额度耗尽轮转: %s... → %s... (pool=%d)", usedKey[:minStr(12, len(usedKey))], rotatedKey[:minStr(12, len(rotatedKey))], poolSize)
		p.syncCurrentAPIKeyToClient(rotatedKey, MitmCurrentKeyChangeReasonQuotaRotate)
	} else {
		p.log("★ 额度耗尽但无可轮转 key (pool=%d)", poolSize)
	}
	// ★ 不迁移已有会话：迁移后新号没有旧 conversation 的 session，必然 Invalid Cascade session
	// 已有对话保持粘性，新对话自然会分配到健康 key
	// 异步触发 App 层刷新耗尽 key 的额度 → 更新 store → syncMitmPoolKeys 移除
	if cb != nil {
		p.log("★ 触发 onKeyExhausted 回调: key=%s...", usedKey[:minStr(12, len(usedKey))])
		go cb(usedKey)
	}
	return rotatedKey
}

func (p *MitmProxy) rotateAfterAuthFailure(usedKey, detail string) string {
	detail = strings.TrimSpace(detail)
	// 永久权限拒绝（如号被封）→ 仍然禁用 + 切号
	if isPersistentJWTAccessDeniedDetail(detail) {
		return p.disableKeyAndRotate(usedKey, detail)
	}
	// 普通认证失败 → 不切号，清 JWT + 后台异步刷新
	p.recordUpstreamFailure(upstreamFailureAuth, detail, usedKey)
	p.clearJWT(usedKey)

	p.log("★ 认证失败(不切号): %s...，后台异步刷新 JWT", usedKey[:minStr(12, len(usedKey))])
	if usedKey != "" && p.windsurfSvc != nil && p.windsurfSvc.client != nil {
		// 启动前快速检测 proxy 是否已 Stop —— 测试 / shutdown 时不再发起新刷新。
		p.mu.RLock()
		stopCh := p.stopCh
		p.mu.RUnlock()
		select {
		case <-stopCh:
			return "" // 已停，不启动
		default:
		}
		p.bgWg.Add(1)
		go func(key string, sc chan struct{}) {
			defer p.bgWg.Done()
			// goroutine 启动期间可能被 Stop —— 再 check 一次。
			select {
			case <-sc:
				return
			default:
			}
			refreshed := p.refreshJWTForKey(key)
			// HTTP 完成后可能 proxy 已 Stop。日志路径仍是 thread-safe 的，
			// 但 stopped 后日志泄漏到下次启动会混淆，跳过更干净。
			select {
			case <-sc:
				return
			default:
			}
			if len(refreshed) > 0 {
				p.log("认证失败后后台刷新 JWT 成功: %s...", key[:minStr(12, len(key))])
			} else {
				p.log("认证失败后后台刷新 JWT 失败: %s...", key[:minStr(12, len(key))])
			}
		}(usedKey, stopCh)
	}
	return "" // 不切号
}

// WaitBackgroundForTests 阻塞等待所有 fire-and-forget 后台 goroutine 退出。
// 仅供测试使用 —— 生产 Stop 路径不调用，因为 HTTP refresh 自带 30s timeout
// 等不值。Test cleanup 用此函数避免 -race 误报。
func (p *MitmProxy) WaitBackgroundForTests() {
	p.bgWg.Wait()
}

func (p *MitmProxy) disableKeyAndRotate(usedKey, detail string) string {
	p.markKeyDisabled(usedKey, detail)

	p.mu.Lock()
	rotatedKey := p.rotateKey()
	poolSize := len(p.poolKeys)
	p.mu.Unlock()

	rotatedKey = strings.TrimSpace(rotatedKey)
	if rotatedKey != "" && rotatedKey != strings.TrimSpace(usedKey) {
		p.log("★ 权限拒绝轮转: %s... → %s... (pool=%d)", usedKey[:minStr(12, len(usedKey))], rotatedKey[:minStr(12, len(rotatedKey))], poolSize)
		p.syncCurrentAPIKeyToClient(rotatedKey, MitmCurrentKeyChangeReasonAuthRotate)
		return rotatedKey
	}
	if poolSize <= 1 {
		p.log("权限拒绝但号池无备用 key: %s...", usedKey[:minStr(12, len(usedKey))])
	} else {
		p.log("权限拒绝已禁用当前 key，但没有可立即轮转的备用 key: %s...", usedKey[:minStr(12, len(usedKey))])
	}
	return ""
}

// rotateDebounceWindow 限速/轮转 debounce 窗口：短时间内多次命中限速只切一次号，
// 避免并发请求同时返回 rate limit 时连锁烧 key。
const rotateDebounceWindow = 3 * time.Second

func (p *MitmProxy) markRateLimitedAndRotate(usedKey, detail string) string {
	p.recordUpstreamFailure(upstreamFailureRateLimit, detail, usedKey)
	// ★ 限速：短冷却 + 切号重试（限速是 per-key 的，换号能绕过）
	p.mu.Lock()
	if state := p.keyStates[usedKey]; state != nil {
		state.markRateLimited(detail)
	}
	// ★ Debounce：短时间内已轮转过则不再轮转（pool currentIdx 不再震荡）
	skipRotate := time.Since(p.lastRateLimitRotateAt) < rotateDebounceWindow
	var rotatedKey string
	if !skipRotate {
		rotatedKey = p.rotateKey()
		p.lastRateLimitRotateAt = time.Now()
	}
	poolSize := len(p.poolKeys)
	p.mu.Unlock()
	if rotatedKey != "" && rotatedKey != strings.TrimSpace(usedKey) {
		p.log("★ 限速轮转: %s... → %s... (pool=%d)",
			usedKey[:minStr(12, len(usedKey))],
			rotatedKey[:minStr(12, len(rotatedKey))],
			poolSize)
		p.syncCurrentAPIKeyToClient(rotatedKey, MitmCurrentKeyChangeReasonRateLimitRotate)
		// ★ 不迁移已有会话：保持会话粘性，避免 Invalid Cascade session
		return rotatedKey
	}
	if skipRotate {
		p.log("★ 限速 debounce 跳过轮转 (key 已冷却): %s...", usedKey[:minStr(12, len(usedKey))])
	} else {
		p.log("★ 限速但无可轮转 key，透传给 IDE (pool=%d): %s...", poolSize, usedKey[:minStr(12, len(usedKey))])
	}
	return ""
}

func (p *MitmProxy) markKeyDisabled(apiKey, detail string) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return
	}
	p.clearJWT(apiKey)
	p.mu.Lock()
	if state := p.keyStates[apiKey]; state != nil {
		state.markDisabled()
	}
	cb := p.onKeyAccessDenied
	p.mu.Unlock()
	p.recordUpstreamFailure(upstreamFailurePermission, detail, apiKey)
	p.log("★ JWT 权限拒绝，已禁用 key: %s... (%s)", apiKey[:minStr(12, len(apiKey))], truncate(detail, 140))
	if cb != nil {
		go cb(apiKey, detail)
	}
}

func newQuotaStreamWatchBody(inner io.ReadCloser, onQuota func(detail string), onSuccess func(int)) *quotaStreamWatchBody {
	return &quotaStreamWatchBody{
		inner:     inner,
		onQuota:   onQuota,
		onSuccess: onSuccess,
	}
}

func (b *quotaStreamWatchBody) Read(p []byte) (int, error) {
	n, err := b.inner.Read(p)
	if n > 0 {
		b.scanChunk(p[:n])
	}
	if err == io.EOF {
		b.finalize()
	}
	return n, err
}

func (b *quotaStreamWatchBody) Close() error {
	err := b.inner.Close()
	b.finalize()
	return err
}

func (b *quotaStreamWatchBody) scanChunk(chunk []byte) {
	if len(chunk) == 0 {
		return
	}

	// ── 抓取 Tokens ──
	b.grpcBuf = append(b.grpcBuf, chunk...)
	for len(b.grpcBuf) >= 5 {
		flags := b.grpcBuf[0]
		payloadLen := int(binary.BigEndian.Uint32(b.grpcBuf[1:5]))
		if len(b.grpcBuf) < 5+payloadLen {
			break
		}
		payload := b.grpcBuf[5 : 5+payloadLen]
		b.grpcBuf = b.grpcBuf[5+payloadLen:]

		if flags&2 != 0 {
			continue // skip EOS
		}
		decoded, err := decodeStreamEnvelopePayload(flags, payload)
		if err == nil && len(decoded) > 0 {
			chunkResponse, _, err := ParseChatResponseChunk(decoded)
			if err == nil && len(chunkResponse) > 0 {
				b.completionTokens += estimateTokens(chunkResponse)
			}
		}
	}

	if b.sawQuota {
		return
	}

	lower := strings.ToLower(string(chunk))
	combined := b.recentText + lower
	if len(combined) > streamQuotaWindow {
		combined = combined[len(combined)-streamQuotaWindow:]
	}
	b.recentText = combined
	// 诊断：流式 chunk 中 precondition/quota/exhaust 关键词出现时记录
	if strings.Contains(lower, "precondition") || strings.Contains(lower, "exhaust") || strings.Contains(lower, "quota") {
		trafficLog("  STREAM-SCAN hit: chunk[%d] matched keyword, combined[%d]", len(chunk), len(combined))
	}
	if !isQuotaExhaustedText(combined) {
		return
	}
	b.sawQuota = true
	if b.onQuota != nil {
		b.onQuota("stream-body=" + truncate(strings.TrimSpace(combined), 180))
	}
}

func (b *quotaStreamWatchBody) finalize() {
	if b.finalized {
		return
	}
	b.finalized = true
	if b.sawQuota || b.onSuccess == nil {
		return
	}
	b.onSuccess(b.completionTokens)
}

func (p *MitmProxy) log(format string, args ...interface{}) {
	msg := fmt.Sprintf("[MITM] "+format, args...)
	p.appendRecentEvent(msg)
	utils.DLog("%s", msg) // 同时写 stdout + debug.log（DLog 内部已调用 log.Print）
	if p.logFn != nil {
		p.logFn(msg)
	}
}

func classifyMitmEventTone(message string) string {
	text := strings.ToLower(strings.TrimSpace(message))
	switch {
	case strings.Contains(text, "失败"), strings.Contains(text, "错误"), strings.Contains(text, "异常退出"):
		return "danger"
	case strings.Contains(text, "⚠️"), strings.Contains(text, "耗尽"), strings.Contains(text, "跳过"), strings.Contains(text, "超时"):
		return "warning"
	case strings.Contains(text, "✅"), strings.Contains(text, "成功"), strings.Contains(text, "启动"), strings.Contains(text, "已停止"):
		return "success"
	default:
		return "info"
	}
}

func (p *MitmProxy) appendRecentEvent(message string) {
	event := MitmProxyEvent{
		At:      time.Now().Format(time.RFC3339),
		Message: strings.TrimSpace(message),
		Tone:    classifyMitmEventTone(message),
	}
	p.eventsMu.Lock()
	defer p.eventsMu.Unlock()
	p.recentEvents = append(p.recentEvents, event)
	if len(p.recentEvents) > recentEventLimit {
		p.recentEvents = append([]MitmProxyEvent(nil), p.recentEvents[len(p.recentEvents)-recentEventLimit:]...)
	}
}

func (p *MitmProxy) recentEventsSnapshot() []MitmProxyEvent {
	p.eventsMu.RLock()
	defer p.eventsMu.RUnlock()
	if len(p.recentEvents) == 0 {
		return nil
	}
	out := make([]MitmProxyEvent, 0, len(p.recentEvents))
	for i := len(p.recentEvents) - 1; i >= 0; i-- {
		out = append(out, p.recentEvents[i])
	}
	return out
}

func (p *MitmProxy) SetWindsurfService(windsurfSvc *WindsurfService) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.windsurfSvc = windsurfSvc
}

// SetRouter 注入提供商路由源。
// 注入后, 只要 router.RouteMode()=="providers" 且请求路径是 chat path,
// MITM serve() handler 会绕过号池 reverse proxy, 走 Route
// 把 cascade 请求翻译给 OpenAI/Anthropic/Gemini 上游。
// router=nil = 关闭(永远走号池)。
func (p *MitmProxy) SetRouter(router Router) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.router = router
}

// SetTransportPool 注入全局 transport 池。provider Route 用它走代理出站。
func (p *MitmProxy) SetTransportPool(pool *TransportPool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.transportPool = pool
}

func (p *MitmProxy) SetUpstreamProxy(proxyURL string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.proxyURL = strings.TrimSpace(proxyURL)
}

// SetAutoSwitchOnQuotaExhausted 同步用户「额度耗尽时自动切下一席」开关到 MITM proxy。
// 关闭后：MITM 收到上游 quota_exhausted 不再锁号 / 切号，直接透传错误给 IDE。
//
// ★ 副作用：开关从开 → 关时，自动批量解锁所有已 RuntimeExhausted 的号。
// 用户预期一关开关就立刻见效，不必再点「批量解锁」按钮。
func (p *MitmProxy) SetAutoSwitchOnQuotaExhausted(enabled bool) {
	p.mu.Lock()
	prev := p.autoSwitchOnQuotaExhausted
	p.autoSwitchOnQuotaExhausted = enabled
	autoUnlock := prev && !enabled
	cleared := 0
	if autoUnlock {
		for key, state := range p.keyStates {
			if state == nil || !state.RuntimeExhausted {
				continue
			}
			state.clearExhausted()
			cleared++
			p.log("★ 自动解锁(开关关闭): %s...", key[:minStr(12, len(key))])
		}
	}
	p.mu.Unlock()
	if cleared > 0 {
		p.log("★ SetAutoSwitchOnQuotaExhausted=false → 自动解锁 %d 个号", cleared)
	}
}

// GetAutoSwitchOnQuotaExhausted 返回当前 MITM「额度耗尽自动切」开关状态。
func (p *MitmProxy) GetAutoSwitchOnQuotaExhausted() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.autoSwitchOnQuotaExhausted
}

// ClearKeyExhausted 解除单个 key 的「额度耗尽」锁定（手动解锁入口）。
// 返回是否真正解锁了某个号。
func (p *MitmProxy) ClearKeyExhausted(apiKey string) bool {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	state, ok := p.keyStates[apiKey]
	if !ok || state == nil {
		return false
	}
	if !state.RuntimeExhausted {
		return false
	}
	state.clearExhausted()
	p.log("★ 手动解锁额度耗尽: %s...", apiKey[:minStr(12, len(apiKey))])
	return true
}

// ClearAllExhausted 批量解除号池中所有 RuntimeExhausted 锁定。
// 返回解锁的号数量。
func (p *MitmProxy) ClearAllExhausted() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	cleared := 0
	for key, state := range p.keyStates {
		if state == nil || !state.RuntimeExhausted {
			continue
		}
		state.clearExhausted()
		cleared++
		p.log("★ 批量解锁: %s...", key[:minStr(12, len(key))])
	}
	if cleared > 0 {
		p.log("★ ClearAllExhausted: 共解锁 %d 个号", cleared)
	}
	return cleared
}

// SetDebugDump 开启/关闭 proto dump（GetChatMessage 请求/响应字段树写入文件）
func (p *MitmProxy) SetDebugDump(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.debugDump = enabled
}

// DebugDumpEnabled 返回当前 debug dump 状态
func (p *MitmProxy) DebugDumpEnabled() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.debugDump
}

func (p *MitmProxy) recordUpstreamFailure(kind upstreamFailureKind, detail, apiKey string) {
	if kind == upstreamFailureNone {
		return
	}
	p.mu.Lock()
	p.lastErrorKind = string(kind)
	p.lastErrorSummary = strings.TrimSpace(detail)
	p.lastErrorAt = time.Now().Format(time.RFC3339)
	if apiKey != "" {
		p.lastErrorKey = apiKey[:minStr(12, len(apiKey))]
	} else {
		p.lastErrorKey = ""
	}
	rlCb := p.onUpstreamRateLimit
	p.mu.Unlock()

	// 限速类错误同步通知 ClashRotator 立即换 IP（rotator 内部 debounce）
	if rlCb != nil && (kind == upstreamFailureRateLimit || kind == upstreamFailureGlobalRateLimit) {
		go rlCb(strings.TrimSpace(detail))
	}
}

// SetPoolKeys configures the account pool from API keys.
func (p *MitmProxy) SetPoolKeys(keys []string) {
	p.mu.Lock()
	currentKey := ""
	if len(p.poolKeys) > 0 && p.currentIdx >= 0 && p.currentIdx < len(p.poolKeys) {
		currentKey = p.poolKeys[p.currentIdx]
	}
	previousCurrentKey := currentKey

	p.poolKeys = keys
	for _, k := range keys {
		if _, ok := p.keyStates[k]; !ok {
			p.keyStates[k] = newPoolKeyState(k)
		}
	}
	// Remove stale keys
	for k := range p.keyStates {
		found := false
		for _, pk := range keys {
			if pk == k {
				found = true
				break
			}
		}
		if !found {
			delete(p.keyStates, k)
		}
	}

	if currentKey != "" {
		for i, k := range keys {
			if k == currentKey {
				p.currentIdx = i
				running := p.running
				p.mu.Unlock()
				if running {
					go p.prefetchJWTs()
				}
				return
			}
		}
	}
	if p.currentIdx < 0 || p.currentIdx >= len(keys) {
		p.currentIdx = 0
	}
	newCurrentKey := ""
	if len(keys) > 0 && p.currentIdx >= 0 && p.currentIdx < len(keys) {
		newCurrentKey = keys[p.currentIdx]
	}
	running := p.running
	p.mu.Unlock()
	if running {
		go p.prefetchJWTs()
	}
	if running && newCurrentKey != "" && newCurrentKey != previousCurrentKey {
		p.syncCurrentAPIKeyToClient(newCurrentKey, MitmCurrentKeyChangeReasonPoolSync)
	}
}

// PoolKeyInput carries an API key together with its plan type for pool configuration.
type PoolKeyInput struct {
	APIKey string
	Plan   string // "Pro", "Trial", "Free", "Team", etc.
}

// SetPoolKeysWithPlan configures the account pool from API keys with plan info.
func (p *MitmProxy) SetPoolKeysWithPlan(infos []PoolKeyInput) {
	keys := make([]string, 0, len(infos))
	planMap := make(map[string]string, len(infos))
	for _, info := range infos {
		keys = append(keys, info.APIKey)
		planMap[info.APIKey] = info.Plan
	}
	p.SetPoolKeys(keys)
	// 更新 plan 信息
	p.mu.Lock()
	for k, plan := range planMap {
		if state := p.keyStates[k]; state != nil {
			state.Plan = plan
		}
	}
	p.mu.Unlock()
}

// isTrialOrFreeKey returns true if the key's plan is Trial or Free (subject to global trial rate limit).
func (p *MitmProxy) isTrialOrFreeKey(apiKey string) bool {
	state := p.keyStates[apiKey]
	if state == nil {
		return true // unknown → treat as trial (conservative)
	}
	plan := strings.ToLower(state.Plan)
	return plan == "" || plan == "trial" || plan == "free" || plan == "未识别"
}

// isProOrTeamKey returns true if the key's plan is Pro, Team, Max/Ultimate, or Enterprise.
func (p *MitmProxy) isProOrTeamKey(apiKey string) bool {
	return !p.isTrialOrFreeKey(apiKey)
}

// isGlobalTrialRateLimitActive returns true if we're in global trial rate limit backoff.
func (p *MitmProxy) isGlobalTrialRateLimitActive() bool {
	return time.Now().Before(p.globalTrialRateLimitUntil)
}

const globalTrialRateLimitBackoffSec = 60

// markGlobalTrialRateLimit sets global trial rate limit backoff.
func (p *MitmProxy) markGlobalTrialRateLimit() {
	p.mu.Lock()
	p.globalTrialRateLimitUntil = time.Now().Add(globalTrialRateLimitBackoffSec * time.Second)
	p.mu.Unlock()
	p.log("★ 全局 Trial 限速退避已设置 (%ds)", globalTrialRateLimitBackoffSec)
}

// findProKey finds an available Pro/Team key in the pool, excluding excludeKey.
// Caller must hold p.mu.RLock().
func (p *MitmProxy) findProKey(excludeKey string) string {
	for _, k := range p.poolKeys {
		if k == excludeKey {
			continue
		}
		state := p.keyStates[k]
		if state == nil || !state.isAvailable() {
			continue
		}
		if p.isProOrTeamKey(k) {
			return k
		}
	}
	return ""
}

// rebindSession migrates a conversation's session binding to a new key.
func (p *MitmProxy) rebindSession(convID, newKey string) {
	p.sessionsMu.Lock()
	defer p.sessionsMu.Unlock()
	if binding, ok := p.sessionMap[convID]; ok {
		oldKey := binding.PoolKey
		binding.PoolKey = newKey
		binding.LastSeenAt = time.Now()
		p.log("会话迁移: conv=%s... %s... → %s...",
			convID[:minStr(8, len(convID))],
			oldKey[:minStr(12, len(oldKey))],
			newKey[:minStr(12, len(newKey))])
	} else {
		// New binding
		p.sessionMap[convID] = &SessionBinding{
			PoolKey:    newKey,
			BoundAt:    time.Now(),
			LastSeenAt: time.Now(),
		}
	}
}

// Start starts the MITM proxy.
func (p *MitmProxy) Start() error {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return fmt.Errorf("代理已在运行")
	}
	if len(p.poolKeys) == 0 {
		p.mu.Unlock()
		return fmt.Errorf("号池为空，请先导入带 API Key 的账号")
	}
	p.mu.Unlock()

	// 1. Generate certificates
	p.log("生成 TLS 证书...")
	hostCert, err := EnsureCA(TargetDomain)
	if err != nil {
		return fmt.Errorf("证书生成失败: %w", err)
	}

	// 2. Setup TLS listener
	// ★ D-1: NextProtos 告诉客户端 ALPN 支持 h2 + http/1.1 fallback。
	//   旧实现只设 Certificates → IDE↔MITM 这段降级到 HTTP/1.1，多个并发请求
	//   被拖到 6 连接限制，每个连接独立 TLS 握手 → 首字慢 + 多 tab 卡。
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{*hostCert},
		NextProtos:   []string{"h2", "http/1.1"},
	}

	addr := fmt.Sprintf("127.0.0.1:%d", p.port)
	listener, err := listenTLS(p.port, tlsConfig)
	if err != nil {
		return fmt.Errorf("监听 %s 失败: %w", addr, err)
	}

	p.mu.Lock()
	p.listener = listener
	p.running = true
	p.stopCh = make(chan struct{})
	p.mu.Unlock()

	// P2: 启用 trafficLog 后台批量刷盘，避免每条 fsync 拖累 chat TTFB
	EnableTrafficLogAsync()

	p.log("代理已启动: %s", addr)

	// 3. Start JWT prefetch (synchronous — wait for first JWT)
	p.jwtOnce = sync.Once{}
	p.jwtReady = make(chan struct{})
	go p.prefetchJWTs()

	// Wait up to 15s for at least one JWT
	select {
	case <-p.jwtReady:
		p.log("✅ JWT 就绪，开始接受请求")
	case <-time.After(15 * time.Second):
		p.log("⚠️ JWT 预取超时，先接受请求（不替换身份）")
	}

	// 4. Start JWT refresh loop
	go p.jwtRefreshLoop()

	// 5. Serve requests
	go p.serve()

	return nil
}

// Stop stops the MITM proxy.
func (p *MitmProxy) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	close(p.stopCh)
	if p.listener != nil {
		p.listener.Close()
	}
	p.running = false
	p.log("代理已停止")
	return nil
}

// Status returns the current proxy status.
// ★ 注意锁顺序：先 p.mu 后 sessionsMu，与 pickPoolKeyForSession（先 sessionsMu 后 p.mu）冲突。
// 所以这里必须先完成 p.mu 读取并释放，再单独获取 sessionsMu，避免死锁。
func (p *MitmProxy) Status() MitmProxyStatus {
	// ── Phase 1: 在 p.mu 下收集号池信息，然后释放 ──
	p.mu.RLock()
	status := MitmProxyStatus{
		Running:          p.running,
		Port:             p.port,
		HostsMapped:      IsHostsMapped(TargetDomain),
		CAInstalled:      IsCAInstalled(),
		LastErrorKind:    p.lastErrorKind,
		LastErrorSummary: p.lastErrorSummary,
		LastErrorAt:      p.lastErrorAt,
		LastErrorKey:     p.lastErrorKey,
	}

	// 拷贝 poolKeys 供后续 session 阶段使用（避免再次拿 p.mu）
	poolKeysCopy := make([]string, len(p.poolKeys))
	copy(poolKeysCopy, p.poolKeys)

	totalReqs := 0
	for i, k := range p.poolKeys {
		state := p.keyStates[k]
		if state == nil {
			continue
		}
		totalReqs += state.RequestCount

		short := k
		if len(k) > 16 {
			short = k[:16] + "..."
		}

		p.jwtLock.RLock()
		hasJWT := len(state.JWT) > 0
		p.jwtLock.RUnlock()

		info := PoolKeyInfo{
			KeyShort:         short,
			KeyHash:          HashPoolKey(k),
			Plan:             state.Plan,
			Healthy:          state.Healthy,
			Disabled:         state.Disabled,
			RuntimeExhausted: state.RuntimeExhausted,
			CooldownUntil:    state.CooldownUntil.Format(time.RFC3339),
			HasJWT:           hasJWT,
			RequestCount:     state.RequestCount,
			SuccessCount:     state.SuccessCount,
			TotalExhausted:   state.TotalExhausted,
			IsCurrent:        i == p.currentIdx,
		}
		status.PoolStatus = append(status.PoolStatus, info)

		if info.IsCurrent {
			status.CurrentKey = short
		}
	}
	status.TotalReqs = totalReqs
	p.mu.RUnlock() // ★ 释放 p.mu，再拿 sessionsMu

	status.RecentEvents = p.recentEventsSnapshot()

	// ── Phase 2: 在 sessionsMu 下收集会话信息（不持有 p.mu） ──
	p.sessionsMu.RLock()
	now := time.Now()
	for _, sb := range p.sessionMap {
		if now.Sub(sb.LastSeenAt) > time.Duration(sessionExpireMinutes)*time.Minute {
			continue
		}
		status.ActiveSessions = append(status.ActiveSessions, SessionBindingInfo{
			ConvID:       sb.ConversationID,
			ConvIDShort:  sb.ConversationID[:minStr(12, len(sb.ConversationID))] + "...",
			PoolKeyShort: sb.PoolKey[:minStr(16, len(sb.PoolKey))] + "...",
			BoundAt:      sb.BoundAt.Format(time.RFC3339),
			LastSeenAt:   sb.LastSeenAt.Format(time.RFC3339),
			RequestCount: sb.RequestCount,
			Title:        sb.Title,
		})
	}
	// ★ SessionCount 仅统计活跃（未过期）会话，与 ActiveSessions 列表一致
	status.SessionCount = len(status.ActiveSessions)

	// Fill per-key BoundSessionCount
	if len(status.PoolStatus) > 0 {
		for i := range status.PoolStatus {
			fullKey := ""
			for _, k := range poolKeysCopy {
				short := k
				if len(k) > 16 {
					short = k[:16] + "..."
				}
				if short == status.PoolStatus[i].KeyShort {
					fullKey = k
					break
				}
			}
			if fullKey != "" {
				// ★ BoundSessionCount 反映该 key 上有多少活跃会话绑定（"在用" 状态）
				// IsCurrent 只表示 pool currentIdx 当前指向的 key（前端"当前活跃"语义），
				// 不要因为 key 上有会话就强行标 IsCurrent，否则前端"当前活跃"会被夸大成全池。
				status.PoolStatus[i].BoundSessionCount = p.sessionBindingCount(fullKey)
			}
		}
	}
	p.sessionsMu.RUnlock()

	return status
}

func (p *MitmProxy) CurrentAPIKey() string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.poolKeys) == 0 || p.currentIdx < 0 || p.currentIdx >= len(p.poolKeys) {
		return ""
	}
	return p.poolKeys[p.currentIdx]
}

// buildUpstreamTransport 构建出站 Transport，支持通过用户本地代理 (如 Clash) 访问上游
//
// ★ D-3: 显式启用 http2 + ping。旧实现仅 ForceAttemptHTTP2=true，没有 ping
// 健康检查 → 上游间歇性 RST_STREAM / 静默断连时，死连接被复用，单次请求
// 必须等到 ResponseHeaderTimeout 才报错或重传。ReadIdleTimeout 让连接 30s
// 无数据时主动 PING，PingTimeout 15s 内不响应即关闭重建。
func (p *MitmProxy) buildUpstreamTransport() *http.Transport {
	t := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         UpstreamHost,
			NextProtos:         []string{"h2"}, // ★ D-3: Windsurf 强制 h2，去掉 http/1.1 fallback
		},
		ForceAttemptHTTP2:     true,
		DisableCompression:    true,
		MaxIdleConns:          100,
		MaxConnsPerHost:       20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 180 * time.Second,
	}
	if p.proxyURL != "" {
		if u, err := url.Parse(p.proxyURL); err == nil {
			t.Proxy = http.ProxyURL(u)
			p.log("出站代理: %s", p.proxyURL)
		}
	}
	// ★ D-3: ConfigureTransports（s 复数）返回 *http2.Transport 暴露 ping 字段，
	//   老版 ConfigureTransport 不暴露能力受限。
	if h2t, err := http2.ConfigureTransports(t); err == nil {
		h2t.ReadIdleTimeout = 30 * time.Second
		h2t.PingTimeout = 15 * time.Second
	} else {
		p.log("http2.ConfigureTransports 失败: %v (回退 ForceAttemptHTTP2)", err)
	}
	return t
}

// retryTransport 包装上游 Transport，在检测到额度耗尽时自动切号并重试
type retryTransport struct {
	base     http.RoundTripper
	proxy    *MitmProxy
	maxRetry int
}

type upstreamFailureKind string

const (
	upstreamFailureNone            upstreamFailureKind = ""
	upstreamFailureRateLimit       upstreamFailureKind = "rate_limit"
	upstreamFailureQuota           upstreamFailureKind = "quota"
	upstreamFailureAuth            upstreamFailureKind = "auth"
	upstreamFailureInternal        upstreamFailureKind = "internal"
	upstreamFailurePermission      upstreamFailureKind = "permission"
	upstreamFailureGRPC            upstreamFailureKind = "grpc"
	upstreamFailureGlobalRateLimit upstreamFailureKind = "global_rate_limit"
	upstreamFailureUnavailable     upstreamFailureKind = "unavailable"
)

// isGlobalRateLimitText 检测全局限速（非单 key 限速）关键词。
func isGlobalRateLimitText(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "global") && strings.Contains(lower, "rate") && strings.Contains(lower, "limit")
}

func (rt *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// 保存原始 body 以便重试时重放
	// ★ D-2: 优先用 req.GetBody（handleRequest 已处理过身份替换并通过
	//   setReqBody 设置 GetBody），避免对同一份 body 第二次 ReadAll +
	//   cloneBytes —— 长对话场景内存峰值 3× → 1×。
	//   GetBody 模式下 retryBody=savedBody（重试也调 GetBody 不需 clone）。
	//   非 GetBody 兼容路径（外部直接调 RoundTrip 不经过 handleRequest）保留旧行为。
	var savedBody []byte
	var retryBody []byte
	usedGetBody := false
	if req.GetBody != nil {
		rc, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		savedBody, err = io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		retryBody = savedBody
		usedGetBody = true
		req.Body = io.NopCloser(bytes.NewReader(savedBody))
		req.ContentLength = int64(len(savedBody))
	} else if req.Body != nil {
		var err error
		savedBody, err = io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, err
		}
		retryBody = cloneBytes(savedBody)
		req.Body = io.NopCloser(bytes.NewReader(savedBody))
		req.ContentLength = int64(len(savedBody))
	}

	if savedBody != nil {
		if strings.Contains(req.URL.Path, "GetChatMessage") || strings.Contains(req.URL.Path, "GetCompletions") {
			model, pt := ExtractMitmMetadata(savedBody)
			if model != "" {
				req.Header.Set("X-Mitm-Model", model)
			}
			req.Header.Set("X-Mitm-Prompt-Tokens", fmt.Sprintf("%d", pt))
			req.Header.Set("X-Mitm-Start-Time", fmt.Sprintf("%d", time.Now().UnixMilli()))
		}
	}
	_ = usedGetBody // 未来可用作日志/metric

	// ★ D-2: 重试路径中同步更新 req.Body / ContentLength / GetBody。
	//   后续 retry 点修改 retryBody 后，http2.Transport 可能在连接断开
	//   场景调 GetBody 重拿 body；不同步更新会拿到旧 currentBody。
	setRetryBody := func(b []byte) {
		retryBody = b
		req.Body = io.NopCloser(bytes.NewReader(b))
		req.ContentLength = int64(len(b))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(b)), nil
		}
	}

	// Strip internal headers before sending upstream
	convID := req.Header.Get("X-Conv-ID")
	req.Header.Del("X-Conv-ID")

	for attempt := 0; attempt <= rt.maxRetry; attempt++ {
		resp, err := rt.base.RoundTrip(req)
		if err != nil {
			return nil, err
		}

		// ── 判断是否可以缓冲读取以检测错误 ──
		// Windsurf 使用 Connect 协议：
		//   流式端点(GetChatMessage/GetCompletions): HTTP 200 + Connect frames
		//     - 错误通过 EOS trailer 帧(flag&0x02)返回，通常在最后几百字节
		//     - 正常数据通过 data frames 返回，可能很大
		//   非流式端点: HTTP 4xx + JSON body
		//
		// 策略:
		//   HTTP 4xx → 一定缓冲读取检测错误
		//   HTTP 200 + 小包(CL已知且<5000 或 CL=-1 但 CT=application/json) → 缓冲
		//   HTTP 200 + 大流式(CL=-1, CT=connect+proto) → 交给 handleResponse 的 ConnectStreamWatcher
		canBuffer := false
		if resp.StatusCode >= 400 {
			canBuffer = true
		} else if resp.ContentLength >= 0 && resp.ContentLength < 5000 {
			canBuffer = true
		} else if resp.StatusCode == 200 {
			ct := strings.ToLower(resp.Header.Get("Content-Type"))
			if strings.Contains(ct, "json") {
				canBuffer = true // Connect 协议异常: 200 + application/json
			}
		}

		if !canBuffer {
			// ★ 性能关键：CL=-1 的 connect+proto 响应只 peek **5 字节** Connect 帧头
			//
			// 旧实现 peek 前 8KB(io.ReadFull) 会强制等上游凑齐 8193 字节才把响应交给
			// IDE —— 这是「直连秒开、MITM 拖几秒首字」的根因。流式 chat 的首批
			// data frame 通常每个几十~几百字节，累积 8KB 取决于上游发包节奏，实测可
			// 多出 100-1500ms 的首字延迟。
			//
			// 新策略：只读首帧 5 字节头(flag + 4字节 BE payloadLen)：
			//   - flag&0x02 != 0 且 payloadLen 合理(<=4KB) → EOS-only 错误帧 →
			//     缓冲走错误分类(rate limit / quota 检测仍生效)
			//   - 其余(普通 data 帧 / 非法长度) → MultiReader 还原首 5B，立即
			//     return，零首字延迟
			//
			// 错误检测能力不丢：rate limit / quota 错误响应都是 EOS-only 单帧，
			// 仍然走缓冲分支。普通流式 data 帧的错误已经由下游 ConnectStreamWatcher
			// 在 stream 末尾(EOS trailer)兜底检测。
			if resp.StatusCode == 200 && resp.ContentLength < 0 {
				const headLen = 5
				const eosPayloadCap = 4096 // EOS 错误帧通常 <1KB，4KB 是宽松上限
				head := make([]byte, headLen)
				n, peekErr := io.ReadFull(resp.Body, head)
				if n < headLen {
					// 上游 EOF 在 5 字节内 —— 异常小响应，全缓冲走错误检测
					resp.Body.Close()
					resp.Body = io.NopCloser(bytes.NewReader(head[:n]))
					resp.ContentLength = int64(n)
					canBuffer = true
					// fall through to error detection below
				} else {
					flag := head[0]
					payloadLen := int(binary.BigEndian.Uint32(head[1:5]))
					isEOS := flag&0x02 != 0
					if isEOS && payloadLen >= 0 && payloadLen <= eosPayloadCap {
						// EOS-only 错误帧：读完整帧后走缓冲错误分类
						rest := make([]byte, payloadLen)
						m, _ := io.ReadFull(resp.Body, rest)
						resp.Body.Close()
						full := make([]byte, 0, headLen+m)
						full = append(full, head...)
						full = append(full, rest[:m]...)
						resp.Body = io.NopCloser(bytes.NewReader(full))
						resp.ContentLength = int64(len(full))
						canBuffer = true
						// fall through to error detection below
					} else {
						// 普通 data 帧 / 非法长度：立即 passthrough，零首字延迟
						resp.Body = struct {
							io.Reader
							io.Closer
						}{
							Reader: io.MultiReader(bytes.NewReader(head), resp.Body),
							Closer: resp.Body,
						}
						_ = peekErr
						return resp, nil
					}
				}
			} else {
				return resp, nil
			}
		}

		// 读取响应体
		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			return resp, nil
		}

		// ── Connect 协议错误检测 ──
		// 1. 先尝试 Connect EOS 帧解析（流式端点的错误格式）
		// 2. 再回退到旧的 gRPC header + body text 检测（兼容）
		var kind upstreamFailureKind
		var detail string
		var isCascadeSession bool

		connectResult := ParseConnectEOS(respBody)
		if connectResult.IsError {
			kind, detail = ClassifyConnectError(connectResult)
			isCascadeSession = IsCascadeSessionError(connectResult)
			rt.proxy.log("Connect错误检测: code=%s msg=%s kind=%s cascade=%v path=%s",
				connectResult.Code, truncate(connectResult.Message, 80), kind, isCascadeSession,
				req.URL.Path)
		} else {
			// Fallback: 旧的 gRPC header + body text 检测
			grpcStatus := resp.Header.Get("grpc-status")
			grpcMsg := resp.Header.Get("grpc-message")
			if grpcStatus == "" {
				grpcStatus = resp.Trailer.Get("grpc-status")
			}
			if grpcMsg == "" {
				grpcMsg = resp.Trailer.Get("grpc-message")
			}
			kind, detail = classifyUpstreamFailure(grpcStatus, grpcMsg, string(respBody))
			isCascadeSession = isCascadeSessionFailure(grpcStatus, grpcMsg, string(respBody))
		}

		// ── Invalid Cascade session → 不切号不剥离不重试，直接透传给 IDE ──
		// 切号无意义（新号也没有这个 conversation 的 session），剥离 conv_id 会导致 Invalid argument
		// IDE 会自动重新发起新对话
		if isCascadeSession {
			usedKey := req.Header.Get("X-Pool-Key-Used")
			convShort := convID
			if len(convShort) > 8 {
				convShort = convShort[:8]
			}
			rt.proxy.log("★ Cascade 会话失效(不重试，透传给IDE): path=%s key=%s conv=%s",
				req.URL.Path, safeUsedKeyForLog(usedKey), convShort)
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			return resp, nil
		}

		isExhausted := kind == upstreamFailureQuota
		isRateLimited := kind == upstreamFailureRateLimit
		isGlobalRateLimit := kind == upstreamFailureGlobalRateLimit
		isUnavailable := kind == upstreamFailureUnavailable
		isAuthFailure := kind == upstreamFailureAuth || kind == upstreamFailurePermission
		usedKey := req.Header.Get("X-Pool-Key-Used")

		// ── 上游不可达 (Model provider unreachable)：用同一 key 重试 ──
		if isUnavailable && attempt < rt.maxRetry {
			rt.proxy.recordUpstreamFailure(kind, detail, usedKey)
			rt.proxy.log("★ 上游不可达，同 key 重试(%d/%d): %s... path=%s",
				attempt+1, rt.maxRetry, usedKey[:minStr(12, len(usedKey))], req.URL.Path)
			setRetryBody(retryBody)
			continue
		}

		// ── Trial 限速：标记冷却+轮转，透传给 IDE ──
		// 不能在 retryTransport 里换号重试，因为换号后 Cascade session 不匹配
		// (服务端校验 session 绑定到 key)，会返回 "Invalid Cascade session"。
		// 正确做法：标记当前 key 冷却 → 轮转到新 key → 透传错误给 IDE →
		// IDE 自动重试时，新请求会用新 key + 正确的 session。
		if isGlobalRateLimit {
			rt.proxy.recordUpstreamFailure(kind, detail, usedKey)
			// 标记当前 trial key 冷却
			if rotatedKey := rt.proxy.markRateLimitedAndRotate(usedKey, detail); rotatedKey != "" {
				rt.proxy.log("★ Trial限速→轮转: %s... → %s... (透传给IDE，下次请求用新key)",
					usedKey[:minStr(12, len(usedKey))],
					rotatedKey[:minStr(12, len(rotatedKey))])
			} else {
				rt.proxy.log("★ Trial限速，已标记冷却: %s... (透传给IDE)", usedKey[:minStr(12, len(usedKey))])
			}
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			return resp, nil
		}

		if (!isExhausted && !isAuthFailure && !isRateLimited) || attempt >= rt.maxRetry {
			if kind != upstreamFailureNone {
				rt.proxy.recordUpstreamFailure(kind, detail, usedKey)
				if attempt >= rt.maxRetry {
					rt.proxy.log("上游%s错误(已达重试上限%d): %s", kind.logLabel(), rt.maxRetry, detail)
				} else {
					rt.proxy.log("上游%s错误(不可重试): %s", kind.logLabel(), detail)
				}
			}
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			return resp, nil
		}

		// ── 执行轮转/重试 ──
		if isAuthFailure {
			if rotatedKey := rt.proxy.rotateAfterAuthFailure(usedKey, detail); rotatedKey == "" {
				refreshedJWT := rt.proxy.refreshJWTForKey(usedKey)
				if len(refreshedJWT) > 0 {
					rt.proxy.mu.RLock()
					authFP := rt.proxy.keyFingerprint(usedKey)
					rt.proxy.mu.RUnlock()
					newBody, replaced := ReplaceIdentityInBody(retryBody, []byte(usedKey), refreshedJWT, false, authFP)
					if replaced {
						setRetryBody(newBody)
					} else {
						setRetryBody(retryBody)
					}
					req.Header.Set("Authorization", "Bearer "+string(refreshedJWT))
					req.Header.Set("X-Pool-Key-Used", usedKey)
					rt.proxy.log("★ 认证失败自动刷新 JWT(%d/%d): %s...",
						attempt+1, rt.maxRetry,
						usedKey[:minStr(12, len(usedKey))])
					continue
				}

				rt.proxy.log("认证失败且 JWT 刷新失败，无可用备用 key: %s...", usedKey[:minStr(12, len(usedKey))])
				resp.Body = io.NopCloser(bytes.NewReader(respBody))
				return resp, nil
			}
		} else if isRateLimited {
			// ★ 限速：标记冷却+轮转 pool currentIdx，永远透传错误给 IDE（不在响应内重试）
			//   - 用户希望先看到错误再切号，避免连锁切号烧 key
			//   - markRateLimitedAndRotate 内部 debounce，多并发命中只切一次
			//   - 已绑定 convID 的会话保持粘性（避免 Invalid Cascade session）
			//   - IDE 重试时新请求会用新 key
			if rotatedKey := rt.proxy.markRateLimitedAndRotate(usedKey, detail); rotatedKey != "" {
				rt.proxy.log("★ Rate limit→轮转: %s... → %s... (透传给IDE)",
					usedKey[:minStr(12, len(usedKey))],
					rotatedKey[:minStr(12, len(rotatedKey))])
			} else {
				rt.proxy.log("★ Rate limit 已冷却 key=%s... (透传给IDE)", usedKey[:minStr(12, len(usedKey))])
			}
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			return resp, nil
		} else {
			// ★ 额度耗尽，切号
			rt.proxy.markRuntimeExhaustedAndRotate(usedKey, detail)
		}
		// ★ 有 convID + 额度耗尽：尝试用新 key 重试（key 不会自动恢复，必须迁移）
		// pickPoolKeyForSession 会检测到 RuntimeExhausted 并分配新 key

		// 用新号重新构造请求（仅无 convID 的新对话，或认证失败刷 JWT 重试）
		var newKey string
		var newJWT []byte
		if convID != "" {
			// 认证失败重试：保持 session 粘性（同一个 key 刷新 JWT 后重试）
			newKey, newJWT = rt.proxy.pickPoolKeyForSession(convID)
		} else {
			// 无 convID 的新对话：可以用新号
			newKey, newJWT = rt.proxy.pickPoolKeyAndJWT()
		}
		if newKey == "" || len(newJWT) == 0 {
			rt.proxy.log("重试失败: 无可用号池 key")
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			return resp, nil
		}

		// 重新替换身份
		rt.proxy.mu.RLock()
		retryRandFP := rt.proxy.isTrialOrFreeKey(newKey)
		retryFP := rt.proxy.keyFingerprint(newKey)
		rt.proxy.mu.RUnlock()
		newBody, replaced := ReplaceIdentityInBody(retryBody, []byte(newKey), newJWT, retryRandFP, retryFP)
		if replaced {
			setRetryBody(newBody)
		} else {
			setRetryBody(retryBody)
		}
		req.Header.Set("Authorization", "Bearer "+string(newJWT))
		req.Header.Set("X-Pool-Key-Used", newKey)

		rt.proxy.log("★ %s自动重试(%d/%d): %s... → %s...",
			kind.logLabel(), attempt+1, rt.maxRetry,
			usedKey[:minStr(12, len(usedKey))],
			newKey[:minStr(12, len(newKey))])
	}

	return nil, fmt.Errorf("超过最大重试次数")
}

func (rt *retryTransport) checkExhausted(textLower string) bool {
	return isQuotaExhaustedText(textLower)
}

func (p *MitmProxy) newReverseProxy() *httputil.ReverseProxy {
	base := p.buildUpstreamTransport()
	p.mu.Lock()
	p.upstreamBase = base
	p.mu.Unlock()
	return &httputil.ReverseProxy{
		FlushInterval: -1,
		Director: func(req *http.Request) {
			// ★ 保留原始 Host（可能是 server.self-serve.windsurf.com 或 server.codeium.com）
			origHost := req.Host
			if origHost == "" || origHost == "127.0.0.1" || origHost == "127.0.0.1:443" {
				origHost = UpstreamHost
			}
			// 去掉端口部分
			if h, _, err := net.SplitHostPort(origHost); err == nil {
				origHost = h
			}

			p.handleRequest(req, origHost)
			req.URL.Scheme = "https"
			req.URL.Host = ResolveUpstreamIP()
			req.Host = origHost // 用原始域名作为 Host 头
		},
		Transport: &retryTransport{
			base:     base,
			proxy:    p,
			maxRetry: defaultReplayBudget,
		},
		ModifyResponse: func(resp *http.Response) error {
			p.handleResponse(resp)
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			p.log("上游错误: %s %s: %v", req.Method, req.URL.Path, err)
			w.WriteHeader(http.StatusBadGateway)
		},
	}
}

func (p *MitmProxy) serve() {
	proxy := p.newReverseProxy()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p.tryServeStaticCache(w, r) {
			return
		}
		// ★ 统计进行中的 chat 请求 → ClashRotator 切节点前会等空闲，避免断流
		if isChatPath(r.URL.Path) {
			atomic.AddInt64(&p.inFlightChatStreams, 1)
			defer atomic.AddInt64(&p.inFlightChatStreams, -1)
		}
		// ★ 阶段 2 提供商路由分流: 胶囊=providers 且 chat path 时,
		// 走 Route(cascade ↔ OpenAI/Anthropic/Gemini)
		// 而不是号池 reverse proxy。
		if p.tryServeRoute(w, r) {
			return
		}
		proxy.ServeHTTP(w, r)
	})
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 30 * time.Second,
	}
	// ★ D-1: 显式注册 h2 handler。Go 的 http.Server 自动 h2 仅在 ListenAndServeTLS
	//   路径下触发；这里走 tls.Listen + Serve 必须手动调用，否则即使 ALPN 协商
	//   到 h2，server 也不会处理 h2 帧 → 客户端协议错乱。
	if err := http2.ConfigureServer(server, &http2.Server{
		IdleTimeout:          120 * time.Second,
		MaxConcurrentStreams: 250,
	}); err != nil {
		p.log("http2.ConfigureServer 失败: %v (回退到 HTTP/1.1)", err)
	}

	if err := server.Serve(p.listener); err != nil {
		select {
		case <-p.stopCh:
			// normal shutdown
		default:
			p.log("服务异常退出: %v", err)
		}
	}
}

func (p *MitmProxy) handleRequest(req *http.Request, origHost string) {
	// 使用传入的原始域名设置 Host 头（可能是 server.self-serve.windsurf.com 或 server.codeium.com）
	req.Host = origHost
	req.Header.Set("Host", origHost)

	// ★ D-2: setReqBody 同时设置 Body / ContentLength / GetBody。
	// 设置 GetBody 后 retryTransport 重试可调 GetBody 重拿可重放的 ReadCloser，
	// 不再需要对 body 做第二次 ReadAll + cloneBytes —— 长对话 body
	// 内存峰值从 3× 降到 1×。
	setReqBody := func(b []byte) {
		req.Body = io.NopCloser(bytes.NewReader(b))
		req.ContentLength = int64(len(b))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(b)), nil
		}
	}

	// ★ 全量抓包：请求
	p.captureRequest(req)

	path := req.URL.Path
	pathTail := path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		pathTail = path[idx+1:]
	}

	ct := req.Header.Get("Content-Type")
	isProto := strings.Contains(strings.ToLower(ct), "proto") || strings.Contains(strings.ToLower(ct), "grpc")

	if shouldCaptureTrafficPath(path) {
		trafficLog("REQ  %s %s (host=%s ct=%s cl=%d)", req.Method, path, origHost, ct, req.ContentLength)
	}

	if !isProto {
		// Non-protobuf requests: just forward
		return
	}

	// ★ 只有承载 conversation_id 的路径才注入号池身份 ──
	//
	// 需要替换：
	//   - Chat/Completions     (api_server_pb.GetChatMessage / GetCompletions)  → 计费归属号池
	//   - Cortex / Trajectory  (CortexService / TrajectoryService)              → 会话生命周期
	//
	// 一律透传 IDE 真实凭据的路径（mayHaveConversationID == false）：
	//   - 登录/认证      (auth_pb.AuthService/*)            ← 替换会卡死登录
	//   - 用户/套餐状态  (seat_management_pb.*/GetUserStatus|GetPlanStatus)
	//   - 插件注册表     (cascade_plugins_pb.*/GetAllAcpRegistries)
	//   - 心跳/工作流    (api_server_pb.*/Ping|GetDefaultWorkflowTemplates|...)
	//   - 用户档案       (GetProfileData)
	//
	// 历史教训：commit d322656 把所有非 conv 路径都替换成号池 JWT，目的是隐藏
	// IDE 缓存 token 过期时的 401。但实测代价是 IDE 永远登不上(GetUserStatus/
	// GetAllAcpRegistries 用号池失效 JWT 校验 → "failed to validate Devin
	// token: Invalid token")。回归 6d5fcc4 的设计：身份路径透传，IDE 自己用
	// auth_pb 续期，号池只接管聊天计费。
	//
	// 注意：proxy 自身预取号池 JWT/UserStatus 走 WindsurfService 直连
	// ResolveUpstreamIP()，不经过 MITM listener，所以这里透传不影响号池流程。
	if !mayHaveConversationID(path) {
		return
	}

	// Read body — 只对可能包含 conversation_id 的路径 (Chat/Cortex/Trajectory)
	if req.Body == nil {
		return
	}
	bodyBytes, err := io.ReadAll(req.Body)
	req.Body.Close()
	if err != nil || len(bodyBytes) == 0 {
		setReqBody(bodyBytes)
		return
	}

	// ★ 提取 conversation_id 分流路由
	convID, convDbg := ExtractConversationIDFromBody(bodyBytes)

	// ★ 迁移会话主动剥离 conv_id，避免 Invalid Cascade session 重试开销
	if convID != "" {
		var migrated bool
		// var migratedKeyShort string
		p.sessionsMu.RLock()
		if binding := p.sessionMap[convID]; binding != nil && binding.Migrated {
			migrated = true
			/*
				if len(binding.PoolKey) > 12 {
					migratedKeyShort = binding.PoolKey[:12] + "..."
				} else {
					migratedKeyShort = binding.PoolKey
				}
			*/
		}
		p.sessionsMu.RUnlock()
		if migrated {
			/*
				if stripped, ok := StripConversationIDFromBody(bodyBytes); ok {
					bodyBytes = stripped
					p.log("迁移会话主动剥离conv_id: conv=%s... key=%s",
						convID[:minStr(8, len(convID))], migratedKeyShort)
				}
			*/
		}
	}

	if convID == "" {
		if isChatPath(path) {
			// 新对话首条消息(无 conv_id)也需要用号池 key
			p.log("session路由(新对话): path=%s [%s]", pathTail, convDbg)
		} else {
			// Cortex/Trajectory 请求但无 conv_id → 透传
			setReqBody(bodyBytes)
			return
		}
	} else {
		p.log("session路由: path=%s convID=%s [%s]", pathTail, convID, convDbg)
	}

	var poolKey string
	var poolJWT []byte

	if convID != "" {
		poolKey, poolJWT = p.pickPoolKeyForSession(convID)
		req.Header.Set("X-Conv-ID", convID)
		// Extract title hint for session binding display
		if titleHint := ExtractSessionTitleHint(bodyBytes); titleHint != "" {
			p.sessionsMu.Lock()
			if binding := p.sessionMap[convID]; binding != nil && binding.Title == "" {
				binding.Title = titleHint
			}
			p.sessionsMu.Unlock()
		}
	} else {
		// ★ 首条消息(无 convID)使用 session 感知分配，而非固定 currentKey
		poolKey, poolJWT = p.pickKeyForNewConversation()
	}

	if poolKey == "" || len(poolJWT) == 0 {
		// ★ 核心安全逻辑：没有 JWT 绝不替换身份，直接透传原始请求
		if poolKey == "" {
			p.log("无可用号池 key")
		} else {
			p.log("跳过身份替换: %s (JWT 未就绪)", pathTail)
		}
		setReqBody(bodyBytes)
		return
	}

	// P4: 一次 RLock 取出本请求需要的所有 mu 保护字段，减少锁切换。
	// 旧实现 3 次连续 RLock/RUnlock + 1 次 DebugDumpEnabled() 内 RLock，热路径
	// 锁开销可观；snapshot 后所有判断都基于不可变副本。
	p.mu.RLock()
	randFP := p.isTrialOrFreeKey(poolKey)
	fp := p.keyFingerprint(poolKey)
	jb := p.jailbreakConfig
	sfEnabled := p.smartFriendEnabled
	debugDumpEnabled := p.debugDump
	p.mu.RUnlock()

	// Replace identity in protobuf body
	// ★ 每个 key 用独立的 session ID 和设备指纹（防 session 级限速）
	newBody, replaced := ReplaceIdentityInBody(bodyBytes, []byte(poolKey), poolJWT, randFP, fp)
	// ★ D-2: currentBody 跟踪当前身份、破限、F7 patch 连环后的最终 body。
	//   避免后续段从 req.Body 反读 (旧实现 SmartFriend 那段 io.ReadAll)。
	currentBody := bodyBytes
	if replaced {
		currentBody = newBody
		sid := ""
		if fp != nil {
			sid = fp.SessionID[:minStr(8, len(fp.SessionID))]
		}
		p.log("身份替换: %s key=%s...%s sid=%s fp=%v", pathTail, poolKey[:minStr(12, len(poolKey))], suffix3(poolKey), sid, randFP)
	}
	setReqBody(currentBody)

	// ★ 破限注入：仅 chat 路径(GetChatMessage / GetCompletions)，向 F2 system
	// prompt 末尾追加 override 文本。Cortex / Trajectory 不带 system prompt，
	// 跳过避免破坏其它消息。
	if jb.Enabled && jb.Override != "" && isChatPath(path) {
		if injected, ok := InjectSystemPromptOverride(currentBody, jb.Override); ok {
			p.jailbreakStats.record()
			p.log("破限注入: %s (+%dB to system prompt, total=%d today=%d)",
				pathTail, len(injected)-len(currentBody),
				p.jailbreakStats.totalInjects.Load(),
				p.jailbreakStats.snapshot().TodayInjects)
			currentBody = injected
			setReqBody(currentBody)
		}
	}

	// F7-REMOVAL: 下面整个 if sfEnabled && isChatPath(path) 块删除
	// ★ SmartFriend F7 patch: CASCADE(5) → SMART_FRIEND(13)
	// ★ D-2: 旧实现这里又 io.ReadAll(req.Body) 拿 currentBody，复用上面本地变量免到。
	if sfEnabled && isChatPath(path) {
		if patched, ok := PatchF7ToSmartFriend(currentBody); ok {
			p.log("SmartFriend F7 patch: %s (+%dB)", pathTail, len(patched)-len(currentBody))
			currentBody = patched
			setReqBody(currentBody)
		}
	}

	// Debug dump: GetChatMessage 请求
	if debugDumpEnabled && strings.Contains(path, "GetChatMessage") {
		if dumpPath, err := WriteProtoDump("req_"+pathTail, bodyBytes); err == nil {
			p.log("📝 dump 请求: %s", dumpPath)
		}
	}

	// Force Authorization header
	req.Header.Set("Authorization", "Bearer "+string(poolJWT))

	// Store pool key in request context via header (for response tracking)
	req.Header.Set("X-Pool-Key-Used", poolKey)
}

func (p *MitmProxy) recordMitmUsage(req *http.Request, usedKey string, completionTokens int, kind upstreamFailureKind, detail string) {
	if p.usageTracker == nil || req == nil {
		return
	}
	model := req.Header.Get("X-Mitm-Model")
	promptTokensStr := req.Header.Get("X-Mitm-Prompt-Tokens")
	if model == "" && promptTokensStr == "" {
		return
	}
	promptTokens := 0
	fmt.Sscanf(promptTokensStr, "%d", &promptTokens)

	var durationMs int64 = 0
	if startStr := req.Header.Get("X-Mitm-Start-Time"); startStr != "" {
		var startMs int64
		if _, err := fmt.Sscanf(startStr, "%d", &startMs); err == nil && startMs > 0 {
			durationMs = time.Now().UnixMilli() - startMs
		}
	}

	status := "ok"
	if kind != upstreamFailureNone {
		status = "error"
	}

	p.usageTracker.Record(UsageRecord{
		Model:            model,
		RequestModel:     "mitm-direct",
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
		DurationMs:       durationMs,
		APIKeyShort:      usedKey[:minStr(12, len(usedKey))],
		Status:           status,
		ErrorDetail:      detail,
		Format:           "windsurf-mitm",
	})
}

func (p *MitmProxy) handleResponse(resp *http.Response) {
	// ★ D-4: 一次 RLock 拿出本响应路径需要的所有 snapshot，热路径减少锁切换。
	// 旧实现：FullCaptureEnabled() / DebugDumpEnabled() / forgeConfig 各
	// 独立 RLock，高 QPS 下 reader 队列累积竞争。
	p.mu.RLock()
	fullCapture := p.fullCapture
	debugDumpEnabled := p.debugDump
	forgeCfg := p.forgeConfig
	p.mu.RUnlock()

	// ★ 全量抓包：响应（仅启用时才进入，省掉 captureResponse 内部一次 RLock）
	if fullCapture {
		p.captureResponse(resp)
	}

	path := resp.Request.URL.Path
	pathTail := path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		pathTail = path[idx+1:]
	}

	respCT := resp.Header.Get("Content-Type")
	grpcSt := resp.Header.Get("grpc-status")
	if shouldCaptureTrafficPath(path) {
		trafficLog("RESP %s %s → %d (ct=%s cl=%d grpc-status=%s)", resp.Request.Method, path, resp.StatusCode, respCT, resp.ContentLength, grpcSt)
	}

	if shouldCaptureTrafficPath(path) && resp.Body != nil && resp.ContentLength >= 0 && resp.ContentLength < 500000 {
		bodySnap, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err == nil && len(bodySnap) > 0 {
			seq := int(trafficSeq.Load())
			dumpPath := TrafficDumpBody(seq, sanitizePathForFile(pathTail), bodySnap)
			trafficLog("  DUMP %s (%d bytes) → %s", pathTail, len(bodySnap), dumpPath)
		}
		resp.Body = io.NopCloser(bytes.NewReader(bodySnap))
	}

	usedKey := resp.Request.Header.Get("X-Pool-Key-Used")
	resp.Request.Header.Del("X-Pool-Key-Used") // clean up internal header

	if usedKey == "" {
		return
	}

	// 优先检查响应 Content-Type；某些 gRPC 上游不返回 CT 时回退到请求 CT
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = resp.Request.Header.Get("Content-Type")
	}
	isProto := strings.Contains(strings.ToLower(ct), "proto") || strings.Contains(strings.ToLower(ct), "grpc")
	isBilling := strings.Contains(path, "GetChatMessage") || strings.Contains(path, "GetCompletions")

	// Check for exhaustion/quota errors in ALL protobuf responses
	isExhausted := false
	isSuccess := false
	exhaustedDetail := ""

	if isProto && resp.Body != nil {
		// ── Connect 协议错误检测 ──
		// Windsurf 使用 Connect 协议:
		//   流式端点(GetChatMessage/GetCompletions): HTTP 200 + Connect frames
		//     错误通过 EOS trailer 帧(flag&0x02)返回
		//   非流式端点: HTTP 4xx + JSON body {"code":"xxx","message":"yyy"}
		//   协议异常: HTTP 200 + Content-Type: application/json (应为 connect+proto)
		ct := strings.ToLower(resp.Header.Get("Content-Type"))
		isConnectJSON := resp.StatusCode == 200 && strings.Contains(ct, "json")
		shouldCheckBuffered := (resp.ContentLength >= 0 && resp.ContentLength < 5000) || resp.StatusCode >= 400 || isConnectJSON
		shouldWatchStream := isBilling && resp.StatusCode == 200 && !shouldCheckBuffered

		if isBilling {
			trafficLog("  BILLING-PATH: path=%s buffered=%v stream=%v cl=%d status=%d ct=%s key=%s...",
				pathTail, shouldCheckBuffered, shouldWatchStream, resp.ContentLength, resp.StatusCode, ct, usedKey[:minStr(12, len(usedKey))])
		}

		if shouldCheckBuffered {
			bodyBytes, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err == nil {
				// 先用 Connect EOS 帧解析
				connectResult := ParseConnectEOS(bodyBytes)
				var kind upstreamFailureKind
				var detail string
				if connectResult.IsError {
					kind, detail = ClassifyConnectError(connectResult)
				} else {
					// Fallback: 旧的 gRPC header + body text
					gs := resp.Header.Get("grpc-status")
					gm := resp.Header.Get("grpc-message")
					if gs == "" {
						gs = resp.Trailer.Get("grpc-status")
					}
					if gm == "" {
						gm = resp.Trailer.Get("grpc-message")
					}
					kind, detail = classifyUpstreamFailure(gs, gm, string(bodyBytes))
				}

				switch {
				case kind == upstreamFailureQuota:
					isExhausted = true
					exhaustedDetail = detail
					p.log("额度耗尽(buffered): %s key=%s... %s", pathTail, usedKey[:minStr(12, len(usedKey))], truncate(detail, 100))
				case kind == upstreamFailureGlobalRateLimit && isBilling:
					p.log("Trial限速(buffered): %s key=%s... (标记冷却，换号)", pathTail, usedKey[:minStr(12, len(usedKey))])
					if rotatedKey := p.markRateLimitedAndRotate(usedKey, detail); rotatedKey != "" {
						p.log("★ Trial限速立即轮转(buffered): %s... → %s...", usedKey[:minStr(12, len(usedKey))], rotatedKey[:minStr(12, len(rotatedKey))])
					}
					p.recordUpstreamFailure(kind, detail, usedKey)
				case kind == upstreamFailureRateLimit && isBilling:
					p.log("限速(buffered): %s key=%s...", pathTail, usedKey[:minStr(12, len(usedKey))])
					if rotatedKey := p.markRateLimitedAndRotate(usedKey, detail); rotatedKey != "" {
						p.log("★ 限速立即轮转: %s... → %s...", usedKey[:minStr(12, len(usedKey))], rotatedKey[:minStr(12, len(rotatedKey))])
					}
				case (kind == upstreamFailureAuth || kind == upstreamFailurePermission) && isBilling:
					p.log("认证/权限失败(buffered): %s key=%s...", pathTail, usedKey[:minStr(12, len(usedKey))])
					if rotatedKey := p.rotateAfterAuthFailure(usedKey, detail); rotatedKey != "" {
						p.log("★ 认证失败立即轮转: %s... → %s...", usedKey[:minStr(12, len(usedKey))], rotatedKey[:minStr(12, len(rotatedKey))])
					}
				case kind != upstreamFailureNone && isBilling:
					p.recordUpstreamFailure(kind, detail, usedKey)
					p.log("上游%s错误(buffered): %s key=%s...", kind.logLabel(), truncate(detail, 100), usedKey[:minStr(12, len(usedKey))])
				}

				if debugDumpEnabled && strings.Contains(path, "GetChatMessage") {
					if dumpPath, err := WriteProtoDump("resp_small_"+pathTail, bodyBytes); err == nil {
						p.log("📝 dump 响应(buffered): %s", dumpPath)
					}
				}

				if isBilling {
					p.recordMitmUsage(resp.Request, usedKey, 0, kind, detail)
				}
			}
			resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		} else if shouldWatchStream {
			// ★ 流式响应: 用 ConnectStreamWatcher 检测 EOS trailer 帧中的错误
			var dumpBody io.ReadCloser
			if debugDumpEnabled && strings.Contains(path, "GetChatMessage") {
				dumpBody = newDumpTeeBody(resp.Body, "resp_stream_"+pathTail, p)
			}
			baseBody := resp.Body
			if dumpBody != nil {
				baseBody = dumpBody
			}

			resp.Body = NewConnectStreamWatcher(baseBody,
				// onError: Connect EOS 帧检测到错误
				func(ce ConnectErrorResult) {
					kind, detail := ClassifyConnectError(ce)
					p.log("流式Connect错误: %s code=%s msg=%s kind=%s key=%s...",
						pathTail, ce.Code, truncate(ce.Message, 80), kind, usedKey[:minStr(12, len(usedKey))])
					switch kind {
					case upstreamFailureQuota:
						if rotatedKey := p.markRuntimeExhaustedAndRotate(usedKey, detail); rotatedKey != "" {
							p.log("★ 流式额度耗尽轮转: %s... → %s...", usedKey[:minStr(12, len(usedKey))], rotatedKey[:minStr(12, len(rotatedKey))])
						}
					case upstreamFailureGlobalRateLimit:
						p.log("★ 流式Trial限速: %s key=%s... (标记冷却，换号)", pathTail, usedKey[:minStr(12, len(usedKey))])
						if rotatedKey := p.markRateLimitedAndRotate(usedKey, detail); rotatedKey != "" {
							p.log("★ 流式Trial限速轮转: %s... → %s...", usedKey[:minStr(12, len(usedKey))], rotatedKey[:minStr(12, len(rotatedKey))])
						}
						p.recordUpstreamFailure(kind, detail, usedKey)
					case upstreamFailureRateLimit:
						if rotatedKey := p.markRateLimitedAndRotate(usedKey, detail); rotatedKey != "" {
							p.log("★ 流式限速轮转: %s... → %s...", usedKey[:minStr(12, len(usedKey))], rotatedKey[:minStr(12, len(rotatedKey))])
						}
					case upstreamFailureAuth, upstreamFailurePermission:
						if rotatedKey := p.rotateAfterAuthFailure(usedKey, detail); rotatedKey != "" {
							p.log("★ 流式认证失败轮转: %s... → %s...", usedKey[:minStr(12, len(usedKey))], rotatedKey[:minStr(12, len(rotatedKey))])
						}
					default:
						p.recordUpstreamFailure(kind, detail, usedKey)
						p.log("流式上游%s错误: %s key=%s...", kind.logLabel(), truncate(detail, 100), usedKey[:minStr(12, len(usedKey))])
					}
					// Only record usage if it's billing endpoint
					if isBilling {
						p.recordMitmUsage(resp.Request, usedKey, 0, kind, detail)
					}
				},
				// onSuccess: 流式结束无错误
				func(completionTokens int) {
					p.mu.Lock()
					if state := p.keyStates[usedKey]; state != nil {
						state.recordSuccess()
					}
					p.mu.Unlock()
					if isBilling {
						p.recordMitmUsage(resp.Request, usedKey, completionTokens, upstreamFailureNone, "")
					}
				},
			)
		} else if isBilling && resp.StatusCode == 200 {
			isSuccess = true
			p.recordMitmUsage(resp.Request, usedKey, 0, upstreamFailureNone, "")
		}
	}

	// Capture JWT from GetUserJwt response
	if strings.Contains(path, "GetUserJwt") && resp.StatusCode == 200 && resp.Body != nil {
		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err == nil && len(bodyBytes) > 0 {
			jwt := ExtractJWTFromBody(bodyBytes)
			if jwt != "" && usedKey != "" {
				p.updateJWT(usedKey, []byte(jwt))
				p.log("捕获 JWT: key=%s... (%dB)", usedKey[:minStr(12, len(usedKey))], len(jwt))
			}
		}
		resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	// Forge GetUserStatus / GetPlanStatus responses
	// ★ D-4: forgeCfg 已在函数入口 snapshot、复用
	if forgeCfg.Enabled && resp.StatusCode == 200 && resp.Body != nil {
		isUserStatus := strings.Contains(path, "GetUserStatus")
		isPlanStatus := strings.Contains(path, "GetPlanStatus")
		if isUserStatus || isPlanStatus {
			forgeBody, forgeErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if forgeErr == nil && len(forgeBody) > 0 {
				var forged []byte
				if isUserStatus {
					forged = forgeUserStatusResponse(forgeBody, forgeCfg)
				} else {
					forged = forgePlanStatusResponse(forgeBody, forgeCfg)
				}
				resp.Body = io.NopCloser(bytes.NewReader(forged))
				resp.ContentLength = int64(len(forged))
				resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(forged)))
				resp.Header.Del("Content-Encoding")
				resp.Header.Del("Transfer-Encoding")
				p.log("伪造 %s: %s (%d→%d bytes)", pathTail, forgeCfg.FakeSubType, len(forgeBody), len(forged))
			} else {
				resp.Body = io.NopCloser(bytes.NewReader(forgeBody))
			}
		}
	}

	// Update key state
	rotatedKey := ""
	if isExhausted {
		p.log("★ key 额度耗尽，立即轮转: %s...", usedKey[:minStr(12, len(usedKey))])
		rotatedKey = p.markRuntimeExhaustedAndRotate(usedKey, exhaustedDetail)
	} else {
		p.mu.Lock()
		state := p.keyStates[usedKey]
		if state != nil && isSuccess && isBilling {
			state.recordSuccess()
		}
		p.mu.Unlock()
	}
	if rotatedKey != "" {
		p.log("★ 额度耗尽已切换到: %s...", rotatedKey[:minStr(12, len(rotatedKey))])
	}
}

// ── Pool key selection ──

func (p *MitmProxy) pickPoolKeyAndJWT() (string, []byte) {
	p.mu.Lock()
	if len(p.poolKeys) == 0 {
		p.mu.Unlock()
		return "", nil
	}

	// Check if current key is still available
	currentKey := p.poolKeys[p.currentIdx]
	previousKey := currentKey
	state := p.keyStates[currentKey]
	rotatedKey := ""
	if state != nil && !state.isAvailable() {
		// Current key cooling down, rotate
		rotatedKey = p.rotateKey()
		currentKey = p.poolKeys[p.currentIdx]
	}
	currentIdx := p.currentIdx
	keys := make([]string, len(p.poolKeys))
	copy(keys, p.poolKeys)
	p.mu.Unlock()
	if rotatedKey != "" && rotatedKey != previousKey {
		p.syncCurrentAPIKeyToClient(rotatedKey, MitmCurrentKeyChangeReasonUnavailableRotate)
	}

	jwt := p.usableJWTForKey(currentKey)

	// If current key has no JWT, find one that does
	if len(jwt) == 0 {
		for i := 0; i < len(keys); i++ {
			idx := (currentIdx + i) % len(keys)
			k := keys[idx]
			j := p.usableJWTForKey(k)
			if len(j) > 0 {
				changed := k != currentKey
				p.mu.Lock()
				for liveIdx, liveKey := range p.poolKeys {
					if liveKey == k {
						p.currentIdx = liveIdx
						break
					}
				}
				p.mu.Unlock()
				if changed {
					p.syncCurrentAPIKeyToClient(k, MitmCurrentKeyChangeReasonJWTFallback)
				}
				return k, j
			}
		}
	}

	return currentKey, jwt
}

func (p *MitmProxy) rotateKey() string {
	if len(p.poolKeys) <= 1 {
		if len(p.poolKeys) == 1 {
			return p.poolKeys[p.currentIdx]
		}
		return ""
	}
	oldKey := p.poolKeys[p.currentIdx]

	// Find next available key
	for i := 1; i < len(p.poolKeys); i++ {
		idx := (p.currentIdx + i) % len(p.poolKeys)
		state := p.keyStates[p.poolKeys[idx]]
		if state != nil && state.isAvailable() {
			p.currentIdx = idx
			p.log("轮转: %s... → %s...", oldKey[:minStr(12, len(oldKey))],
				p.poolKeys[idx][:minStr(12, len(p.poolKeys[idx]))])
			return p.poolKeys[idx]
		}
	}

	// All keys unavailable. Priority:
	//  1. Non-disabled, non-RuntimeExhausted (just cooling down) → shortest cooldown
	//  2. RuntimeExhausted but not disabled → shortest cooldown (App 层可能已刷新额度)
	//  3. Disabled → last resort
	type candidate struct {
		idx      int
		priority int // 0=cooldown, 1=exhausted, 2=disabled
		cd       time.Duration
	}
	var candidates []candidate
	for i, k := range p.poolKeys {
		state := p.keyStates[k]
		if state == nil {
			continue
		}
		pri := 0
		if state.Disabled {
			pri = 2
		} else if state.RuntimeExhausted {
			pri = 1
		}
		candidates = append(candidates, candidate{idx: i, priority: pri, cd: time.Until(state.CooldownUntil)})
	}
	if len(candidates) == 0 {
		return ""
	}
	// Sort: lowest priority first, then shortest cooldown
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.priority < best.priority || (c.priority == best.priority && c.cd < best.cd) {
			best = c
		}
	}
	p.currentIdx = best.idx
	chosen := p.poolKeys[best.idx]
	p.log("所有 key 不可用，选优先级最高者: %s... (pri=%d cd=%v)", chosen[:minStr(12, len(chosen))], best.priority, best.cd)
	return chosen
}

// SwitchToKey 手动切换 MITM 代理到指定 API Key（前端「切换到此账号」「下一席位」调用）
func (p *MitmProxy) SwitchToKey(apiKey string) bool {
	p.mu.Lock()
	switchedKey := ""

	for i, k := range p.poolKeys {
		if k == apiKey {
			p.currentIdx = i
			// 手动切换应真正解除 runtime exhausted / cooldown，允许用户立即重试这一席位。
			if state := p.keyStates[k]; state != nil {
				state.Healthy = true
				state.RuntimeExhausted = false
				state.CooldownUntil = time.Time{}
				state.ConsecutiveErrs = 0
			}
			p.log("手动切换: → %s...", apiKey[:minStr(12, len(apiKey))])
			switchedKey = k
			break
		}
	}
	p.mu.Unlock()
	if switchedKey == "" {
		return false
	}
	p.syncCurrentAPIKeyToClient(switchedKey, MitmCurrentKeyChangeReasonManualSwitch)
	return true
}

// SwitchToNext 手动切到 MITM 号池中的下一席位。
func (p *MitmProxy) SwitchToNext() string {
	p.mu.Lock()
	if len(p.poolKeys) == 0 {
		p.mu.Unlock()
		return ""
	}
	nextKey := p.rotateKey()
	if nextKey != "" {
		if state := p.keyStates[nextKey]; state != nil {
			state.Healthy = true
			state.RuntimeExhausted = false
			state.CooldownUntil = time.Time{}
			state.ConsecutiveErrs = 0
		}
	}
	p.mu.Unlock()
	if nextKey == "" {
		return ""
	}
	p.log("手动切换到下一席位: %s...", nextKey[:minStr(12, len(nextKey))])
	p.syncCurrentAPIKeyToClient(nextKey, MitmCurrentKeyChangeReasonManualNext)
	return nextKey
}

// ── Session-aware pool key selection ──

// isChatPath returns true for endpoints that should use per-session routing.
func isChatPath(path string) bool {
	return strings.Contains(path, "GetChatMessage") ||
		strings.Contains(path, "GetChatMessageBurst") ||
		strings.Contains(path, "GetCompletions")
}

// mayHaveConversationID returns true for paths that might carry a conversation_id.
// Chat paths + Cortex/Trajectory session lifecycle paths.
//
// 这是 MITM 是否注入号池身份的唯一总开关：
//   - true  → 解析 conv_id，按会话粘性绑定号池 key，替换 Authorization+body
//   - false → 完全透传 IDE 真实凭据(登录/状态/插件/心跳/工作流/档案等)
//
// 凡是不在 Chat/Cortex/Trajectory 范畴的路径都返回 false，由 handleRequest
// 直接 return 透传。曾经的 d322656 在 false 分支强行替换号池身份，导致 IDE
// 登录、用户状态查询、插件加载全部失效。
func mayHaveConversationID(path string) bool {
	return isChatPath(path) ||
		strings.Contains(path, "Cortex") ||
		strings.Contains(path, "Trajectory")
}

// sessionBindingCount returns how many active sessions are bound to the given key.
// Caller must hold sessionsMu (read or write).
func (p *MitmProxy) sessionBindingCount(poolKey string) int {
	count := 0
	for _, b := range p.sessionMap {
		if b.PoolKey == poolKey {
			count++
		}
	}
	return count
}

// leastConnectionsKey selects the healthy pool key with the fewest bound sessions.
// excludeKey is skipped (e.g. the exhausted key being rotated away from).
// Caller must hold p.mu (read).
func (p *MitmProxy) leastConnectionsKey(excludeKey string) string {
	globalBackoff := p.isGlobalTrialRateLimitActive()
	bestKey := ""
	bestCount := int(^uint(0) >> 1) // max int
	bestReqs := int(^uint(0) >> 1)
	var fallbackKey string // Trial key 备选（仅当无 Pro key 时使用）
	var fallbackCount, fallbackReqs int
	fallbackCount = int(^uint(0) >> 1)
	fallbackReqs = int(^uint(0) >> 1)

	candidateCount := 0
	for _, k := range p.poolKeys {
		if k == excludeKey {
			continue
		}
		state := p.keyStates[k]
		if state == nil || !state.isAvailable() {
			p.log("leastConn: key=%s... 不可用(state=%v avail=%v)",
				k[:minStr(12, len(k))], state != nil, state != nil && state.isAvailable())
			continue
		}
		p.jwtLock.RLock()
		hasJWT := len(state.JWT) > 0
		p.jwtLock.RUnlock()
		if !hasJWT {
			p.log("leastConn: key=%s... 无JWT，跳过", k[:minStr(12, len(k))])
			continue
		}

		candidateCount++
		count := p.sessionBindingCount(k)
		// 全局退避期间跳过 Trial/Free key，优先 Pro/Team
		if globalBackoff && p.isTrialOrFreeKey(k) {
			if count < fallbackCount || (count == fallbackCount && state.RequestCount < fallbackReqs) {
				fallbackKey = k
				fallbackCount = count
				fallbackReqs = state.RequestCount
			}
			continue
		}
		if count < bestCount || (count == bestCount && state.RequestCount < bestReqs) {
			bestKey = k
			bestCount = count
			bestReqs = state.RequestCount
		}
	}
	p.log("leastConn: 候选=%d 总=%d best=%s(sessions=%d) exclude=%s globalBackoff=%v",
		candidateCount, len(p.poolKeys),
		func() string {
			if bestKey == "" && fallbackKey == "" {
				return "无"
			}
			k := bestKey
			if k == "" {
				k = fallbackKey
			}
			return k[:minStr(12, len(k))] + "..."
		}(),
		func() int {
			if bestKey != "" {
				return bestCount
			}
			if fallbackKey != "" {
				return fallbackCount
			}
			return -1
		}(),
		func() string {
			if excludeKey == "" {
				return "无"
			}
			return excludeKey[:minStr(12, len(excludeKey))] + "..."
		}(),
		globalBackoff)
	if bestKey == "" {
		return fallbackKey // 无 Pro key 时降级到 Trial
	}
	return bestKey
}

// pickKeyForNewConversation 为首条消息（无 convID）选择 pool key。
// 使用 leastConnectionsKeyWithPending 在号池间均匀分配新对话，并将选用的 key 推入
// pendingNewConvKeys 队列，以便第二条消息（带 convID）能绑定到同一个 key。
//
// ★ ManualPin 例外：如果设置了 stickyKey 且健康可用，直接返回 sticky 而不走
// 负载均衡。语义：用户「手动切到 X」= 接下来的新对话也用 X，是明确意图。
func (p *MitmProxy) pickKeyForNewConversation() (string, []byte) {
	// ★ ManualPin: 用户明确意图优先级最高
	if stickyKey, stickyJWT := p.pickStickyKey(); stickyKey != "" {
		// 也推入 pending，让第二条消息（带 convID）能匹配回同一个 key
		p.pendingNewConvMu.Lock()
		p.pendingNewConvKeys = append(p.pendingNewConvKeys, pendingKeyEntry{PoolKey: stickyKey, At: time.Now()})
		if len(p.pendingNewConvKeys) > pendingNewConvMaxSize {
			p.pendingNewConvKeys = p.pendingNewConvKeys[len(p.pendingNewConvKeys)-pendingNewConvMaxSize:]
		}
		p.pendingNewConvMu.Unlock()
		p.log("新对话分配(sticky/ManualPin): key=%s...", stickyKey[:minStr(12, len(stickyKey))])
		return stickyKey, stickyJWT
	}

	// ★ 取锁顺序统一为 sessionsMu → pendingNewConvMu → p.mu,与 pickPoolKeyForSession
	// 完全一致,消除 AB-BA 死锁(后者持 sessionsMu 时会调 popPendingNewConvKey 拿
	// pendingNewConvMu;本函数过去先拿 pendingNewConvMu 再拿 sessionsMu,方向相反)。
	// 选 key + push pending 在两把锁内原子完成;JWT 获取(可能触发网络刷新)移到锁外,
	// 避免长时间占用 sessionsMu 阻塞所有会话请求。
	p.sessionsMu.RLock()
	p.pendingNewConvMu.Lock()

	cutoff := time.Now().Add(-pendingNewConvMaxAge)
	// 清理过期条目
	trimIdx := 0
	for i, e := range p.pendingNewConvKeys {
		if !e.At.Before(cutoff) {
			trimIdx = i
			break
		}
		if i == len(p.pendingNewConvKeys)-1 {
			trimIdx = len(p.pendingNewConvKeys) // 全部过期
		}
	}
	if trimIdx > 0 {
		p.pendingNewConvKeys = p.pendingNewConvKeys[trimIdx:]
	}
	pendingCounts := make(map[string]int)
	for _, e := range p.pendingNewConvKeys {
		pendingCounts[e.PoolKey]++
	}

	// 选最少连接 key（计入 pending 虚拟会话）。p.mu 是最内层锁。
	p.mu.RLock()
	key := p.leastConnectionsKeyWithPending("", pendingCounts)
	p.mu.RUnlock()

	if key == "" {
		p.pendingNewConvMu.Unlock()
		p.sessionsMu.RUnlock()
		return p.pickPoolKeyAndJWT()
	}

	// 原子 push pending（仍在 pendingNewConvMu 锁内）
	p.pendingNewConvKeys = append(p.pendingNewConvKeys, pendingKeyEntry{PoolKey: key, At: time.Now()})
	if len(p.pendingNewConvKeys) > pendingNewConvMaxSize {
		p.pendingNewConvKeys = p.pendingNewConvKeys[len(p.pendingNewConvKeys)-pendingNewConvMaxSize:]
	}
	savedPendingCount := pendingCounts[key] + 1
	p.pendingNewConvMu.Unlock()
	p.sessionsMu.RUnlock()

	// ── 锁外获取 JWT(可能触发网络刷新)──
	jwt := p.usableJWTForKey(key)
	if len(jwt) == 0 {
		// 该 key 暂无可用 JWT。上面 push 的 pending 条目无害(会过期,且第二条消息
		// 即便匹配到它也会因 JWT 缺失再迁移),直接回落全局选择。
		return p.pickPoolKeyAndJWT()
	}

	p.log("新对话分配(pending): key=%s... (pending=%d sessions=%d)",
		key[:minStr(12, len(key))], savedPendingCount, p.sessionBindingCountSafe(key))
	return key, jwt
}

// leastConnectionsKeyWithPending 与 leastConnectionsKey 类似，但额外考虑 pending 队列中的虚拟会话数。
// Caller must hold p.mu (read) AND sessionsMu (read).
func (p *MitmProxy) leastConnectionsKeyWithPending(excludeKey string, pendingCounts map[string]int) string {
	globalBackoff := p.isGlobalTrialRateLimitActive()
	bestKey := ""
	bestCount := int(^uint(0) >> 1) // max int
	bestReqs := int(^uint(0) >> 1)
	var fallbackKey string
	var fallbackCount, fallbackReqs int
	fallbackCount = int(^uint(0) >> 1)
	fallbackReqs = int(^uint(0) >> 1)

	for _, k := range p.poolKeys {
		if k == excludeKey {
			continue
		}
		state := p.keyStates[k]
		if state == nil || !state.isAvailable() {
			continue
		}
		p.jwtLock.RLock()
		hasJWT := len(state.JWT) > 0
		p.jwtLock.RUnlock()
		if !hasJWT {
			continue
		}

		count := p.sessionBindingCount(k) + pendingCounts[k]
		if globalBackoff && p.isTrialOrFreeKey(k) {
			if count < fallbackCount || (count == fallbackCount && state.RequestCount < fallbackReqs) {
				fallbackKey = k
				fallbackCount = count
				fallbackReqs = state.RequestCount
			}
			continue
		}
		if count < bestCount || (count == bestCount && state.RequestCount < bestReqs) {
			bestKey = k
			bestCount = count
			bestReqs = state.RequestCount
		}
	}
	if bestKey == "" {
		return fallbackKey
	}
	return bestKey
}

// sessionBindingCountSafe 线程安全版 sessionBindingCount（内部加锁 sessionsMu）。
func (p *MitmProxy) sessionBindingCountSafe(poolKey string) int {
	p.sessionsMu.RLock()
	defer p.sessionsMu.RUnlock()
	return p.sessionBindingCount(poolKey)
}

// popPendingNewConvKey 从 pending 队列弹出最旧的未过期条目（FIFO），返回 pool key。
// 返回 "" 表示队列为空或全部过期。
func (p *MitmProxy) popPendingNewConvKey() string {
	p.pendingNewConvMu.Lock()
	defer p.pendingNewConvMu.Unlock()

	cutoff := time.Now().Add(-pendingNewConvMaxAge)
	for len(p.pendingNewConvKeys) > 0 {
		entry := p.pendingNewConvKeys[0]
		p.pendingNewConvKeys = p.pendingNewConvKeys[1:]
		if !entry.At.Before(cutoff) {
			return entry.PoolKey
		}
	}
	return ""
}

// pickPoolKeyForSession selects a pool key for a specific conversation.
// - If convID is already bound and the bound key is healthy → sticky return
// - If convID is unbound or bound key is unavailable → least-connections assignment
// - excludeKeys: keys to avoid (e.g. a key that just caused cascade session failure)
// Returns (poolKey, jwt).
func (p *MitmProxy) pickPoolKeyForSession(convID string, excludeKeys ...string) (string, []byte) {
	p.sessionsMu.Lock()

	// Check existing binding
	if binding, ok := p.sessionMap[convID]; ok {
		// 如果绑定的 key 在排除列表中，跳过 sticky
		excluded := false
		for _, ek := range excludeKeys {
			if ek == binding.PoolKey {
				excluded = true
				break
			}
		}
		if !excluded {
			p.mu.RLock()
			state := p.keyStates[binding.PoolKey]
			available := state != nil && state.isAvailable()
			// ★ 会话粘性保护（仅限速短冷却）：
			// 限速冷却 ≤ 60s：保持粘性，等冷却结束（迁移会触发 Invalid Cascade session）。
			// 限速冷却 > 60s：迁移到新 key——服务端"Resets in: 16m / 30m"长冷却时，
			//   继续粘连只会让用户连续踩限速；接受一次 Cascade session 重建代价更优。
			// ★ 额度耗尽(RuntimeExhausted) 不保持粘性 — key 不会自动恢复，必须迁移。
			stickyOverride := false
			if !available && state != nil && !state.Disabled && !state.RuntimeExhausted {
				cooldownLeft := time.Until(state.CooldownUntil)
				if cooldownLeft > 0 && cooldownLeft <= 60*time.Second {
					stickyOverride = true
				}
			}
			p.mu.RUnlock()

			if available || stickyOverride {
				jwt := p.usableJWTForKey(binding.PoolKey)
				if len(jwt) > 0 {
					binding.LastSeenAt = time.Now()
					binding.RequestCount++
					p.sessionsMu.Unlock()
					if stickyOverride {
						p.log("会话 %s... 绑定的 key %s... 限速冷却中，保持粘性（避免迁移）",
							convID[:minStr(8, len(convID))],
							binding.PoolKey[:minStr(12, len(binding.PoolKey))])
					}
					return binding.PoolKey, jwt
				}
			}
		}
		// Bound key unavailable (额度耗尽/禁用) or excluded — need to migrate this session
		p.log("会话 %s... 绑定的 key %s... 不可用(耗尽/禁用)或被排除，重新分配",
			convID[:minStr(8, len(convID))],
			binding.PoolKey[:minStr(12, len(binding.PoolKey))])
	}

	// ★ 优先从 pending 队列弹出首条消息使用的 key（保证首/二条消息用同一 key）
	excludeKey := ""
	if len(excludeKeys) > 0 {
		excludeKey = excludeKeys[0]
	}
	newKey := ""
	if pendingKey := p.popPendingNewConvKey(); pendingKey != "" && pendingKey != excludeKey {
		p.mu.RLock()
		state := p.keyStates[pendingKey]
		usable := state != nil && (state.isAvailable() || !state.Disabled)
		p.mu.RUnlock()
		if usable {
			newKey = pendingKey
			p.log("新会话绑定(pending匹配): conv=%s... → key=%s...",
				convID[:minStr(8, len(convID))], pendingKey[:minStr(12, len(pendingKey))])
		}
	}

	// ★ ManualPin sticky 优先于 leastConnections fallback
	// 场景：第二条消息带 convID 但 pending 已过期 / 被排空 / IDE 重启后第一次消息
	// 直接带了 convID。这时仍应让 sticky 起作用，而不是被 leastConnections 抢走。
	if newKey == "" {
		if stickyKey, _ := p.pickStickyKey(); stickyKey != "" && stickyKey != excludeKey {
			newKey = stickyKey
			p.log("新会话绑定(sticky/ManualPin): conv=%s... → key=%s...",
				convID[:minStr(8, len(convID))], stickyKey[:minStr(12, len(stickyKey))])
		}
	}

	// Fallback: least-connections（迁移场景或 pending/sticky 都无匹配时）
	if newKey == "" {
		p.mu.RLock()
		newKey = p.leastConnectionsKey(excludeKey)
		p.mu.RUnlock()
	}

	// 如果排除后没找到，不排除再试一次
	if newKey == "" && excludeKey != "" {
		p.mu.RLock()
		newKey = p.leastConnectionsKey("")
		p.mu.RUnlock()
	}

	if newKey == "" {
		// Fallback to global selection
		p.sessionsMu.Unlock()
		return p.pickPoolKeyAndJWT()
	}

	jwt := p.usableJWTForKey(newKey)
	if len(jwt) == 0 {
		p.sessionsMu.Unlock()
		return p.pickPoolKeyAndJWT()
	}

	// Create or update binding
	if existing, ok := p.sessionMap[convID]; ok {
		oldKey := existing.PoolKey
		existing.PoolKey = newKey
		existing.Migrated = true
		existing.LastSeenAt = time.Now()
		existing.RequestCount++
		p.sessionsMu.Unlock()
		p.log("会话迁移: %s... conv=%s... → %s...",
			oldKey[:minStr(12, len(oldKey))], convID[:minStr(8, len(convID))],
			newKey[:minStr(12, len(newKey))])
		return newKey, jwt
	}

	p.sessionMap[convID] = &SessionBinding{
		ConversationID: convID,
		PoolKey:        newKey,
		BoundAt:        time.Now(),
		LastSeenAt:     time.Now(),
		RequestCount:   1,
	}

	// Evict oldest entries if over limit
	if len(p.sessionMap) > sessionMapMaxEntries {
		var oldestID string
		oldestTime := time.Now()
		for id, b := range p.sessionMap {
			if b.LastSeenAt.Before(oldestTime) {
				oldestTime = b.LastSeenAt
				oldestID = id
			}
		}
		if oldestID != "" {
			delete(p.sessionMap, oldestID)
		}
	}

	p.sessionsMu.Unlock()
	p.log("会话绑定: conv=%s... → key=%s... (绑定数=%d)",
		convID[:minStr(8, len(convID))], newKey[:minStr(12, len(newKey))],
		p.sessionBindingCount(newKey)+1)
	return newKey, jwt
}

// migrateSessionsFromKey moves all sessions bound to exhaustedKey to other healthy keys.
func (p *MitmProxy) migrateSessionsFromKey(exhaustedKey string) {
	p.sessionsMu.Lock()
	defer p.sessionsMu.Unlock()

	migrated := 0
	for convID, binding := range p.sessionMap {
		if binding.PoolKey != exhaustedKey {
			continue
		}
		p.mu.RLock()
		newKey := p.leastConnectionsKey(exhaustedKey)
		p.mu.RUnlock()
		if newKey != "" {
			binding.PoolKey = newKey
			binding.Migrated = true
			migrated++
			p.log("会话迁移: %s... → %s... (conv=%s...)",
				exhaustedKey[:minStr(12, len(exhaustedKey))],
				newKey[:minStr(12, len(newKey))],
				convID[:minStr(8, len(convID))])
		}
	}
	if migrated > 0 {
		p.log("完成会话迁移: %d 个会话从 %s... 迁出", migrated, exhaustedKey[:minStr(12, len(exhaustedKey))])
	}
}

// cleanExpiredSessions removes sessions that haven't been seen for sessionExpireMinutes.
func (p *MitmProxy) cleanExpiredSessions() {
	p.sessionsMu.Lock()
	defer p.sessionsMu.Unlock()

	cutoff := time.Now().Add(-time.Duration(sessionExpireMinutes) * time.Minute)
	removed := 0
	for id, b := range p.sessionMap {
		if b.LastSeenAt.Before(cutoff) {
			delete(p.sessionMap, id)
			removed++
		}
	}
	if removed > 0 {
		p.log("清理过期会话: %d 个 (剩余 %d)", removed, len(p.sessionMap))
	}
}

// GetSessionBindings returns a snapshot of active session bindings for the frontend.
func (p *MitmProxy) GetSessionBindings() []SessionBindingInfo {
	p.sessionsMu.RLock()
	defer p.sessionsMu.RUnlock()

	result := make([]SessionBindingInfo, 0, len(p.sessionMap))
	for _, b := range p.sessionMap {
		convShort := b.ConversationID
		if len(convShort) > 12 {
			convShort = convShort[:8] + "..." + convShort[len(convShort)-4:]
		}
		keyShort := b.PoolKey
		if len(keyShort) > 16 {
			keyShort = keyShort[:16] + "..."
		}
		result = append(result, SessionBindingInfo{
			ConvIDShort:  convShort,
			PoolKeyShort: keyShort,
			PoolKeyHash:  HashPoolKey(b.PoolKey),
			BoundAt:      b.BoundAt.Format(time.RFC3339),
			LastSeenAt:   b.LastSeenAt.Format(time.RFC3339),
			RequestCount: b.RequestCount,
			Title:        b.Title,
		})
	}
	return result
}

// UnbindSession removes a specific session binding by conversation_id prefix.
func (p *MitmProxy) UnbindSession(convIDPrefix string) bool {
	p.sessionsMu.Lock()
	defer p.sessionsMu.Unlock()

	for id := range p.sessionMap {
		if strings.HasPrefix(id, convIDPrefix) || id == convIDPrefix {
			delete(p.sessionMap, id)
			p.log("手动解绑会话: %s...", convIDPrefix[:minStr(8, len(convIDPrefix))])
			return true
		}
	}
	return false
}

// SessionCount returns the number of active session bindings.
func (p *MitmProxy) SessionCount() int {
	p.sessionsMu.RLock()
	defer p.sessionsMu.RUnlock()
	return len(p.sessionMap)
}

// ── JWT management ──

func (p *MitmProxy) updateJWT(apiKey string, jwt []byte) {
	p.mu.Lock()
	state := p.keyStates[apiKey]
	p.mu.Unlock()
	if state == nil {
		return
	}
	p.jwtLock.Lock()
	state.JWT = jwt
	state.JWTUpdatedAt = time.Now()
	p.jwtLock.Unlock()
}

func (p *MitmProxy) clearJWT(apiKey string) {
	p.mu.RLock()
	state := p.keyStates[apiKey]
	p.mu.RUnlock()
	if state == nil {
		return
	}
	p.jwtLock.Lock()
	state.JWT = nil
	state.JWTUpdatedAt = time.Time{}
	p.jwtLock.Unlock()
}

func (p *MitmProxy) jwtSnapshotForKey(apiKey string) ([]byte, time.Time) {
	p.mu.RLock()
	state := p.keyStates[apiKey]
	p.mu.RUnlock()
	if state == nil {
		return nil, time.Time{}
	}
	p.jwtLock.RLock()
	defer p.jwtLock.RUnlock()
	if len(state.JWT) == 0 {
		return nil, state.JWTUpdatedAt
	}
	jwt := make([]byte, len(state.JWT))
	copy(jwt, state.JWT)
	return jwt, state.JWTUpdatedAt
}

func (p *MitmProxy) jwtBytesForKey(apiKey string) []byte {
	jwt, _ := p.jwtSnapshotForKey(apiKey)
	return jwt
}

func (p *MitmProxy) jwtNeedsRefresh(apiKey string) bool {
	jwt, updatedAt := p.jwtSnapshotForKey(apiKey)
	if len(jwt) == 0 || updatedAt.IsZero() {
		return false
	}
	return time.Since(updatedAt) >= jwtRefreshMinutes*time.Minute
}

func (p *MitmProxy) usableJWTForKey(apiKey string) []byte {
	jwt, _ := p.jwtSnapshotForKey(apiKey)
	if len(jwt) == 0 {
		return p.ensureJWTForKey(apiKey)
	}
	if p.jwtNeedsRefresh(apiKey) {
		p.log("JWT 已过期，使用前强制刷新: %s...", apiKey[:minStr(12, len(apiKey))])
		if refreshed := p.refreshJWTForKey(apiKey); len(refreshed) > 0 {
			return refreshed
		}
		p.log("JWT 强制刷新失败，回退旧 JWT 使用: %s...", apiKey[:minStr(12, len(apiKey))])
	}
	return jwt
}

func cloneBytes(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	out := make([]byte, len(src))
	copy(out, src)
	return out
}

func (p *MitmProxy) keyIsDisabled(apiKey string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	state := p.keyStates[apiKey]
	return state != nil && state.Disabled
}

func (p *MitmProxy) markJWTReady() {
	p.jwtOnce.Do(func() {
		close(p.jwtReady)
	})
}

func (p *MitmProxy) beginJWTFetch(apiKey string, force bool) (*jwtFetchCall, []byte, bool) {
	p.jwtFetchMu.Lock()
	if call := p.jwtFetches[apiKey]; call != nil {
		p.jwtFetchMu.Unlock()
		return call, nil, false
	}
	if !force {
		if jwt := p.jwtBytesForKey(apiKey); len(jwt) > 0 {
			p.jwtFetchMu.Unlock()
			return nil, jwt, false
		}
	}
	call := &jwtFetchCall{done: make(chan struct{})}
	p.jwtFetches[apiKey] = call
	p.jwtFetchMu.Unlock()
	return call, nil, true
}

func (p *MitmProxy) finishJWTFetch(apiKey string, call *jwtFetchCall, jwt []byte, err error) {
	p.jwtFetchMu.Lock()
	call.jwt = cloneBytes(jwt)
	call.err = err
	delete(p.jwtFetches, apiKey)
	close(call.done)
	p.jwtFetchMu.Unlock()
}

func (p *MitmProxy) waitJWTFetch(call *jwtFetchCall) []byte {
	<-call.done
	return cloneBytes(call.jwt)
}

func (p *MitmProxy) fetchJWTForKey(apiKey string, force bool) []byte {
	if apiKey == "" || p.windsurfSvc == nil || !isJWTMintablePoolKey(apiKey) {
		return nil
	}
	if p.keyIsDisabled(apiKey) {
		return nil
	}
	call, cached, leader := p.beginJWTFetch(apiKey, force)
	if len(cached) > 0 {
		p.markJWTReady()
		return cached
	}
	if !leader {
		return p.waitJWTFetch(call)
	}
	if force {
		p.clearJWT(apiKey)
	}
	jwt, err := getJWTByAPIKeyFn(p.windsurfSvc, apiKey)
	if err != nil {
		p.finishJWTFetch(apiKey, call, nil, err)
		if isJWTAccessDeniedError(err) {
			p.markKeyDisabled(apiKey, err.Error())
		}
		if force {
			p.log("JWT 强制刷新失败: %s... (%v)", apiKey[:minStr(12, len(apiKey))], err)
		} else {
			p.log("JWT 按需获取失败: %s... (%v)", apiKey[:minStr(12, len(apiKey))], err)
		}
		return nil
	}
	out := []byte(jwt)
	p.updateJWT(apiKey, out)
	p.markJWTReady()
	p.finishJWTFetch(apiKey, call, out, nil)
	if force {
		p.log("JWT 强制刷新成功: %s... (%dB)", apiKey[:minStr(12, len(apiKey))], len(out))
	} else {
		p.log("JWT 按需获取成功: %s... (%dB)", apiKey[:minStr(12, len(apiKey))], len(out))
	}
	return cloneBytes(out)
}

func (p *MitmProxy) ensureJWTForKey(apiKey string) []byte {
	return p.fetchJWTForKey(apiKey, false)
}

func (p *MitmProxy) refreshJWTForKey(apiKey string) []byte {
	return p.fetchJWTForKey(apiKey, true)
}

func isJWTMintablePoolKey(apiKey string) bool {
	apiKey = strings.TrimSpace(apiKey)
	return strings.HasPrefix(apiKey, "sk-ws-") || strings.HasPrefix(apiKey, "devin-session-token$")
}

func isJWTAccessDeniedError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	if !strings.Contains(lower, "http 403") {
		return false
	}
	return strings.Contains(lower, "permission_denied") ||
		strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "forbidden") ||
		strings.Contains(lower, "user is disabled in windsurf team") ||
		strings.Contains(lower, "subscription is not active")
}

func isPersistentJWTAccessDeniedDetail(detail string) bool {
	lower := strings.ToLower(strings.TrimSpace(detail))
	if lower == "" {
		return false
	}
	if isRateLimitText(lower) {
		return false
	}
	return strings.Contains(lower, `"code":"permission_denied"`) ||
		strings.Contains(lower, "'code':'permission_denied'") ||
		strings.Contains(lower, "[permission_denied]") ||
		strings.Contains(lower, "permission_denied") ||
		strings.Contains(lower, "user is disabled in windsurf team") ||
		strings.Contains(lower, "subscription is not active") ||
		strings.Contains(lower, "failed to validate devin token") ||
		strings.Contains(lower, "try logging out and logging in")
}

func (p *MitmProxy) prefetchSpecificJWTs(keys []string, force bool) {
	if force {
		p.log("开始强制刷新 %d 个 key 的 JWT...", len(keys))
	} else {
		p.log("开始预取 %d 个 key 的 JWT...", len(keys))
	}
	// ★ 并行预取，限制并发数避免上游限流
	const maxConcurrent = 5
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	for _, key := range keys {
		if !force && len(p.jwtBytesForKey(key)) > 0 && !p.jwtNeedsRefresh(key) {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(k string) {
			defer wg.Done()
			defer func() { <-sem }()
			if force {
				_ = p.refreshJWTForKey(k)
			} else {
				_ = p.ensureJWTForKey(k)
			}
		}(key)
	}
	wg.Wait()
}

func (p *MitmProxy) prefetchJWTs() {
	keys := p.jwtRefreshKeys()
	if len(keys) == 0 {
		return
	}
	// ★ 先检查是否至少有一个 key 已有 JWT（标记 ready），再预取其余
	for _, k := range keys {
		if len(p.jwtBytesForKey(k)) > 0 {
			p.markJWTReady()
			break
		}
	}
	p.prefetchSpecificJWTs(keys, false)
	p.markJWTReady() // 预取完毕后确保标记 ready
}

func (p *MitmProxy) jwtRefreshKeys() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if len(p.poolKeys) == 0 {
		return nil
	}
	// ★ 返回所有可用(非耗尽/非禁用) key，确保每个 key 都有 JWT 以支持 session 分配
	var keys []string
	for _, k := range p.poolKeys {
		if k == "" {
			continue
		}
		state := p.keyStates[k]
		if state != nil && (state.RuntimeExhausted || state.Disabled) {
			continue
		}
		keys = append(keys, k)
	}
	return keys
}

func (p *MitmProxy) refreshJWTsOnce() {
	keys := p.jwtRefreshKeys()
	if len(keys) == 0 {
		return
	}
	p.prefetchSpecificJWTs(keys, true)
}

func (p *MitmProxy) jwtRefreshLoop() {
	jwtTicker := time.NewTicker(jwtRefreshMinutes * time.Minute)
	sessionTicker := time.NewTicker(5 * time.Minute) // 每 5 分钟清理过期会话
	defer jwtTicker.Stop()
	defer sessionTicker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-jwtTicker.C:
			p.log("定时刷新所有号池 key 的 JWT...")
			p.refreshJWTsOnce()
		case <-sessionTicker.C:
			p.cleanExpiredSessions()
		}
	}
}

// ── Helpers ──

func minStr(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func suffix3(s string) string {
	if len(s) < 6 {
		return ""
	}
	return s[len(s)-3:]
}

func (k upstreamFailureKind) logLabel() string {
	switch k {
	case upstreamFailureQuota:
		return "额度"
	case upstreamFailureRateLimit:
		return "限速"
	case upstreamFailureAuth:
		return "认证"
	case upstreamFailureInternal:
		return "内部"
	case upstreamFailurePermission:
		return "权限"
	case upstreamFailureGRPC:
		return "gRPC"
	case upstreamFailureGlobalRateLimit:
		return "全局限速"
	case upstreamFailureUnavailable:
		return "上游不可达"
	default:
		return "未知"
	}
}

func decodeGRPCMessage(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if decoded, err := url.QueryUnescape(raw); err == nil && decoded != "" {
		return decoded
	}
	return raw
}

func isCascadeRetryPath(path string) bool {
	// 所有可能携带 conversation_id 的 Connect 端点都应允许重试
	// 不仅仅是 GetChatMessage/GetCompletions，还有 GetChatContext 等
	path = strings.ToLower(strings.TrimSpace(path))
	return strings.Contains(path, "windsurf.ai_codeium_windsurf_service") ||
		strings.Contains(path, "getchatmessage") ||
		strings.Contains(path, "getcompletions") ||
		strings.Contains(path, "getchatcontext") ||
		strings.Contains(path, "acceptcompletion") ||
		strings.Contains(path, "getprocesses") ||
		strings.Contains(path, "chatcontext") ||
		strings.Contains(path, "language_server_service") ||
		strings.Contains(path, "seat_management_service")
}

func isInvalidCascadeSessionText(textLower string) bool {
	return strings.Contains(textLower, "invalid cascade session") ||
		((strings.Contains(textLower, "failed_precondition") || strings.Contains(textLower, "failed precondition")) &&
			strings.Contains(textLower, "cascade session"))
}

func isCascadeSessionFailure(grpcStatus, grpcMessage, bodyText string) bool {
	status := strings.TrimSpace(grpcStatus)
	msg := strings.ToLower(decodeGRPCMessage(grpcMessage))
	body := strings.ToLower(strings.TrimSpace(bodyText))
	combined := strings.TrimSpace(body + "\n" + msg)
	if !isInvalidCascadeSessionText(combined) {
		return false
	}
	return status == "" || status == "9" || status == "13"
}

func safeUsedKeyForLog(apiKey string) string {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "<no-key>"
	}
	return apiKey[:minStr(12, len(apiKey))] + "..."
}

func classifyUpstreamFailure(grpcStatus, grpcMessage, bodyText string) (upstreamFailureKind, string) {
	status := strings.TrimSpace(grpcStatus)
	msg := decodeGRPCMessage(grpcMessage)
	msgLower := strings.ToLower(msg)
	bodyLower := strings.ToLower(bodyText)
	combined := strings.TrimSpace(bodyLower + "\n" + msgLower)

	if isRateLimitText(combined) {
		return upstreamFailureRateLimit, formatUpstreamFailureDetail(status, msg, bodyText)
	}
	// gRPC 8=RESOURCE_EXHAUSTED, 9=FAILED_PRECONDITION (Windsurf 额度耗尽常返回 9)
	if status == "8" || isQuotaExhaustedText(combined) {
		return upstreamFailureQuota, formatUpstreamFailureDetail(status, msg, bodyText)
	}
	if status == "9" && (strings.Contains(combined, "quota") || strings.Contains(combined, "usage") || strings.Contains(combined, "credits")) {
		return upstreamFailureQuota, formatUpstreamFailureDetail(status, msg, bodyText)
	}
	if status == "16" || strings.Contains(combined, "unauthenticated") || strings.Contains(combined, "authentication credentials") {
		return upstreamFailureAuth, formatUpstreamFailureDetail(status, msg, bodyText)
	}
	if strings.Contains(combined, `"code":"permission_denied"`) ||
		strings.Contains(combined, "'code':'permission_denied'") ||
		strings.Contains(combined, "[permission_denied]") ||
		strings.Contains(combined, "api server wire error: permission denied") ||
		strings.Contains(combined, "permission_denied") {
		return upstreamFailureAuth, formatUpstreamFailureDetail(status, msg, bodyText)
	}
	if status == "14" || strings.Contains(combined, "provider unreachable") || strings.Contains(combined, "model provider") {
		return upstreamFailureUnavailable, formatUpstreamFailureDetail(status, msg, bodyText)
	}
	if status == "13" || strings.Contains(combined, "internal server error") || strings.Contains(combined, "error number 13") {
		return upstreamFailureInternal, formatUpstreamFailureDetail(status, msg, bodyText)
	}
	if status == "7" || strings.Contains(combined, "permission denied") || strings.Contains(combined, "unauthorized") || strings.Contains(combined, "forbidden") {
		return upstreamFailurePermission, formatUpstreamFailureDetail(status, msg, bodyText)
	}
	if status != "" && status != "0" {
		return upstreamFailureGRPC, formatUpstreamFailureDetail(status, msg, bodyText)
	}
	return upstreamFailureNone, ""
}

func formatUpstreamFailureDetail(grpcStatus, grpcMessage, bodyText string) string {
	parts := make([]string, 0, 3)
	if s := strings.TrimSpace(grpcStatus); s != "" {
		parts = append(parts, "grpc-status="+s)
	}
	if s := strings.TrimSpace(grpcMessage); s != "" {
		parts = append(parts, "grpc-message="+truncate(s, 140))
	}
	body := strings.TrimSpace(bodyText)
	if body != "" {
		parts = append(parts, "body="+truncate(body, 180))
	}
	if len(parts) == 0 {
		return "无上游细节"
	}
	return strings.Join(parts, " ")
}

func isQuotaExhaustedText(textLower string) bool {
	patterns := []string{
		"resource_exhausted",
		"resource exhausted",
		"not enough credits",
		"daily usage quota has been exhausted",
		"weekly usage quota has been exhausted",
		"usage quota has been exhausted",
		"usage quota is exhausted",
		"included usage quota is exhausted",
		"quota has been exhausted",
		"quota is exhausted",
		"quota exhausted",
		"daily_quota_exhausted",
		"weekly_quota_exhausted",
		"purchase extra usage",
	}
	for _, pat := range patterns {
		if strings.Contains(textLower, pat) {
			return true
		}
	}
	return (strings.Contains(textLower, "failed_precondition") || strings.Contains(textLower, "failed precondition")) &&
		(strings.Contains(textLower, "quota") || strings.Contains(textLower, "usage") || strings.Contains(textLower, "credits"))
}

func isRateLimitText(textLower string) bool {
	patterns := []string{
		"rate limit exceeded",
		"rate limit error",
		"rate limit",
		"rate_limit",
		"global rate limit",
		"over their global rate limit",
		"all api providers are over",
		"message limit",
		"limit will reset",
		"too many requests",
		"try again in about an hour",
		"upgrade to pro for higher limits",
		"higher limits",
		"add-credits",
		"no credits were used",
	}
	for _, pat := range patterns {
		if strings.Contains(textLower, pat) {
			return true
		}
	}
	return false
}
