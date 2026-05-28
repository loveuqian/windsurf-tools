package services

// proxy_provider_route.go ── MITM 入口分流到提供商上游(阶段 2)。
//
// MITM serve() 的 handler 调本文件的 tryServeRoute:
//   - 不参与时返回 false, 调用方继续走号池 reverse proxy(向后兼容)
//   - 参与时直接写完响应返回 true
//
// 跳过条件(任一即跳过, 落回号池):
//   - p.router == nil
//   - router.RouteMode() != "providers"
//   - 请求路径不是 chat path
//   - Content-Type 不是 protobuf/grpc
//   - body 解 envelope/gzip 失败

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (p *MitmProxy) tryServeRoute(w http.ResponseWriter, r *http.Request) bool {
	router := p.router
	if router == nil {
		return false
	}
	if router.RouteMode() != "providers" {
		return false
	}
	if !isChatPath(r.URL.Path) {
		return false
	}
	ct := strings.ToLower(r.Header.Get("Content-Type"))
	if !strings.Contains(ct, "proto") && !strings.Contains(ct, "grpc") {
		return false
	}

	// 读完整 body 出来 — chat 请求一般 30-94KB, buffer 进内存可接受
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		// 读失败已经无法回退号池(body 已部分消费), 直接 EOS error
		writeRouteEOSError(w, "internal", "读 body 失败: "+err.Error())
		return true
	}
	cascadeBody, err := stripGRPCEnvelopeMaybeGzip(rawBody)
	if err != nil {
		// envelope 解析失败 → 还原 body 让号池流程接(可能是非标准 IDE 协议)
		p.log("provider route: 解 envelope 失败,回落号池: %v", err)
		r.Body = io.NopCloser(bytes.NewReader(rawBody))
		return false
	}

	ctx := r.Context()
	ctx, cancel := context.WithTimeout(ctx, routeTimeout)
	defer cancel()

	httpClient := p.routeClient()
	if Route(ctx, w, httpClient, router, cascadeBody, p.usageTracker) == RouteFallback {
		// 预检未通过且未写任何字节 → 还原 body,回落号池
		r.Body = io.NopCloser(bytes.NewReader(rawBody))
		return false
	}
	return true
}

// routeClient 返回给 provider Route 用的 *http.Client。
// 优先用全局 TransportPool(支持 clash/env 代理 + 连接池复用);
// 未注入时回退 http.DefaultClient(直连)。
func (p *MitmProxy) routeClient() *http.Client {
	p.mu.RLock()
	pool := p.transportPool
	p.mu.RUnlock()
	if pool != nil {
		return pool.Client()
	}
	return http.DefaultClient
}

// writeRouteEOSError 直接写 cascade EOS error frame 给 IDE,
// 用于读 body 失败这类无法回退号池的死路。
func writeRouteEOSError(w http.ResponseWriter, code, message string) {
	w.Header().Set("Content-Type", "application/connect+proto")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(EncodeCascadeEOSError(code, message))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// stripGRPCEnvelopeMaybeGzip 剥 5 字节 Connect/gRPC envelope, 如果 flag bit 0
// 标记 gzip 压缩则解压。对偶 chat_proto.go 的 WrapGRPCEnvelope*。
//
// envelope 布局:
//   byte 0    : flags (bit 0 = compressed)
//   byte 1-4  : payload length (big-endian uint32)
//   byte 5..  : payload
func stripGRPCEnvelopeMaybeGzip(raw []byte) ([]byte, error) {
	if len(raw) < 5 {
		return nil, fmt.Errorf("body 短于 5 字节, 不是 envelope")
	}
	flags := raw[0]
	payload := raw[5:]
	// 字段 length(BE32) 仅作软校验 — 部分 client 标记不准, 用实际剩余字节
	_ = binary.BigEndian.Uint32(raw[1:5])
	if flags&0x01 == 0 {
		return payload, nil
	}
	gzr, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer gzr.Close()
	out, err := io.ReadAll(gzr)
	if err != nil {
		return nil, fmt.Errorf("gzip read: %w", err)
	}
	return out, nil
}
