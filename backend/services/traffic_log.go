package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// trafficLogger 记录 MITM 代理经过的所有 HTTP 请求/响应到文件。
//
// 异步设计（P2）：
//   - 旧实现每条 trafficLog 调用都 `Sync()` 强制刷盘，SSD 上 5-10ms、
//     HDD 上 50-200ms，加全局互斥锁后串行化，是 chat TTFB 的次要拖累。
//   - 新实现：调用方把行推入 buffered chan（微秒级），后台 worker 批量
//     write + 定时 fsync。通道满则 drop（traffic log 仅诊断用，丢可接受）。
//   - 生产路径在 MitmProxy.Start() 显式调用 EnableTrafficLogAsync() 启动
//     worker；测试路径不启用 → 走同步 fallback，保持测试可观察性。
var (
	trafficLogMu   sync.Mutex
	trafficLogFile *os.File
	trafficLogPath string
	trafficSeq     atomic.Uint64

	trafficCh      chan string
	trafficStopCh  chan struct{}
	trafficWorker  sync.Once
	trafficDropped atomic.Uint64
)

const (
	trafficChanCap     = 1024
	trafficFlushPeriod = 200 * time.Millisecond
	trafficBatchMax    = 64
)

// TrafficLogPath 返回流量日志文件路径
func TrafficLogPath() string {
	trafficLogMu.Lock()
	defer trafficLogMu.Unlock()
	return trafficLogPath
}

func initTrafficLog() {
	trafficLogMu.Lock()
	defer trafficLogMu.Unlock()
	if trafficLogFile != nil {
		return
	}
	dir, _ := os.UserConfigDir()
	trafficLogPath = filepath.Join(dir, "WindsurfTools", "traffic.log")
	os.MkdirAll(filepath.Dir(trafficLogPath), 0755)
	f, err := os.OpenFile(trafficLogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return
	}
	trafficLogFile = f
	fmt.Fprintf(f, "=== Traffic Log Started %s ===\n", time.Now().Format(time.RFC3339))
}

// EnableTrafficLogAsync 启动后台 worker，把同步刷盘改成批量异步。
// 由 MitmProxy.Start() 调用一次；多次调用幂等。测试路径不调用 → 同步 fallback。
func EnableTrafficLogAsync() {
	trafficWorker.Do(func() {
		trafficLogMu.Lock()
		trafficCh = make(chan string, trafficChanCap)
		trafficStopCh = make(chan struct{})
		trafficLogMu.Unlock()
		go trafficLogWorker()
	})
}

func trafficLogWorker() {
	ticker := time.NewTicker(trafficFlushPeriod)
	defer ticker.Stop()
	batch := make([]string, 0, trafficBatchMax)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		trafficLogMu.Lock()
		f := trafficLogFile
		trafficLogMu.Unlock()
		if f != nil {
			for _, line := range batch {
				_, _ = f.WriteString(line)
			}
			_ = f.Sync()
		}
		batch = batch[:0]
	}

	for {
		select {
		case line, ok := <-trafficCh:
			if !ok {
				flush()
				return
			}
			batch = append(batch, line)
			if len(batch) >= trafficBatchMax {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-trafficStopCh:
			flush()
			return
		}
	}
}

func trafficLog(format string, args ...interface{}) {
	initTrafficLog()
	seq := trafficSeq.Add(1)
	line := fmt.Sprintf("#%04d [%s] %s\n", seq, time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))

	// 异步路径：生产代码在 Start() 显式启用；通道满则 drop（诊断 log 可丢失）
	if trafficCh != nil {
		select {
		case trafficCh <- line:
		default:
			trafficDropped.Add(1)
		}
		return
	}

	// 同步 fallback：未启用 async（测试或 init 失败）
	trafficLogMu.Lock()
	defer trafficLogMu.Unlock()
	if trafficLogFile != nil {
		_, _ = trafficLogFile.WriteString(line)
		_ = trafficLogFile.Sync()
	}
}

func shouldCaptureTrafficPath(path string) bool {
	path = strings.ToLower(strings.TrimSpace(path))
	if path == "" {
		return false
	}
	return strings.Contains(path, "getchatmessage") || strings.Contains(path, "getcompletions")
}

func sanitizePathForFile(s string) string {
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	s = strings.ReplaceAll(s, ":", "_")
	s = strings.ReplaceAll(s, "?", "_")
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}

// TrafficDumpBody dump 响应体到文件，返回文件路径
func TrafficDumpBody(seq int, suffix string, data []byte) string {
	dir, _ := os.UserConfigDir()
	dumpDir := filepath.Join(dir, "WindsurfTools", "traffic_dumps")
	os.MkdirAll(dumpDir, 0755)
	fname := fmt.Sprintf("%04d_%s.bin", seq, suffix)
	fpath := filepath.Join(dumpDir, fname)
	os.WriteFile(fpath, data, 0644)
	return fpath
}
