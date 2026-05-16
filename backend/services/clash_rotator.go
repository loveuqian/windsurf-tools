// Package services - clash_rotator.go
//
// ClashRotator: 周期性 + 限速触发地通过 Clash/Mihomo external-controller
// REST API 切换 selector 组当前节点（换出口 IP），用于绕过 windsurf 上游
// per-IP 限速。MITM 自身的出站代理 (proxyURL) 仍指向 Clash 入口；
// 此模块不动 transport，只 PUT /proxies/{group}。
package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ClashRotatorConfig 由 Settings 映射而来。
type ClashRotatorConfig struct {
	ControllerURL    string        // 例: http://127.0.0.1:9097
	Secret           string        // 可空
	Group            string        // selector 组名
	Whitelist        []string      // 节点白名单；空表示用组内所有节点
	Interval         time.Duration // 轮换间隔，最低 2 分钟
	RotateOnRL       bool          // 检测到 rate-limit 时立即切
	LatencyTestURL   string
	LatencyMaxMs     int           // <=0 表示跳过测延迟
	IdleWaitMax      time.Duration // 切换前等待 chat 流空闲的最长时间
	RateLimitMinGap  time.Duration // 两次限速触发的最短间隔（debounce）
}

// ProxyDelayQuerier 抽象 in-flight 计数 + 关闭空闲连接，便于测试。
type ProxyDelayQuerier interface {
	InFlightChatStreams() int64
	CloseUpstreamIdleConnections()
}

// ClashRotator 切节点循环。
type ClashRotator struct {
	cfg       ClashRotatorConfig
	proxy     ProxyDelayQuerier
	logFn     func(string)
	httpc     *http.Client

	mu        sync.Mutex
	running   bool
	stopCh    chan struct{}
	triggerCh chan string
	lastIdx   int
	lastRLAt  time.Time
	lastNode  string
	// originalNode 在 Start 时捕获 selector 组的当前节点，Stop 时用于恢复。
	// 空串 = 未捕获到（探活失败 / Stop 也跳过恢复）。
	// 行为目标：用户启用 IP 轮换 → 关闭软件 / 关掉开关后，组应回到启动时的状态。
	originalNode string

	// metrics（无锁原子）
	rotations uint64
	failures  uint64
}

// NewClashRotator 创建实例（不启动）。
func NewClashRotator(cfg ClashRotatorConfig, proxy ProxyDelayQuerier, logFn func(string)) *ClashRotator {
	if cfg.Interval <= 0 {
		cfg.Interval = 8 * time.Minute
	}
	if cfg.Interval < 2*time.Minute {
		cfg.Interval = 2 * time.Minute
	}
	if cfg.IdleWaitMax <= 0 {
		cfg.IdleWaitMax = 30 * time.Second
	}
	if cfg.RateLimitMinGap <= 0 {
		cfg.RateLimitMinGap = 30 * time.Second
	}
	if cfg.LatencyTestURL == "" {
		cfg.LatencyTestURL = "http://www.gstatic.com/generate_204"
	}
	return &ClashRotator{
		cfg:   cfg,
		proxy: proxy,
		logFn: logFn,
		httpc: &http.Client{Timeout: 10 * time.Second},
	}
}

func (r *ClashRotator) log(format string, args ...interface{}) {
	if r.logFn != nil {
		r.logFn("[Clash] " + fmt.Sprintf(format, args...))
	}
}

// Config 返回当前配置（只读副本）。
func (r *ClashRotator) Config() ClashRotatorConfig { return r.cfg }

// Stats 返回运行统计。
func (r *ClashRotator) Stats() (rotations, failures uint64, lastNode string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return atomic.LoadUint64(&r.rotations), atomic.LoadUint64(&r.failures), r.lastNode
}

// Start 启动后台循环。重复调用安全。
//
// 副作用：捕获当前 selector 组的「原始节点」存到 r.originalNode，
//
//	Stop 时用于把组恢复回启动前的状态。
//	拉取失败时只 log，不阻塞启动；Stop 也会因 originalNode 为空跳过恢复。
func (r *ClashRotator) Start() {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	r.running = true
	r.stopCh = make(chan struct{})
	r.triggerCh = make(chan string, 4)
	r.mu.Unlock()

	// 捕获原始节点（best-effort）。在主线程同步拉一次，避免与 loop 内首次切换竞争。
	if r.cfg.Group != "" {
		if _, current, err := r.listGroupNodes(r.cfg.Group); err == nil && current != "" {
			r.mu.Lock()
			r.originalNode = current
			r.mu.Unlock()
			r.log("已记录原始节点: %s（关闭轮换时将恢复）", current)
		} else if err != nil {
			r.log("记录原始节点失败（关闭时无法恢复）: %v", err)
		}
	}

	go r.loop()
	r.log("已启动: controller=%s group=%q interval=%s rl_trigger=%v",
		r.cfg.ControllerURL, r.cfg.Group, r.cfg.Interval, r.cfg.RotateOnRL)
}

// Stop 停止循环并把 selector 组恢复到 Start 时的原始节点。
//
//	恢复是 best-effort：失败 log 不抛错；originalNode 为空（Start 时拉取失败）
//	或与当前节点已经一致时跳过 PUT。
func (r *ClashRotator) Stop() {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return
	}
	r.running = false
	close(r.stopCh)
	original := r.originalNode
	group := r.cfg.Group
	r.originalNode = ""
	r.mu.Unlock()

	// 恢复原始节点（best-effort）。
	if original != "" && group != "" {
		// 拉一次当前节点，已经一致就跳过 PUT。
		if _, current, err := r.listGroupNodes(group); err == nil && current == original {
			r.log("已停止；当前节点与原始节点一致 (%s)，无需恢复", original)
		} else if err := r.switchTo(group, original); err != nil {
			r.log("已停止；恢复原始节点失败 %s: %v", original, err)
		} else {
			r.log("已停止；✓ 已恢复原始节点: %s", original)
		}
		return
	}
	r.log("已停止")
}

// TriggerRotate 非阻塞投递一次立即切换请求。
func (r *ClashRotator) TriggerRotate(reason string) {
	if !r.cfg.RotateOnRL && reason != "manual" && reason != "interval" {
		return
	}
	// debounce 限速触发
	if strings.HasPrefix(reason, "rate_limit") {
		r.mu.Lock()
		if time.Since(r.lastRLAt) < r.cfg.RateLimitMinGap {
			r.mu.Unlock()
			return
		}
		r.lastRLAt = time.Now()
		r.mu.Unlock()
	}
	r.mu.Lock()
	ch := r.triggerCh
	r.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- reason:
	default:
	}
}

func (r *ClashRotator) loop() {
	r.mu.Lock()
	stopCh := r.stopCh
	triggerCh := r.triggerCh
	r.mu.Unlock()

	ticker := time.NewTicker(r.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			r.rotateOnce("interval")
		case reason := <-triggerCh:
			r.rotateOnce(reason)
		}
	}
}

func (r *ClashRotator) rotateOnce(reason string) {
	if r.cfg.Group == "" {
		r.log("跳过: group 为空")
		return
	}

	nodes, current, err := r.listGroupNodes(r.cfg.Group)
	if err != nil {
		atomic.AddUint64(&r.failures, 1)
		r.log("拉取节点失败: %v", err)
		return
	}
	// 拉一次全量 proxy type 表，用于过滤掉子组(selector/fallback/urltest)
	// 和伪条目(direct/reject)。失败时降级走纯名字过滤，保证基本可用。
	kinds, _ := proxyKindMap(r.cfg.ControllerURL, r.cfg.Secret)
	candidates := r.filterCandidatesWithKinds(nodes, current, kinds)
	if len(candidates) == 0 {
		r.log("无可用候选节点 (group=%q nodes=%d whitelist=%d)",
			r.cfg.Group, len(nodes), len(r.cfg.Whitelist))
		return
	}

	if r.cfg.LatencyMaxMs > 0 {
		filtered := r.filterByLatency(candidates)
		if len(filtered) > 0 {
			candidates = filtered
		} else {
			r.log("延迟过滤后无可用节点，回退至所有候选")
		}
	}

	next := r.selectNext(candidates, current)
	if next == "" || next == current {
		r.log("已是目标节点，跳过 (current=%s reason=%s)", current, reason)
		return
	}

	// 等空闲（避免切瞬间断流）
	r.waitIdleOrTimeout()

	if err := r.switchTo(r.cfg.Group, next); err != nil {
		atomic.AddUint64(&r.failures, 1)
		r.log("切换失败 %s -> %s: %v", current, next, err)
		return
	}
	atomic.AddUint64(&r.rotations, 1)
	r.mu.Lock()
	r.lastNode = next
	r.mu.Unlock()
	r.log("✓ 节点切换: %s -> %s (reason=%s)", current, next, reason)

	// 强制重建 HTTP/2 连接，使后续请求走新出口
	if r.proxy != nil {
		r.proxy.CloseUpstreamIdleConnections()
	}
}

func (r *ClashRotator) waitIdleOrTimeout() {
	if r.proxy == nil || r.cfg.IdleWaitMax <= 0 {
		return
	}
	deadline := time.Now().Add(r.cfg.IdleWaitMax)
	for time.Now().Before(deadline) {
		if r.proxy.InFlightChatStreams() == 0 {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	r.log("等待空闲超时 (%s)，仍有 %d 个进行中流，强切",
		r.cfg.IdleWaitMax, r.proxy.InFlightChatStreams())
}

// filterCandidatesWithKinds 是 type-aware 版本：用 Clash /proxies 返回的 type
// 字段精确剔除子组(selector/fallback/urltest)和伪条目(direct/reject)。
//
// 当 kinds 为 nil（探活失败时降级路径），退回到 filterCandidates。
//
// 历史 bug：之前 rotator 只看名字，机场把"剩余流量：xx GB"、"套餐到期"、
// "防失联"等条目以普通节点形式注入到 selector.all，rotator 切到这些条目
// 时 PUT 成功(group.now 变了)，但下次请求依然走原节点 —— Clash 把这些伪
// 节点视作 reject 类型。type-aware 过滤直接从源头排除。
func (r *ClashRotator) filterCandidatesWithKinds(all []string, current string, kinds map[string]string) []string {
	if kinds == nil {
		return r.filterCandidates(all, current)
	}
	whitelist := map[string]struct{}{}
	for _, n := range r.cfg.Whitelist {
		n = strings.TrimSpace(n)
		if n != "" {
			whitelist[n] = struct{}{}
		}
	}
	out := make([]string, 0, len(all))
	for _, n := range all {
		if n == current || n == r.cfg.Group {
			continue
		}
		// 关键：用 type 而非名字判断
		if !isRealNodeKind(kinds[n]) {
			continue
		}
		// 兜底：名字 heuristic 抓那些被机场伪装成 vmess 的"流量/套餐/广告"条目
		if isFakeNodeName(n) {
			continue
		}
		if len(whitelist) > 0 {
			if _, ok := whitelist[n]; !ok {
				continue
			}
		}
		out = append(out, n)
	}
	return out
}

// filterCandidates 应用白名单 + 排除 group 自身/特殊节点 + 排除 current。
func (r *ClashRotator) filterCandidates(all []string, current string) []string {
	whitelist := map[string]struct{}{}
	for _, n := range r.cfg.Whitelist {
		n = strings.TrimSpace(n)
		if n != "" {
			whitelist[n] = struct{}{}
		}
	}
	excluded := map[string]struct{}{
		"DIRECT": {}, "REJECT": {}, "GLOBAL": {},
		r.cfg.Group: {},
	}
	out := make([]string, 0, len(all))
	for _, n := range all {
		if _, skip := excluded[n]; skip {
			continue
		}
		if n == current {
			continue
		}
		if len(whitelist) > 0 {
			if _, ok := whitelist[n]; !ok {
				continue
			}
		}
		out = append(out, n)
	}
	return out
}

func (r *ClashRotator) filterByLatency(nodes []string) []string {
	type res struct {
		name  string
		delay int
	}
	results := make([]res, 0, len(nodes))
	var wg sync.WaitGroup
	mu := sync.Mutex{}
	sem := make(chan struct{}, 8)
	for _, n := range nodes {
		wg.Add(1)
		sem <- struct{}{}
		go func(node string) {
			defer wg.Done()
			defer func() { <-sem }()
			d, err := r.testDelay(node)
			if err != nil {
				return
			}
			mu.Lock()
			results = append(results, res{node, d})
			mu.Unlock()
		}(n)
	}
	wg.Wait()
	max := r.cfg.LatencyMaxMs
	keep := make([]string, 0, len(results))
	for _, x := range results {
		if x.delay > 0 && x.delay <= max {
			keep = append(keep, x.name)
		}
	}
	sort.Strings(keep) // 稳定输入用于 round-robin
	return keep
}

func (r *ClashRotator) selectNext(candidates []string, current string) string {
	if len(candidates) == 0 {
		return ""
	}
	if len(candidates) == 1 {
		return candidates[0]
	}
	// 优先尝试从 lastIdx 开始的 round-robin
	r.mu.Lock()
	idx := (r.lastIdx + 1) % len(candidates)
	for i := 0; i < len(candidates); i++ {
		cand := candidates[(idx+i)%len(candidates)]
		if cand != current {
			r.lastIdx = (idx + i) % len(candidates)
			r.mu.Unlock()
			return cand
		}
	}
	// 兜底随机（理论上 candidates 已排除 current）
	pick := candidates[rand.Intn(len(candidates))]
	r.mu.Unlock()
	return pick
}

// ── REST helpers ──

func (r *ClashRotator) doJSON(method, path string, body interface{}, out interface{}) error {
	base := strings.TrimRight(r.cfg.ControllerURL, "/")
	if base == "" {
		return fmt.Errorf("controller URL 为空")
	}
	full := base + path

	var rdr *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	var req *http.Request
	var err error
	if rdr != nil {
		req, err = http.NewRequestWithContext(context.Background(), method, full, rdr)
	} else {
		req, err = http.NewRequestWithContext(context.Background(), method, full, nil)
	}
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if r.cfg.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+r.cfg.Secret)
	}
	resp, err := r.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if out != nil {
		dec := json.NewDecoder(resp.Body)
		return dec.Decode(out)
	}
	return nil
}

type clashGroupResp struct {
	Now  string   `json:"now"`
	All  []string `json:"all"`
	Type string   `json:"type"`
	Name string   `json:"name"`
}

type clashAllProxiesResp struct {
	Proxies map[string]clashGroupResp `json:"proxies"`
}

type clashDelayResp struct {
	Delay int    `json:"delay"`
	Msg   string `json:"message"`
}

func (r *ClashRotator) listGroupNodes(group string) ([]string, string, error) {
	var g clashGroupResp
	if err := r.doJSON(http.MethodGet, "/proxies/"+url.PathEscape(group), nil, &g); err != nil {
		return nil, "", err
	}
	return g.All, g.Now, nil
}

func (r *ClashRotator) switchTo(group, node string) error {
	body := map[string]string{"name": node}
	return r.doJSON(http.MethodPut, "/proxies/"+url.PathEscape(group), body, nil)
}

func (r *ClashRotator) testDelay(node string) (int, error) {
	q := url.Values{}
	q.Set("url", r.cfg.LatencyTestURL)
	q.Set("timeout", "3000")
	var d clashDelayResp
	err := r.doJSON(http.MethodGet, "/proxies/"+url.PathEscape(node)+"/delay?"+q.Encode(), nil, &d)
	if err != nil {
		return 0, err
	}
	if d.Msg != "" {
		return 0, fmt.Errorf(d.Msg)
	}
	return d.Delay, nil
}

// ── 探活/列举（供 UI 使用） ──

// ProbeController 测试 controller 联通性 + 列出 selector 组。
type ClashProbeResult struct {
	OK     bool     `json:"ok"`
	Error  string   `json:"error,omitempty"`
	Groups []string `json:"groups"`
}

// ProbeController 不依赖 r.cfg，仅用 controllerURL/secret 做探活；
// 用作 UI「测试连接」按钮。
func ProbeClashController(controllerURL, secret string) ClashProbeResult {
	tmp := &ClashRotator{
		cfg:   ClashRotatorConfig{ControllerURL: controllerURL, Secret: secret},
		httpc: &http.Client{Timeout: 5 * time.Second},
	}
	var resp clashAllProxiesResp
	if err := tmp.doJSON(http.MethodGet, "/proxies", nil, &resp); err != nil {
		return ClashProbeResult{OK: false, Error: err.Error()}
	}
	groups := make([]string, 0)
	for name, g := range resp.Proxies {
		t := strings.ToLower(g.Type)
		if t == "selector" || t == "fallback" {
			groups = append(groups, name)
		}
	}
	sort.Strings(groups)
	return ClashProbeResult{OK: true, Groups: groups}
}

// ListClashGroupNodes 列出指定组内的节点列表（用于 UI 多选白名单）。
// 留空组名时默认 GLOBAL（与 UI 提示保持一致）。
func ListClashGroupNodes(controllerURL, secret, group string) ([]string, error) {
	if strings.TrimSpace(group) == "" {
		group = "GLOBAL"
	}
	tmp := &ClashRotator{
		cfg:   ClashRotatorConfig{ControllerURL: controllerURL, Secret: secret},
		httpc: &http.Client{Timeout: 5 * time.Second},
	}
	nodes, _, err := tmp.listGroupNodes(group)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(nodes))
	excluded := map[string]struct{}{"DIRECT": {}, "REJECT": {}, "GLOBAL": {}, group: {}}
	for _, n := range nodes {
		if _, skip := excluded[n]; skip {
			continue
		}
		out = append(out, n)
	}
	return out, nil
}
