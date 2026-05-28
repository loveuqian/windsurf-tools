package services

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"fmt"
	"io"
	"sync"
	"testing"
)

// TestGzipPoolRoundTrip —— D-5 等价性回归
//
// 单线程：gzipBytes + gunzipBytes 往返必须等于原始 bytes，且与标准库
// gzip.NewReader / gzip.NewWriter 行为一致。
func TestGzipPoolRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"short", []byte("hello world")},
		{"binary", []byte{0x00, 0xff, 0x55, 0xaa, 0x10, 0x20, 0x30}},
		{"long-zero", bytes.Repeat([]byte{0}, 64*1024)},
		{"random-128k", randBytes(t, 128*1024)},
		{"json-like", []byte(`{"key":"sk-ws-abc123","jwt":"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ1c2VyLTEyMyJ9","data":[1,2,3]}`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			compressed := gzipBytes(tc.data)
			// 解压回来必须等于原始
			out, err := gunzipBytes(compressed)
			if err != nil {
				t.Fatalf("gunzipBytes: %v", err)
			}
			if !bytes.Equal(out, tc.data) {
				t.Fatalf("round-trip mismatch: got %d bytes, want %d", len(out), len(tc.data))
			}
			// 标准库也能解我们 pool 写出来的 gzip
			zr, err := gzip.NewReader(bytes.NewReader(compressed))
			if err != nil {
				t.Fatalf("std gzip.NewReader: %v", err)
			}
			stdOut, err := io.ReadAll(zr)
			zr.Close()
			if err != nil {
				t.Fatalf("std ReadAll: %v", err)
			}
			if !bytes.Equal(stdOut, tc.data) {
				t.Fatalf("std lib decompress mismatch: got %d bytes, want %d", len(stdOut), len(tc.data))
			}
		})
	}
}

// TestGzipPoolDecompressStdGzip —— D-5 互操作性回归
//
// 上游传来的 gzip envelope 是标准库 gzip.NewWriter 生成的，pool 的
// gunzipBytes 必须能正确解析（不漏数据 / 不损坏）。
func TestGzipPoolDecompressStdGzip(t *testing.T) {
	original := []byte("payload-from-upstream-server-with-gzip-encoding")
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write(original)
	_ = zw.Close()

	got, err := gunzipBytes(buf.Bytes())
	if err != nil {
		t.Fatalf("gunzipBytes: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("decompressed = %q, want %q", got, original)
	}
}

// TestGzipPoolInvalidGzip —— 错误路径回归
//
// 非 gzip 字节传给 gunzipBytes 应该返回 error，pool 内部状态保持健康
// （后续 Get 还能正常工作）。
func TestGzipPoolInvalidGzip(t *testing.T) {
	bad := []byte("not even close to a gzip magic header")
	_, err := gunzipBytes(bad)
	if err == nil {
		t.Fatalf("expected error on invalid gzip, got nil")
	}

	// 验证 pool 仍然健康：后续合法调用应当成功
	original := []byte("after-error-pool-still-healthy")
	compressed := gzipBytes(original)
	got, err := gunzipBytes(compressed)
	if err != nil {
		t.Fatalf("subsequent gunzipBytes after error: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("post-error round-trip mismatch")
	}
}

// TestGzipPoolConcurrentRoundTrip —— D-5 并发安全回归
//
// 100 goroutine 同时 round-trip 不同 body，验证 pool reset 不漏污染、
// 不数据竞争。每个 goroutine 都用唯一 payload，断言往返等价。
func TestGzipPoolConcurrentRoundTrip(t *testing.T) {
	const goroutines = 100
	const itersPerG = 20
	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)

	for g := 0; g < goroutines; g++ {
		g := g
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < itersPerG; i++ {
				payload := []byte(fmt.Sprintf("g=%d i=%d %s", g, i, randPayload(g, i)))
				compressed := gzipBytes(payload)
				out, err := gunzipBytes(compressed)
				if err != nil {
					errCh <- fmt.Errorf("g=%d i=%d gunzip: %w", g, i, err)
					return
				}
				if !bytes.Equal(out, payload) {
					errCh <- fmt.Errorf("g=%d i=%d round-trip mismatch: %d vs %d bytes", g, i, len(out), len(payload))
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent: %v", err)
	}
}

// BenchmarkGzipDirect —— 旧实现基线（每次 gzip.NewReader/NewWriter 直创建）
func BenchmarkGzipDirect(b *testing.B) {
	payload := bytes.Repeat([]byte("the quick brown fox jumps over the lazy dog. "), 200) // ~9KB
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		_, _ = zw.Write(payload)
		_ = zw.Close()

		zr, err := gzip.NewReader(bytes.NewReader(buf.Bytes()))
		if err != nil {
			b.Fatal(err)
		}
		_, _ = io.ReadAll(zr)
		_ = zr.Close()
	}
}

// BenchmarkGzipPool —— D-5 pool 实现
func BenchmarkGzipPool(b *testing.B) {
	payload := bytes.Repeat([]byte("the quick brown fox jumps over the lazy dog. "), 200) // ~9KB
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		compressed := gzipBytes(payload)
		_, err := gunzipBytes(compressed)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ── helpers ──

func randBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	return b
}

// randPayload 给并发测试做出可重现且不同 goroutine 间各异的 payload。
func randPayload(g, i int) string {
	// 让长度也在变化，覆盖 pool reset 不同状态
	n := (g*7 + i*13) % 4096
	return string(bytes.Repeat([]byte{byte('a' + (g+i)%26)}, n+1))
}
