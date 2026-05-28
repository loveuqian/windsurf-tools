package services

// clash_rotator_state.go ── ClashRotator「原始节点」的磁盘持久化。
//
// 动机:rotator 把 selector 组切到别的出口节点后,只有进程正常退出走 Stop() 才会
// 切回原节点。kill -9 / 崩溃 / 断电时 Stop 不执行,节点永久停在最后切到的出口,且
// 原节点信息只存在内存 → 下次启动也无从恢复。
//
// 方案:Start 捕获原节点后把 {group,node} 落盘;Stop 成功恢复后删除。进程下次启动时
// 调 RecoverResidualClashNode:若磁盘仍有残留记录(说明上次没正常恢复),先把组切回
// 该节点再删文件。与 hosts 的 .bak 兜底对齐。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type clashRotatorState struct {
	Group string `json:"group"`
	Node  string `json:"node"`
}

// persistState 把待恢复的 {group,node} 写到 StateFile(StateFile 为空则 no-op)。
func (r *ClashRotator) persistState(group, node string) {
	if r.cfg.StateFile == "" || group == "" || node == "" {
		return
	}
	b, err := json.Marshal(clashRotatorState{Group: group, Node: node})
	if err != nil {
		return
	}
	if err := os.WriteFile(r.cfg.StateFile, b, 0644); err != nil {
		r.log("持久化原始节点失败: %v", err)
	}
}

// clearState 删除 StateFile(StateFile 为空则 no-op)。
func (r *ClashRotator) clearState() {
	if r.cfg.StateFile == "" {
		return
	}
	_ = os.Remove(r.cfg.StateFile)
}

// readClashState 读 StateFile,返回 (group, node, ok)。
func readClashState(stateFile string) (string, string, bool) {
	if strings.TrimSpace(stateFile) == "" {
		return "", "", false
	}
	b, err := os.ReadFile(stateFile)
	if err != nil {
		return "", "", false
	}
	var st clashRotatorState
	if err := json.Unmarshal(b, &st); err != nil {
		return "", "", false
	}
	if strings.TrimSpace(st.Group) == "" || strings.TrimSpace(st.Node) == "" {
		return "", "", false
	}
	return st.Group, st.Node, true
}

// RecoverResidualClashNode 在进程启动时调用一次:若 StateFile 残留(上次未正常恢复),
// 把 Clash selector 组切回记录里的原节点,然后删掉文件。无残留 / controller 不可达时
// 静默返回。在 ClashRotator 再次 Start 之前调用,避免与新一轮捕获/切换竞争。
func RecoverResidualClashNode(controllerURL, secret, stateFile string, logFn func(string)) {
	group, node, ok := readClashState(stateFile)
	if !ok {
		return
	}
	log := func(format string, args ...interface{}) {
		if logFn != nil {
			logFn("[Clash] " + fmt.Sprintf(format, args...))
		}
	}
	if strings.TrimSpace(controllerURL) == "" {
		// 没有 controller 地址无法恢复,但保留文件等下次有地址时再试。
		log("发现残留节点记录但 controller_url 为空,暂不恢复: group=%s node=%s", group, node)
		return
	}
	tmp := &ClashRotator{
		cfg:   ClashRotatorConfig{ControllerURL: controllerURL, Secret: secret},
		httpc: &http.Client{Timeout: 5 * time.Second},
	}
	// 当前已是目标节点则只清文件;否则切回。
	if _, current, err := tmp.listGroupNodes(group); err == nil && current == node {
		log("启动恢复:残留节点已是当前节点 (%s),清理记录", node)
		_ = os.Remove(stateFile)
		return
	}
	if err := tmp.switchTo(group, node); err != nil {
		log("启动恢复失败(保留记录下次再试) group=%s node=%s: %v", group, node, err)
		return
	}
	log("✓ 启动恢复:已把 %s 切回上次原始节点 %s", group, node)
	_ = os.Remove(stateFile)
}
