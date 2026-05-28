package services

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/http2"
)

// TestMitmServerNegotiatesHTTP2 —— D-1 回归
//
// 复现 D-1 修复路径：tlsConfig 设 NextProtos + http2.ConfigureServer →
// 客户端 ALPN 协商应该选 h2，请求 ProtoMajor==2。旧实现 (没 NextProtos /
// 没 ConfigureServer) 会降级到 HTTP/1.1 → IDE 多请求被 6-连接限制 + 各自
// TLS 握手 → 首字慢。
func TestMitmServerNegotiatesHTTP2(t *testing.T) {
	cert := newSelfSignedCert(t)

	// 模拟 D-1 修复后的 listener 配置
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h2", "http/1.1"},
	}
	ln, err := tls.Listen("tcp4", "127.0.0.1:0", tlsConfig)
	if err != nil {
		t.Fatalf("tls.Listen: %v", err)
	}
	defer ln.Close()

	var hits atomic.Int32
	server := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			// 把客户端实际协议版本写到响应头里给单测断言
			w.Header().Set("X-Server-Saw-Proto", fmt.Sprintf("HTTP/%d.%d", r.ProtoMajor, r.ProtoMinor))
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "ok")
		}),
	}
	if err := http2.ConfigureServer(server, &http2.Server{
		IdleTimeout:          30 * time.Second,
		MaxConcurrentStreams: 100,
	}); err != nil {
		t.Fatalf("ConfigureServer: %v", err)
	}

	go func() { _ = server.Serve(ln) }()
	defer server.Close()

	// 客户端走 http2.Transport（ALPN 选 h2）
	clientTransport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			NextProtos:         []string{"h2", "http/1.1"},
		},
		ForceAttemptHTTP2: true,
	}
	if _, err := http2.ConfigureTransports(clientTransport); err != nil {
		t.Fatalf("ConfigureTransports: %v", err)
	}
	client := &http.Client{Transport: clientTransport, Timeout: 5 * time.Second}

	addr := ln.Addr().String()
	u := &url.URL{Scheme: "https", Host: addr, Path: "/test"}
	req, _ := http.NewRequest(http.MethodGet, u.String(), nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if hits.Load() == 0 {
		t.Fatalf("server didn't see request")
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q, want %q", body, "ok")
	}
	// 客户端视角 — 应该是 HTTP/2
	if resp.ProtoMajor != 2 {
		t.Fatalf("client saw proto major %d, want 2", resp.ProtoMajor)
	}
	// 服务端视角 — Header 上回写的应该是 HTTP/2.0
	if got := resp.Header.Get("X-Server-Saw-Proto"); got != "HTTP/2.0" {
		t.Fatalf("server saw %q, want %q", got, "HTTP/2.0")
	}
}

// TestMitmServerFallsBackToHTTP1 —— D-1 兼容性回归
//
// 客户端不支持 h2 时（NextProtos: ["http/1.1"]），ALPN 应该选 http/1.1，
// 服务端继续工作。验证 D-1 没破坏旧客户端兼容性。
func TestMitmServerFallsBackToHTTP1(t *testing.T) {
	cert := newSelfSignedCert(t)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h2", "http/1.1"},
	}
	ln, err := tls.Listen("tcp4", "127.0.0.1:0", tlsConfig)
	if err != nil {
		t.Fatalf("tls.Listen: %v", err)
	}
	defer ln.Close()

	server := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Server-Saw-Proto", fmt.Sprintf("HTTP/%d.%d", r.ProtoMajor, r.ProtoMinor))
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "ok")
		}),
	}
	_ = http2.ConfigureServer(server, &http2.Server{})
	go func() { _ = server.Serve(ln) }()
	defer server.Close()

	// 客户端只声明 http/1.1
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				NextProtos:         []string{"http/1.1"},
			},
			ForceAttemptHTTP2: false,
		},
		Timeout: 5 * time.Second,
	}

	addr := ln.Addr().String()
	resp, err := client.Get((&url.URL{Scheme: "https", Host: addr, Path: "/test"}).String())
	if err != nil {
		t.Fatalf("client.Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.ProtoMajor != 1 {
		t.Fatalf("client saw proto major %d, want 1 (h2 fallback expected)", resp.ProtoMajor)
	}
}

// newSelfSignedCert 给单测生成一个临时自签证书，避免依赖项目的 EnsureCA。
func newSelfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "mitm-h2-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  priv,
	}
}
