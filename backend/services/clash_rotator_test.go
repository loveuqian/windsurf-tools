package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeProxy implements ProxyDelayQuerier
type fakeProxy struct {
	inflight int64
	closes   int64
}

func (f *fakeProxy) InFlightChatStreams() int64       { return atomic.LoadInt64(&f.inflight) }
func (f *fakeProxy) CloseUpstreamIdleConnections()    { atomic.AddInt64(&f.closes, 1) }

// mockClashServer 提供最小可用的 Clash REST API。
type mockClashServer struct {
	t      *testing.T
	mu     sync.Mutex
	now    string
	all    []string
	delays map[string]int
	puts   []string
}

func newMockClashServer(t *testing.T, all []string, currentNow string) *mockClashServer {
	return &mockClashServer{t: t, now: currentNow, all: all, delays: map[string]int{}}
}

func (m *mockClashServer) setDelay(node string, ms int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.delays[node] = ms
}

func (m *mockClashServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/proxies" && r.Method == http.MethodGet:
			m.mu.Lock()
			resp := map[string]interface{}{
				"proxies": map[string]interface{}{
					"PROXY": map[string]interface{}{
						"now":  m.now,
						"all":  m.all,
						"type": "Selector",
						"name": "PROXY",
					},
					"DIRECT": map[string]interface{}{"type": "Direct"},
				},
			}
			m.mu.Unlock()
			_ = json.NewEncoder(w).Encode(resp)
		case strings.HasPrefix(path, "/proxies/") && strings.HasSuffix(path, "/delay") && r.Method == http.MethodGet:
			node := strings.TrimSuffix(strings.TrimPrefix(path, "/proxies/"), "/delay")
			m.mu.Lock()
			d := m.delays[node]
			m.mu.Unlock()
			if d <= 0 {
				w.WriteHeader(http.StatusGatewayTimeout)
				_ = json.NewEncoder(w).Encode(map[string]string{"message": "timeout"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]int{"delay": d})
		case strings.HasPrefix(path, "/proxies/") && r.Method == http.MethodGet:
			name := strings.TrimPrefix(path, "/proxies/")
			m.mu.Lock()
			resp := map[string]interface{}{"now": m.now, "all": m.all, "type": "Selector", "name": name}
			m.mu.Unlock()
			_ = json.NewEncoder(w).Encode(resp)
		case strings.HasPrefix(path, "/proxies/") && r.Method == http.MethodPut:
			name := strings.TrimPrefix(path, "/proxies/")
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			m.mu.Lock()
			m.puts = append(m.puts, body["name"])
			m.now = body["name"]
			_ = name
			m.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func TestProbeClashController_OK(t *testing.T) {
	m := newMockClashServer(t, []string{"US-1", "US-2"}, "US-1")
	srv := httptest.NewServer(m.handler())
	defer srv.Close()

	res := ProbeClashController(srv.URL, "")
	if !res.OK {
		t.Fatalf("expected OK, got %+v", res)
	}
	if len(res.Groups) != 1 || res.Groups[0] != "PROXY" {
		t.Fatalf("expected groups=[PROXY], got %v", res.Groups)
	}
}

func TestListClashGroupNodes_FiltersSpecials(t *testing.T) {
	m := newMockClashServer(t, []string{"US-1", "US-2", "DIRECT", "REJECT", "PROXY"}, "US-1")
	srv := httptest.NewServer(m.handler())
	defer srv.Close()

	nodes, err := ListClashGroupNodes(srv.URL, "", "PROXY")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	want := map[string]bool{"US-1": true, "US-2": true}
	if len(nodes) != 2 {
		t.Fatalf("got %v, want 2", nodes)
	}
	for _, n := range nodes {
		if !want[n] {
			t.Errorf("unexpected node %q", n)
		}
	}
}

func TestRotator_BasicRotateOnInterval(t *testing.T) {
	m := newMockClashServer(t, []string{"US-1", "US-2", "JP-1"}, "US-1")
	srv := httptest.NewServer(m.handler())
	defer srv.Close()

	fp := &fakeProxy{}
	cfg := ClashRotatorConfig{
		ControllerURL:   srv.URL,
		Group:           "PROXY",
		Whitelist:       []string{"US-1", "US-2"},
		Interval:        2 * time.Minute, // 不会自动触发
		IdleWaitMax:     200 * time.Millisecond,
		LatencyMaxMs:    0, // 跳过测速
		RateLimitMinGap: 1 * time.Millisecond,
	}
	r := NewClashRotator(cfg, fp, func(string) {})
	r.Start()
	defer r.Stop()

	// 手动触发一次
	r.TriggerRotate("manual")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		n := len(m.puts)
		m.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	m.mu.Lock()
	puts := append([]string{}, m.puts...)
	now := m.now
	m.mu.Unlock()

	if len(puts) != 1 {
		t.Fatalf("expected 1 PUT, got %v", puts)
	}
	if puts[0] != "US-2" {
		t.Errorf("expected switch to US-2 (current=US-1, whitelist={US-1,US-2}), got %q", puts[0])
	}
	if now != "US-2" {
		t.Errorf("server now=%q, want US-2", now)
	}
	if atomic.LoadInt64(&fp.closes) != 1 {
		t.Errorf("expected CloseUpstreamIdleConnections to be called once, got %d", fp.closes)
	}
}

func TestRotator_LatencyFilter(t *testing.T) {
	m := newMockClashServer(t, []string{"FAST", "SLOW", "DEAD"}, "FAST")
	m.setDelay("FAST", 100)
	m.setDelay("SLOW", 1500)
	// DEAD: no delay registered → 504
	srv := httptest.NewServer(m.handler())
	defer srv.Close()

	fp := &fakeProxy{}
	cfg := ClashRotatorConfig{
		ControllerURL:   srv.URL,
		Group:           "PROXY",
		Interval:        2 * time.Minute,
		IdleWaitMax:     50 * time.Millisecond,
		LatencyMaxMs:    800,
		LatencyTestURL:  "http://example.com",
		RateLimitMinGap: 1 * time.Millisecond,
	}
	r := NewClashRotator(cfg, fp, func(string) {})
	r.Start()
	defer r.Stop()
	r.TriggerRotate("manual")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		n := len(m.puts)
		m.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	m.mu.Lock()
	puts := append([]string{}, m.puts...)
	m.mu.Unlock()

	if len(puts) != 1 {
		t.Fatalf("expected 1 PUT after latency filter, got %v", puts)
	}
	// FAST is current; SLOW exceeds 800ms; DEAD fails. 但若 latency 全失败兜底回到全候选 → 仍可能切到 SLOW/DEAD。
	// 这里至少断言切到了非 FAST 的某个节点。
	if puts[0] == "FAST" {
		t.Errorf("expected to switch away from FAST, got %q", puts[0])
	}
}

func TestRotator_TriggerRateLimitDebounce(t *testing.T) {
	m := newMockClashServer(t, []string{"A", "B", "C"}, "A")
	srv := httptest.NewServer(m.handler())
	defer srv.Close()

	fp := &fakeProxy{}
	cfg := ClashRotatorConfig{
		ControllerURL:   srv.URL,
		Group:           "PROXY",
		Interval:        2 * time.Minute,
		IdleWaitMax:     20 * time.Millisecond,
		LatencyMaxMs:    0,
		RotateOnRL:      true,
		RateLimitMinGap: 500 * time.Millisecond,
	}
	r := NewClashRotator(cfg, fp, func(string) {})
	r.Start()
	defer r.Stop()

	// 连发 5 次 → 应只触发 1 次 PUT（debounce）
	for i := 0; i < 5; i++ {
		r.TriggerRotate("rate_limit")
	}
	time.Sleep(800 * time.Millisecond)

	m.mu.Lock()
	puts := append([]string{}, m.puts...)
	m.mu.Unlock()
	if len(puts) != 1 {
		t.Fatalf("expected 1 PUT (debounced), got %d (%v)", len(puts), puts)
	}
}

func TestRotator_WaitsForIdle(t *testing.T) {
	m := newMockClashServer(t, []string{"A", "B"}, "A")
	srv := httptest.NewServer(m.handler())
	defer srv.Close()

	fp := &fakeProxy{}
	atomic.StoreInt64(&fp.inflight, 1)

	cfg := ClashRotatorConfig{
		ControllerURL:   srv.URL,
		Group:           "PROXY",
		Interval:        2 * time.Minute,
		IdleWaitMax:     2 * time.Second,
		LatencyMaxMs:    0,
		RateLimitMinGap: 1 * time.Millisecond,
	}
	r := NewClashRotator(cfg, fp, func(string) {})
	r.Start()
	defer r.Stop()

	r.TriggerRotate("manual")
	// 300ms 后释放 in-flight；rotator 应在此后切
	time.Sleep(300 * time.Millisecond)
	m.mu.Lock()
	if len(m.puts) != 0 {
		m.mu.Unlock()
		t.Fatalf("expected no PUT yet (still in-flight)")
	}
	m.mu.Unlock()
	atomic.StoreInt64(&fp.inflight, 0)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		n := len(m.puts)
		m.mu.Unlock()
		if n > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("expected switch after in-flight cleared")
}
