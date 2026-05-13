package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockClashAllProxiesServer 模拟一个真实机场的复杂配置：
//   - 顶层 selector "PROXY_TOP" 含 30 个真节点
//   - 顶层 selector "GLOBAL"   含一切（应被排除）
//   - 子组 selector "AUTO_SUB" 仅 5 个真节点（不该被选中）
//   - fallback 组 "FB"          不可手动 PUT（应被排除）
//   - urltest 组 "UT"           不可手动 PUT
//   - 真节点：vmess / trojan / shadowsocks / hysteria 各种
//   - 伪节点："剩余流量：xx GB" type=reject、"套餐到期" type=reject
func mockClashAllProxiesHandler() http.Handler {
	proxies := map[string]map[string]interface{}{
		// 真节点
		"美国01":   {"type": "Vmess"},
		"美国02":   {"type": "Vmess"},
		"日本01":   {"type": "Trojan"},
		"日本02":   {"type": "Trojan"},
		"新加坡01": {"type": "Shadowsocks"},
		"香港01":   {"type": "Hysteria2"},
		"台湾01":   {"type": "Wireguard"},
		// 伪节点
		"剩余流量：814.41 GB":   {"type": "Reject"},
		"套餐到期：2026-12-30": {"type": "Reject"},
		"防失联模式 zhuiyun": {"type": "Direct"},
		// 子组
		"FB":       {"type": "Fallback", "now": "美国01", "all": []string{"美国01", "日本01"}},
		"UT":       {"type": "URLTest", "now": "美国02", "all": []string{"美国02", "日本02"}},
		"AUTO_SUB": {"type": "Selector", "now": "新加坡01", "all": []string{"新加坡01", "香港01"}},
		// 顶层组
		"PROXY_TOP": {"type": "Selector", "now": "美国01", "all": []string{
			"美国01", "美国02", "日本01", "日本02", "新加坡01", "香港01", "台湾01",
			"剩余流量：814.41 GB", "套餐到期：2026-12-30", "防失联模式 zhuiyun",
			"FB", "UT", "AUTO_SUB",
		}},
		"GLOBAL": {"type": "Selector", "now": "DIRECT", "all": []string{
			"DIRECT", "REJECT", "PROXY_TOP", "美国01",
		}},
		"DIRECT": {"type": "Direct"},
		"REJECT": {"type": "Reject"},
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/proxies" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"proxies": proxies})
			return
		}
		if len(path) > len("/proxies/") && path[:len("/proxies/")] == "/proxies/" {
			name := path[len("/proxies/"):]
			if p, ok := proxies[name]; ok {
				_ = json.NewEncoder(w).Encode(p)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
}

func TestAutoDetectClashGroup_PicksTopSelectorWithMostRealNodes(t *testing.T) {
	srv := httptest.NewServer(mockClashAllProxiesHandler())
	defer srv.Close()

	res := AutoDetectClashGroup(srv.URL, "")
	if !res.OK {
		t.Fatalf("expected OK, got %+v", res)
	}
	if res.Group != "PROXY_TOP" {
		t.Errorf("expected PROXY_TOP (most real nodes), got %q", res.Group)
	}
	// 7 真节点 + 0 伪节点 + 0 子组 = 7
	if res.NodeCount != 7 {
		t.Errorf("expected 7 real nodes after type filter, got %d (candidates=%v)",
			res.NodeCount, res.Candidates)
	}
	for _, n := range res.Candidates {
		switch n {
		case "FB", "UT", "AUTO_SUB":
			t.Errorf("子组 %q 不应作为候选节点", n)
		case "剩余流量：814.41 GB", "套餐到期：2026-12-30", "防失联模式 zhuiyun":
			t.Errorf("伪节点 %q 不应作为候选节点", n)
		case "DIRECT", "REJECT":
			t.Errorf("特殊节点 %q 不应作为候选节点", n)
		}
	}
}

func TestAutoDetectClashGroup_ExcludesGLOBAL(t *testing.T) {
	srv := httptest.NewServer(mockClashAllProxiesHandler())
	defer srv.Close()

	res := AutoDetectClashGroup(srv.URL, "")
	if res.Group == "GLOBAL" {
		t.Error("GLOBAL 必须被排除（会让切换到 DIRECT/REJECT）")
	}
}

func TestFilterCandidatesWithKinds_DropsSubgroupsAndFakeEntries(t *testing.T) {
	r := &ClashRotator{cfg: ClashRotatorConfig{Group: "PROXY_TOP"}}
	all := []string{
		"美国01", "美国02", "日本01",
		"FB", "UT", "AUTO_SUB",
		"剩余流量：xx", "DIRECT",
	}
	kinds := map[string]string{
		"美国01":      "vmess",
		"美国02":      "vmess",
		"日本01":      "trojan",
		"FB":         "fallback",
		"UT":         "urltest",
		"AUTO_SUB":   "selector",
		"剩余流量：xx": "reject",
		"DIRECT":     "direct",
	}
	got := r.filterCandidatesWithKinds(all, "美国01", kinds)

	// 期望：仅 "美国02"、"日本01"。
	// "美国01" 是 current；其余全因 type 被排除。
	want := map[string]bool{"美国02": true, "日本01": true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for _, n := range got {
		if !want[n] {
			t.Errorf("unexpected candidate %q", n)
		}
	}
}

func TestFilterCandidatesWithKinds_FallsBackWhenKindsNil(t *testing.T) {
	r := &ClashRotator{cfg: ClashRotatorConfig{Group: "PROXY"}}
	all := []string{"DIRECT", "REJECT", "GLOBAL", "PROXY", "美国01", "美国02"}
	got := r.filterCandidatesWithKinds(all, "美国01", nil)
	// kinds=nil 时降级到 filterCandidates，特殊名字仍被排除
	if len(got) != 1 || got[0] != "美国02" {
		t.Errorf("got %v, want [美国02]", got)
	}
}

func TestIsFakeNodeName_AirportAdInjections(t *testing.T) {
	fake := []string{
		"剩余流量：814.41 GB",
		"套餐到期：2026-12-30",
		"防失联 (无痕模式访问):zhuiyun.one",
		"过期续费请联系 TG 客服",
		"zhuiyun.one",
		"example.com",
		"ad-server.io",
		"流量重置每月",
		"官网公告",
	}
	for _, n := range fake {
		if !isFakeNodeName(n) {
			t.Errorf("isFakeNodeName(%q) = false, want true (机场广告/伪节点)", n)
		}
	}
	real := []string{
		"美国01【vip1】",
		"新加坡 02【vip1 】",
		"日本-Premium-01",
		"🇺🇸 US-Tokyo-1",
		"HK-Edge-Node",
	}
	for _, n := range real {
		if isFakeNodeName(n) {
			t.Errorf("isFakeNodeName(%q) = true, want false (真节点)", n)
		}
	}
}

func TestIsRealNodeKind(t *testing.T) {
	cases := map[string]bool{
		"vmess": true, "trojan": true, "shadowsocks": true,
		"ss": true, "hysteria": true, "hysteria2": true,
		"wireguard": true, "tuic": true, "snell": true,
		"selector": false, "fallback": false, "urltest": false,
		"loadbalance": false, "url-test": false,
		"direct": false, "reject": false, "compatible": false,
		"": false,
	}
	for kind, want := range cases {
		if got := isRealNodeKind(kind); got != want {
			t.Errorf("isRealNodeKind(%q) = %v, want %v", kind, got, want)
		}
	}
}
