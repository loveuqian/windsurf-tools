//go:build windows

package main

import (
	"os/exec"
	"strconv"
	"strings"
)

// getWindsurfProcesses 获取 Windsurf 相关进程的内存占用
func getWindsurfProcesses() []map[string]interface{} {
	// tasklist /FI "IMAGENAME eq Windsurf.exe" /FO CSV /NH
	out, err := exec.Command("tasklist", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return nil
	}

	var results []map[string]interface{}
	var totalMem int64

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if !strings.Contains(lower, "windsurf") {
			continue
		}
		// CSV: "Image Name","PID","Session Name","Session#","Mem Usage"
		fields := parseCSVLine(line)
		if len(fields) < 5 {
			continue
		}
		name := fields[0]
		pidInt, _ := strconv.Atoi(strings.TrimSpace(fields[1]))
		memStr := strings.ReplaceAll(fields[4], ",", "")
		memStr = strings.ReplaceAll(memStr, ".", "")
		memStr = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(memStr), "K"))
		memKB, _ := strconv.ParseInt(memStr, 10, 64)

		memBytes := memKB * 1024
		totalMem += memBytes
		results = append(results, map[string]interface{}{
			"pid":          pidInt,
			"name":         name,
			"cpu_percent":  0.0, // tasklist 不直接给 CPU%；保留 0 不至于让前端渲染崩
			"memory_bytes": memBytes,
		})
	}

	if len(results) > 0 {
		// 汇总行
		results = append(results, map[string]interface{}{
			"pid":          0,
			"name":         "== 合计 ==",
			"cpu_percent":  0.0,
			"memory_bytes": totalMem,
		})
	}
	return results
}

func parseCSVLine(line string) []string {
	var fields []string
	inQuote := false
	var current strings.Builder
	for _, ch := range line {
		switch {
		case ch == '"':
			inQuote = !inQuote
		case ch == ',' && !inQuote:
			fields = append(fields, current.String())
			current.Reset()
		default:
			current.WriteRune(ch)
		}
	}
	fields = append(fields, current.String())
	return fields
}
