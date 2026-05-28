package services

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// makeConnectFrame 构造 Connect 协议帧：[1B flag][4B BE payloadLen][payload]
func makeConnectFrame(flag byte, payload []byte) []byte {
	out := make([]byte, 5+len(payload))
	out[0] = flag
	binary.BigEndian.PutUint32(out[1:5], uint32(len(payload)))
	copy(out[5:], payload)
	return out
}

// TestRetryTransportPeekPassthroughOnDataFrame —— P1 回归
//
// 普通 data frame（flag=0x00）必须立即 passthrough，不阻塞首字延迟。
// 旧实现 peek 8KB 会等上游凑齐 8193 字节才返回；新实现只读 5B 帧头。
func TestRetryTransportPeekPassthroughOnDataFrame(t *testing.T) {
	proxy := NewMitmProxy(nil, nil, "", nil)

	dataFrame := makeConnectFrame(0x00, bytes.Repeat([]byte("X"), 1024))
	eosFrame := makeConnectFrame(0x02, []byte(`{}`))
	fullStream := append(dataFrame, eosFrame...)

	body := &countingReadCloser{reader: bytes.NewReader(fullStream)}
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    200,
			ContentLength: -1,
			Body:          body,
			Header:        http.Header{"Content-Type": []string{"application/connect+proto"}},
			Request:       req,
		}, nil
	})

	rt := &retryTransport{base: base, proxy: proxy, maxRetry: 1}
	req, _ := http.NewRequest(http.MethodPost, "https://server.self-serve.windsurf.com/test", bytes.NewBufferString(""))
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	if resp == nil {
		t.Fatal("resp is nil")
	}

	// peek 只读 5B 帧头一次（bytes.Reader 一次性返回），剩余 stream 由调用方消费
	if body.reads != 1 {
		t.Fatalf("data-frame peek reads = %d, want 1 (only 5B header)", body.reads)
	}

	// 完整读 response body 应该 = 原始 stream（5B 还原 + 后续直读）
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !bytes.Equal(got, fullStream) {
		t.Fatalf("body bytes mismatch: got %dB, want %dB", len(got), len(fullStream))
	}
}

// TestRetryTransportPeekBuffersEOSErrorFrame —— P1 回归
//
// EOS-only 错误帧（flag=0x02 + 合理 payloadLen）必须仍然被缓冲、走错误分类，
// 否则 rate limit / quota 检测会失效。
func TestRetryTransportPeekBuffersEOSErrorFrame(t *testing.T) {
	proxy := NewMitmProxy(nil, nil, "", nil)
	proxy.poolKeys = []string{"sk-ws-a"}
	proxy.keyStates["sk-ws-a"] = &PoolKeyState{APIKey: "sk-ws-a", Healthy: true, JWT: []byte("jwt-a")}

	errPayload, _ := json.Marshal(map[string]any{
		"error": map[string]string{
			"code":    "resource_exhausted",
			"message": "out of credits",
		},
	})
	eosFrame := makeConnectFrame(0x02, errPayload)

	body := &countingReadCloser{reader: bytes.NewReader(eosFrame)}
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    200,
			ContentLength: -1,
			Body:          body,
			Header:        http.Header{"Content-Type": []string{"application/connect+proto"}},
			Request:       req,
		}, nil
	})

	rt := &retryTransport{base: base, proxy: proxy, maxRetry: 0}
	req, _ := http.NewRequest(http.MethodPost, "https://server.self-serve.windsurf.com/test", bytes.NewBufferString(""))
	req.Header.Set("X-Pool-Key-Used", "sk-ws-a")
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	if resp == nil {
		t.Fatal("resp is nil")
	}

	// EOS-only 帧 peek 路径：5B 头 + payload 各一次 read（bytes.Reader 一次到位）
	if body.reads < 1 || body.reads > 3 {
		t.Fatalf("EOS-frame peek reads = %d, want 1-3", body.reads)
	}

	// resp.Body = 完整 EOS frame（被缓冲后 NopCloser 包装供下游再次解析）
	got, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(got, eosFrame) {
		t.Fatalf("body bytes mismatch: got %dB, want %dB", len(got), len(eosFrame))
	}

	// 关键：buffered body 必须仍然能被 Connect EOS 解析器还原成原始错误。
	// 这是 P1 改造保留错误检测能力的核心断言（下游 handleResponse 用同一个
	// ParseConnectEOS 做最终分类，能在这里解析出来即等价于不丢错误检测）。
	parsed := ParseConnectEOS(got)
	if !parsed.IsError {
		t.Fatalf("ParseConnectEOS() failed to recover error from buffered EOS frame")
	}
	if parsed.Code != "resource_exhausted" {
		t.Fatalf("ParseConnectEOS() code = %q, want %q", parsed.Code, "resource_exhausted")
	}
}

// TestRetryTransportPeekPassthroughOnIllegalLength —— P1 回归
//
// flag&0x02 但 payloadLen 异常大（>4KB 上限），不应被误当成 EOS error 缓冲；
// 必须立即 passthrough 防止误读大量数据。
func TestRetryTransportPeekPassthroughOnIllegalLength(t *testing.T) {
	proxy := NewMitmProxy(nil, nil, "", nil)

	// flag=0x02 + payloadLen=0xFFFFFFFF（远超 eosPayloadCap=4096）
	head := []byte{0x02, 0xFF, 0xFF, 0xFF, 0xFF}
	streamRest := bytes.Repeat([]byte("Y"), 100)
	full := append(head, streamRest...)

	body := &countingReadCloser{reader: bytes.NewReader(full)}
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    200,
			ContentLength: -1,
			Body:          body,
			Header:        http.Header{"Content-Type": []string{"application/connect+proto"}},
			Request:       req,
		}, nil
	})

	rt := &retryTransport{base: base, proxy: proxy, maxRetry: 1}
	req, _ := http.NewRequest(http.MethodPost, "https://server.self-serve.windsurf.com/test", bytes.NewBufferString(""))
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}

	// 非法长度走 passthrough：peek 只读 5B 帧头一次
	if body.reads != 1 {
		t.Fatalf("illegal-length peek reads = %d, want 1", body.reads)
	}

	got, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(got, full) {
		t.Fatalf("body bytes mismatch: got %dB, want %dB", len(got), len(full))
	}
}
