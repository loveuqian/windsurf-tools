package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
)

// countingReader 在每次 Read 调用计数 —— 用来探测 retryTransport 是否
// 对同一份 body 做了二次 ReadAll。
type countingReader struct {
	data    []byte
	pos     int
	reads   *atomic.Int32
	closed  *atomic.Int32
}

func (r *countingReader) Read(p []byte) (int, error) {
	r.reads.Add(1)
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func (r *countingReader) Close() error {
	r.closed.Add(1)
	return nil
}

// stubBaseRT 模拟 http.RoundTripper —— 把 req.Body 全部读出来后返回固定响应。
// 调 1 次后端 = 一次 RoundTrip。
type stubBaseRT struct {
	body         []byte
	hits         atomic.Int32
	respFactory  func() *http.Response
	bodyConsumed [][]byte // 每次 RoundTrip 实际收到的 body（用于断言）
}

func (rt *stubBaseRT) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.hits.Add(1)
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		_ = req.Body.Close()
		rt.bodyConsumed = append(rt.bodyConsumed, b)
	}
	if rt.respFactory != nil {
		return rt.respFactory(), nil
	}
	return &http.Response{
		StatusCode: 200,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(rt.body)),
	}, nil
}

// TestRetryTransportPrefersGetBody —— D-2 回归
//
// handleRequest 已设 GetBody 时，retryTransport.RoundTrip 应该用 GetBody
// 重新拿可重放的 body，而不是再读一次原 req.Body。
func TestRetryTransportPrefersGetBody(t *testing.T) {
	originalBody := []byte("payload-from-handle-request-after-replace-identity")

	// 模拟 handleRequest 完成后的 req：req.Body 是 NopCloser，但 GetBody 已设置
	innerReads := &atomic.Int32{}
	innerCloses := &atomic.Int32{}
	getBodyCalls := &atomic.Int32{}

	req, err := http.NewRequest("POST", "https://example.com/test", &countingReader{
		data:   originalBody,
		reads:  innerReads,
		closed: innerCloses,
	})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.GetBody = func() (io.ReadCloser, error) {
		getBodyCalls.Add(1)
		return io.NopCloser(bytes.NewReader(originalBody)), nil
	}
	req.ContentLength = int64(len(originalBody))

	base := &stubBaseRT{body: []byte("ok")}
	rt := &retryTransport{
		base:     base,
		proxy:    newProxyForTest(),
		maxRetry: 0, // 不重试
	}

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	// 关键断言：GetBody 被调一次，原 req.Body（countingReader）一次都没被读
	if got := getBodyCalls.Load(); got != 1 {
		t.Errorf("GetBody calls = %d, want 1", got)
	}
	if got := innerReads.Load(); got != 0 {
		t.Errorf("original body Read calls = %d, want 0 (should not double-ReadAll)", got)
	}
	// 上游收到的 body 应该是完整的原始 body
	if base.hits.Load() != 1 {
		t.Errorf("base RoundTrip hits = %d, want 1", base.hits.Load())
	}
	if len(base.bodyConsumed) != 1 || !bytes.Equal(base.bodyConsumed[0], originalBody) {
		t.Errorf("body sent upstream = %q, want %q", base.bodyConsumed, originalBody)
	}
}

// TestRetryTransportFallbackWithoutGetBody —— D-2 兼容性回归
//
// req.GetBody == nil 时（外部直接调 RoundTrip 没经过 handleRequest），
// retryTransport 应该走旧路径：ReadAll 原 req.Body + cloneBytes 留作重试。
func TestRetryTransportFallbackWithoutGetBody(t *testing.T) {
	originalBody := []byte("legacy-no-getbody-path")

	innerReads := &atomic.Int32{}
	innerCloses := &atomic.Int32{}
	req, err := http.NewRequest("POST", "https://example.com/test", &countingReader{
		data:   originalBody,
		reads:  innerReads,
		closed: innerCloses,
	})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	// 关键：不设 GetBody —— 模拟旧路径
	req.GetBody = nil

	base := &stubBaseRT{body: []byte("ok")}
	rt := &retryTransport{
		base:     base,
		proxy:    newProxyForTest(),
		maxRetry: 0,
	}

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	// 旧路径：原 req.Body 应该被读了至少一次（io.ReadAll 内部多次 Read）
	if got := innerReads.Load(); got == 0 {
		t.Errorf("original body Read calls = 0, want >0 (legacy path should ReadAll)")
	}
	if got := innerCloses.Load(); got == 0 {
		t.Errorf("original body Close calls = 0, want >0")
	}
	if len(base.bodyConsumed) != 1 || !bytes.Equal(base.bodyConsumed[0], originalBody) {
		t.Errorf("body sent upstream = %q, want %q", base.bodyConsumed, originalBody)
	}
}

// TestRetryTransportReplayUsesUpdatedGetBody —— D-2 重试场景回归
//
// 第一次 RoundTrip 返回 quota_exceeded → retryTransport 切号重试 →
// 重试前 setRetryBody 应该已经更新 GetBody，新的 GetBody closure 捕获新 body。
// 验证 transport 在重试时收到的是新 body。
func TestRetryTransportReplayUsesUpdatedGetBody(t *testing.T) {
	originalBody := []byte("orig-body-with-old-key")

	req, err := http.NewRequest("POST", "https://example.com/exa.api_server_pb.ApiServerService/GetChatMessage", bytes.NewReader(originalBody))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(originalBody)), nil
	}
	req.ContentLength = int64(len(originalBody))
	req.Header.Set("X-Pool-Key-Used", "old-key-pretend-exhausted")
	req.Header.Set("X-Conv-ID", "")

	// 第一次返回 quota_exceeded（用 connect EOS 帧伪造）
	// 简化：直接用 grpc-status header 触发 fallback 检测
	respCount := atomic.Int32{}
	base := &stubBaseRT{
		respFactory: func() *http.Response {
			n := respCount.Add(1)
			h := http.Header{}
			if n == 1 {
				// 第一次：触发额度耗尽（grpc-status fallback 路径）
				h.Set("grpc-status", "8") // 8 = RESOURCE_EXHAUSTED
				h.Set("grpc-message", "quota exhausted for trial users")
				return &http.Response{
					StatusCode: 200,
					Header:     h,
					Body:       io.NopCloser(bytes.NewReader([]byte("err"))),
					Request:    &http.Request{URL: &url.URL{Path: "/test"}, Header: http.Header{}},
				}
			}
			// 第二次：成功
			return &http.Response{
				StatusCode: 200,
				Header:     http.Header{},
				Body:       io.NopCloser(bytes.NewReader([]byte("ok"))),
				Request:    &http.Request{URL: &url.URL{Path: "/test"}, Header: http.Header{}},
			}
		},
	}

	proxy := newProxyForTest()
	// 注入两个 key 让 pickPoolKeyAndJWT 能切换；新 key 走 retry 路径
	proxy.poolKeys = []string{"old-key-pretend-exhausted", "new-key-fresh"}
	proxy.keyStates = map[string]*PoolKeyState{
		"old-key-pretend-exhausted": newPoolKeyState("old-key-pretend-exhausted"),
		"new-key-fresh":             newPoolKeyState("new-key-fresh"),
	}
	// 给 new-key 一个有效 JWT 让 pickPoolKey 能选到它
	proxy.keyStates["new-key-fresh"].JWT = []byte("fake-jwt-12345")

	rt := &retryTransport{
		base:     base,
		proxy:    proxy,
		maxRetry: 1,
	}

	resp, err := rt.RoundTrip(req.WithContext(context.Background()))
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	// 应该 RoundTrip 调了 2 次（首次 + 重试 1 次）
	if got := base.hits.Load(); got != 2 {
		t.Errorf("base hits = %d, want 2", got)
	}
	// 两次发送的 body 都是有效（不一定相等，因为重试时身份可能被替换）
	if len(base.bodyConsumed) != 2 {
		t.Fatalf("bodyConsumed entries = %d, want 2", len(base.bodyConsumed))
	}
	for i, b := range base.bodyConsumed {
		if len(b) == 0 {
			t.Errorf("body sent on attempt %d is empty", i+1)
		}
	}
}

// newProxyForTest 给 retryTransport 单测建一个最小可用 *MitmProxy，
// 不启动 listener 不需要 cert，只满足 retryTransport 调用的 method 不 panic。
func newProxyForTest() *MitmProxy {
	p := &MitmProxy{
		poolKeys:   []string{},
		keyStates:  map[string]*PoolKeyState{},
		sessionMap: map[string]*SessionBinding{},
		jwtFetches: map[string]*jwtFetchCall{},
		stopCh:     make(chan struct{}),
		logFn:      func(string) {},
	}
	return p
}

// 防止 errors / fmt 在某些路径未被引用时编译警告
var _ = errors.New
var _ = fmt.Errorf
