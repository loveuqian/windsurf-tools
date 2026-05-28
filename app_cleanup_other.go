//go:build !windows

package main

import (
	"os/exec"
	"strconv"
	"strings"
)

// getWindsurfProcesses 用 ps 列举当前用户运行中的 Windsurf 相关进程。
// 字段对齐 Windows 实现 + 前端 Cleanup 页期望：pid/name/cpu_percent/memory_bytes。
//
// macOS 上 Windsurf 会拉起多个子进程（GPU/Renderer/Helper），都用 grep 兜住。
func getWindsurfProcesses() []map[string]interface{} {
	// ps -axo pid=,pcpu=,rss=,comm=  (rss 是 KB；comm 是可执行路径或名)
	out, err := exec.Command("ps", "-axo", "pid=,pcpu=,rss=,comm=").Output()
	if err != nil {
		return nil
	}

	var results []map[string]interface{}
	var totalBytes int64

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if !strings.Contains(lower, "windsurf") {
			continue
		}
		// 排除 windsurf-tools 本身（即用户当前在用的工具应用）避免自我列出。
		if strings.Contains(lower, "windsurf-tools") {
			continue
		}
		// 用 Fields 切：前 3 个是 pid/pcpu/rss，剩下全部 join 回 comm（路径含空格罕见但兜底）
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		pid, _ := strconv.Atoi(fields[0])
		cpu, _ := strconv.ParseFloat(fields[1], 64)
		rssKB, _ := strconv.ParseInt(fields[2], 10, 64)
		comm := strings.Join(fields[3:], " ")

		// 仅保留 basename，避免长路径撑爆 UI
		name := comm
		if i := strings.LastIndex(comm, "/"); i >= 0 {
			name = comm[i+1:]
		}

		memBytes := rssKB * 1024
		totalBytes += memBytes
		results = append(results, map[string]interface{}{
			"pid":          pid,
			"name":         name,
			"cpu_percent":  cpu,
			"memory_bytes": memBytes,
		})
	}

	if len(results) > 0 {
		// 汇总行（前端按字段渲染即可，pid=0 用作 sentinel）
		results = append(results, map[string]interface{}{
			"pid":          0,
			"name":         "== 合计 ==",
			"cpu_percent":  0.0,
			"memory_bytes": totalBytes,
		})
	}
	return results
}
