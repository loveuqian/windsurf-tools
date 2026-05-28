package services

import (
	"bytes"
	"compress/gzip"
	"io"
	"sync"
)

// ═══════════════════════════════════════════════════════════════
// gzip Reader/Writer 对象池 — D-5
//
// MITM 路径上的 gzip 操作集中在 ReplaceIdentityInBody（每个 chat 请求都跑：
// decompress → 改 key/JWT/sid → recompress）以及 ExtractJWTFromBody。
//
// 旧实现每次都 gzip.NewReader + gzip.NewWriter：
//   - NewReader 立刻读 10 字节 gzip 头校验，分配内部 deflate state（~32KB
//     window + huffman table）
//   - NewWriter 分配 deflate state（同样 ~32KB） + huffman tree
//
// 长会话场景每分钟几十次 chat 请求 → 每次 ~64KB GC 压力 + 哈夫曼表初始化。
// pool 复用让分配只发生在 cold-start，热路径只有 Reset() 复位 underlying。
//
// 注意：Reset 不会清空 internal buffer，但 io.ReadAll 已读完整内容；Writer
// Close 后 internal state 就绪，下一次 Reset 复位 → 新 underlying writer。
// ═══════════════════════════════════════════════════════════════

var (
	gzipReaderPool sync.Pool // *gzip.Reader
	gzipWriterPool = sync.Pool{
		New: func() any {
			// ★ Writer 必须给 underlying writer 才能 New；先用 io.Discard
			//   占位，每次 Get 后调 Reset(target) 切换。
			return gzip.NewWriter(io.Discard)
		},
	}
)

// gunzipBytes 解压 src 返回新分配的 []byte。
//
// 错误处理：返回非 nil error 时，调用方应当作 src 不是合法 gzip 处理 ——
// 与旧实现 `gzip.NewReader(...) err != nil` 等价。
func gunzipBytes(src []byte) ([]byte, error) {
	v := gzipReaderPool.Get()
	var zr *gzip.Reader
	if v == nil {
		// 第一次：必须用 NewReader 才能初始化（不能 zero-value 后 Reset）。
		var err error
		zr, err = gzip.NewReader(bytes.NewReader(src))
		if err != nil {
			return nil, err
		}
	} else {
		zr = v.(*gzip.Reader)
		if err := zr.Reset(bytes.NewReader(src)); err != nil {
			// Reset 失败说明 src 不是合法 gzip；归还实例（pool 可继续复用）后返回错误。
			gzipReaderPool.Put(zr)
			return nil, err
		}
	}
	out, err := io.ReadAll(zr)
	closeErr := zr.Close()
	gzipReaderPool.Put(zr)
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		// gzip checksum 不匹配也走这；调用方按"非合法 gzip"处理
		return nil, closeErr
	}
	return out, nil
}

// gzipBytes 把 src 压成 gzip 返回新分配的 []byte。
//
// 不返回 error —— gzip.Writer.Write 在标准库实现里只在 underlying writer
// 报错时返回 error，bytes.Buffer 永远不会报错。
func gzipBytes(src []byte) []byte {
	var buf bytes.Buffer
	zw := gzipWriterPool.Get().(*gzip.Writer)
	zw.Reset(&buf)
	_, _ = zw.Write(src)
	_ = zw.Close()
	gzipWriterPool.Put(zw)
	return buf.Bytes()
}
