package services

// clash_auto.go ── ClashRotator 自动配置 + 真节点识别。
//
// 解决两个长期被用户踩到的痛点：
//   1. 不知道该把哪个 group 名填进设置 —— 机场配置常有 4-5 个 selector，
//      用户挑错（比如挑 GLOBAL 或层叠到子组的 group）→ 切换不生效。
//   2. 节点白名单要不要填、填什么 —— 机场把"剩余流量：xx GB"、"套餐到期"、
//      "防失联"等伪节点也注入到 selector.all 里，rotator 撞上这些会失败。
//
// 解法：用 Clash /proxies 返回的 type 字段来精确分类。
//   - "selector" / "fallback" / "urltest" / "loadbalance" → 是组(group)
//   - "direct" / "reject"                                 → 特殊出口
//   - 其他("vmess" / "trojan" / "shadowsocks" / "ss" /
//     "hysteria" / "hysteria2" / "wireguard" / "snell" / ...) → 真节点
//
// 这样不依赖关键字 heuristic，直接用 type 判断，比"看名字猜"健壮得多。

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// proxyKindMap pulls Clash /proxies once and returns name → type (lower-cased).
// 用作 filterCandidates 和 AutoDetectClashGroup 的事实来源。
func proxyKindMap(controllerURL, secret string) (map[string]string, error) {
	tmp := &ClashRotator{
		cfg:   ClashRotatorConfig{ControllerURL: controllerURL, Secret: secret},
		httpc: &http.Client{Timeout: 5 * time.Second},
	}
	var resp clashAllProxiesResp
	if err := tmp.doJSON(http.MethodGet, "/proxies", nil, &resp); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(resp.Proxies))
	for name, p := range resp.Proxies {
		out[name] = strings.ToLower(strings.TrimSpace(p.Type))
	}
	return out, nil
}

// isGroupKind 是否是组类型（selector/fallback/urltest/loadbalance）。
// 切换 selector 的下一个节点时必须排除这些 —— 切到 fallback 子组只会层叠，
// IP 不变；切到 urltest 子组会被它内部按延迟覆盖回去。
func isGroupKind(kind string) bool {
	switch kind {
	case "selector", "fallback", "urltest", "loadbalance", "url-test", "load-balance":
		return true
	}
	return false
}

// isRealNodeKind 是否是真出口节点（vmess/ss/trojan/...）。
//
// 列表为穷举不存在 —— Clash/Mihomo 持续支持新协议。所以反向判断：
// 不是组、不是 direct/reject/compatible，就当作真节点处理。
// 这样 wireguard / hysteria2 / tuic / juicity 等新增类型都能自动覆盖。
func isRealNodeKind(kind string) bool {
	if isGroupKind(kind) {
		return false
	}
	switch kind {
	case "", "direct", "reject", "compatible", "pass":
		return false
	}
	return true
}

// 机场常见的「伪节点」名字关键词。
// 实测某些机场会把"剩余流量：xx GB"、"套餐到期：2026-xx-xx"、"防失联"、
// 官网域名这种条目以 type=vmess/trojan 注入到订阅，让 Clash 客户端看起来
// 像普通节点（用作"广告位"或"客服联系方式"）。Rotator 切到这些会假成功
// (group.now 改变但实际无出口)。type 过滤抓不到，只能用名字 heuristic。
var fakeNodeKeywords = []string{
	"流量", "套餐", "到期", "过期", "续费", "重置",
	"防失联", "无痕", "客服", "联系", "tg", "telegram",
	"官网", "公告", "通知", "更新", "失效",
	"剩余", "expire", "expired", "traffic",
}

// isFakeNodeName 名字含上述关键词，或形如域名（含 "." 但没空格的纯 ASCII），
// 视作伪节点。
func isFakeNodeName(name string) bool {
	low := strings.ToLower(strings.TrimSpace(name))
	if low == "" {
		return true
	}
	for _, kw := range fakeNodeKeywords {
		if strings.Contains(low, kw) {
			return true
		}
	}
	// 形如 "zhuiyun.one" / "example.com" 这种纯域名条目（机场广告位）
	if strings.Contains(low, ".") && !strings.ContainsAny(low, " 【】[]()") {
		dotPart := strings.SplitN(low, ".", 2)[1]
		if len(dotPart) >= 2 && len(dotPart) <= 8 {
			return true // 顶级域名长度通常 2-8，过滤之
		}
	}
	return false
}

// AutoDetectClashGroupResult ……
type AutoDetectClashGroupResult struct {
	OK         bool     `json:"ok"`
	Error      string   `json:"error,omitempty"`
	Group      string   `json:"group"`           // 推荐的 selector 组名
	NodeCount  int      `json:"node_count"`      // 真节点数（已过滤伪节点和子组）
	Candidates []string `json:"candidates"`      // 真节点列表（已排序）
	AllGroups  []string `json:"all_groups"`      // 所有 selector 组名，供用户手动挑
}

// AutoDetectClashGroup 智能挑选最适合做 IP 轮换的 selector group。
//
// 算法：
//   1. 拉 /proxies 拿全部条目和 type
//   2. 收集所有 type=selector 的组（fallback/urltest 不能 PUT，跳过）
//   3. 排除 GLOBAL —— 它一切，连 DIRECT 都会被切到
//   4. 对每个候选 selector 计算 group.all 中"真节点"数量（剔除伪条目和子组）
//   5. 选真节点数最多的那个；并列时取名字字典序最小（稳定）
//
// 这是用户「一键智能启用」的事实依据。
func AutoDetectClashGroup(controllerURL, secret string) AutoDetectClashGroupResult {
	kinds, err := proxyKindMap(controllerURL, secret)
	if err != nil {
		return AutoDetectClashGroupResult{Error: err.Error()}
	}

	tmp := &ClashRotator{
		cfg:   ClashRotatorConfig{ControllerURL: controllerURL, Secret: secret},
		httpc: &http.Client{Timeout: 5 * time.Second},
	}

	type scored struct {
		name       string
		realNodes  []string
	}
	var scoredGroups []scored
	allSelectors := make([]string, 0)
	for name, kind := range kinds {
		if kind != "selector" {
			continue
		}
		if name == "GLOBAL" {
			continue
		}
		allSelectors = append(allSelectors, name)
		all, _, err := tmp.listGroupNodes(name)
		if err != nil {
			continue
		}
		var realNodes []string
		for _, member := range all {
			if !isRealNodeKind(kinds[member]) {
				continue
			}
			if isFakeNodeName(member) {
				continue
			}
			realNodes = append(realNodes, member)
		}
		scoredGroups = append(scoredGroups, scored{name, realNodes})
	}

	sort.Strings(allSelectors)

	if len(scoredGroups) == 0 {
		return AutoDetectClashGroupResult{
			Error:     "未发现可用的 selector 组（你的 Clash 配置里全部组都是 fallback/urltest 类型，无法手动切换）",
			AllGroups: allSelectors,
		}
	}

	// 节点数降序；并列按名字字典序
	sort.Slice(scoredGroups, func(i, j int) bool {
		if len(scoredGroups[i].realNodes) != len(scoredGroups[j].realNodes) {
			return len(scoredGroups[i].realNodes) > len(scoredGroups[j].realNodes)
		}
		return scoredGroups[i].name < scoredGroups[j].name
	})

	best := scoredGroups[0]
	if len(best.realNodes) == 0 {
		return AutoDetectClashGroupResult{
			Error: fmt.Sprintf("找到 selector 组 %q 但内部没有真节点（全部是子组或伪条目）", best.name),
			Group: best.name, AllGroups: allSelectors,
		}
	}

	candidates := append([]string(nil), best.realNodes...)
	sort.Strings(candidates)
	return AutoDetectClashGroupResult{
		OK:         true,
		Group:      best.name,
		NodeCount:  len(best.realNodes),
		Candidates: candidates,
		AllGroups:  allSelectors,
	}
}
