package services

// provider_transport.go ── 提供商上游传输层。
//
// MITM 入口判定胶囊=providers + chat path 命中后, 把 cascade body 喂给本文件:
//   1. DecodeCascadeChatRequest 拆出 model + messages
//   2. 选 ProviderAccount(同 provider 多卡 round-robin)
//   3. 走 *http.Client(承袭 MitmProxy.upstreamBase 的 Clash/系统代理)
//      发 OpenAI/Anthropic/Gemini 上游请求
//   4. 上游 SSE 流式响应 → cascade gRPC frame 实时回写到 IDE ResponseWriter
//   5. 上游错误 / 流结束 → 写 Connect EOS frame 收尾
//
// 复用同包 cascade_codec.go 的 Encode* helpers — 不在这里直接拼字节。

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/utils"
)

// Router MITM 入口分流时由 App 注入的实现。隔离接口避免 services 反向依赖 store。
type Router interface {
	// RouteMode 当前总览胶囊状态;空 / "pool" → 走号池;"providers" → 提供商分流
	RouteMode() string
	// ActiveAccount 返回当前全局唯一激活的 provider 账号。
	// 整库无激活卡时返回 (zero, false)。
	ActiveAccount() (models.ProviderAccount, bool)
	// Candidates 返回当前激活卡 + 同 active_model 的可用候选(激活卡排第一),
	// 用于上游请求失败时的故障切换重试。无激活卡时返回 nil。
	Candidates() []models.ProviderAccount
}

// RouteOutcome 表示 Route 的处理结果。
type RouteOutcome int

const (
	// RouteServed 已经接管并写完响应(成功流式 或 已写 EOS error frame)。
	RouteServed RouteOutcome = iota
	// RouteFallback 预检未通过且未写任何字节,调用方应回落号池。
	RouteFallback
)

// Route 处理一次 IDE chat 请求 — 整个生命周期都在这里。
//
// w 应该是 MITM ServeHTTP 的 ResponseWriter;cascadeBody 是已剥 envelope+gzip 的
// 原始 protobuf body;httpClient 通常注入 MitmProxy.upstreamBase 包装的 client。
//
// 返回值:
//   - RouteFallback: 预检未通过(解码失败 / 无激活卡 / 无 model)且**未写任何字节**,
//     调用方应还原 body 回落号池,避免点亮 providers 后配置不全就让 IDE 全部失败。
//   - RouteServed: 已经接管。成功时写 200 + cascade 流;运行时失败(候选全部上游不可达 /
//     4xx)时也写 200 + 单个 EOS error frame(IDE 才能正确解析为错误而非连接中断)。
//
// 故障切换:在同 active_model 的候选卡之间逐个尝试,任一返回 200 即开始流式;
// 只有所有候选都失败才落到 EOS error。WriteHeader 推迟到拿到上游 200 才发,
// 这样配置 OK 但首选卡挂掉仍能透明切到下一张,且不会过早占死响应。
func Route(
	ctx context.Context,
	w http.ResponseWriter,
	httpClient *http.Client,
	router Router,
	cascadeBody []byte,
	tracker *UsageTracker,
) RouteOutcome {
	startedAt := time.Now()
	// ── 预检阶段(不写任何字节,失败回落号池)──
	decoded, err := DecodeCascadeChatRequest(cascadeBody)
	if err != nil {
		utils.DLog("[Route] 回落号池: 解 cascade 失败: %v", err)
		return RouteFallback
	}

	candidates := router.Candidates()
	if len(candidates) == 0 {
		// 没有激活卡 / 候选 —— 回落号池,而不是写错误帧把 IDE 卡死
		utils.DLog("[Route] 回落号池: 无可用 provider 候选卡")
		return RouteFallback
	}

	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	// ── 逐候选尝试,拿到首个 200 才写 header ──
	var resp *http.Response
	var usedProvider, usedModel string
	var lastErr string
	for i := range candidates {
		acc := candidates[i]
		provider := strings.TrimSpace(strings.ToLower(acc.Provider))
		if provider == "" {
			lastErr = "active provider 账号 provider 字段为空"
			continue
		}
		model := strings.TrimSpace(acc.ActiveModel)
		if model == "" {
			model = decoded.Model
		}
		if model == "" {
			// 配置类问题:首选卡就没 model 且 IDE 没带 → 回落号池
			if i == 0 {
				utils.DLog("[Route] 回落号池: provider=%s 未设 active_model 且 IDE 未带 model", provider)
				return RouteFallback
			}
			lastErr = fmt.Sprintf("provider=%s 未设 active_model", provider)
			continue
		}

		httpReq, berr := buildProviderHTTPRequest(provider, &acc, model, decoded)
		if berr != nil {
			lastErr = "构造 provider 请求失败: " + berr.Error()
			utils.DLog("[Route] 候选#%d 构造失败: %v", i, berr)
			continue
		}
		httpReq = httpReq.WithContext(ctx)
		utils.DLog("[Route] 候选#%d upstream request: %s %s", i, httpReq.Method, httpReq.URL.String())

		r, derr := httpClient.Do(httpReq)
		if derr != nil {
			lastErr = fmt.Sprintf("provider 上游不可达 [%s]: %v", provider, derr)
			utils.DLog("[Route] 候选#%d 不可达,尝试下一张: %v", i, derr)
			continue
		}
		if r.StatusCode >= 400 {
			body, _ := io.ReadAll(io.LimitReader(r.Body, 2048))
			r.Body.Close()
			lastErr = fmt.Sprintf("provider=%s HTTP %d: %s", provider, r.StatusCode, strings.TrimSpace(string(body)))
			utils.DLog("[Route] 候选#%d HTTP %d,尝试下一张", i, r.StatusCode)
			continue
		}
		// 命中可用上游
		resp = r
		usedProvider = provider
		usedModel = model
		break
	}

	// ── 至此已确定接管(写 header)。固化响应头。──
	w.Header().Set("Content-Type", "application/connect+proto")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	botID := NewCascadeBotID()
	var seq uint64

	writeFrame := func(b []byte) {
		_, _ = w.Write(b)
		if flusher != nil {
			flusher.Flush()
		}
	}
	writeText := func(text string) {
		if text == "" {
			return
		}
		writeFrame(EncodeCascadeDeltaFrame(botID, text, seq, false))
		seq++
	}
	writeEnd := func() {
		writeFrame(EncodeCascadeEOTFrame(botID, seq, false))
		seq++
		writeFrame(EncodeCascadeEOSSuccess())
	}
	writeErr := func(code, message string) {
		writeFrame(EncodeCascadeEOSError(code, message))
	}

	if resp == nil {
		// 所有候选都失败
		if lastErr == "" {
			lastErr = "所有 provider 候选卡均不可用"
		}
		recordProviderUsage(tracker, usedProvider, usedModel, &usageStats{}, startedAt, fmt.Errorf("%s", lastErr))
		writeErr("unavailable", lastErr)
		return RouteServed
	}
	defer resp.Body.Close()
	utils.DLog("[Route] upstream response: HTTP %d, Content-Type=%s", resp.StatusCode, resp.Header.Get("Content-Type"))

	// ── 流式翻译: 上游 SSE delta → cascade text frame(实时 flush)──
	utils.DLog("[Route] 开始流式翻译: provider=%s model=%s", usedProvider, usedModel)
	emitToolCall := newToolCallEmitter(botID, &seq, writeFrame)
	usage := &usageStats{}
	hasToolCalls, serr := streamSSEAsCascade(resp.Body, usedProvider, writeText, emitToolCall, usage)
	// 记录用量到统计页(标记来源=provider-relay,区分号池)。
	recordProviderUsage(tracker, usedProvider, usedModel, usage, startedAt, serr)
	if serr != nil {
		writeErr("internal", "流式解析失败: "+serr.Error())
		return RouteServed
	}

	if hasToolCalls {
		writeFrame(EncodeCascadeEOTFrameToolCalls(botID, seq, false))
		seq++
		writeFrame(EncodeCascadeEOSSuccess())
	} else {
		writeEnd()
	}
	return RouteServed
}

// recordProviderUsage 把一次 provider 转发的用量写进统计页(若 tracker 非空)。
// Format=provider-relay 作为来源标记,与号池的 windsurf-mitm 区分;
// RequestModel 存 provider 名(anthropic/openai/...),Model 存实际模型。
func recordProviderUsage(tracker *UsageTracker, provider, model string, u *usageStats, startedAt time.Time, streamErr error) {
	if tracker == nil {
		return
	}
	status := "ok"
	detail := ""
	if streamErr != nil {
		status = "error"
		detail = streamErr.Error()
	}
	prompt, completion := 0, 0
	if u != nil {
		prompt, completion = u.prompt, u.completion
	}
	tracker.Record(UsageRecord{
		Model:            model,
		RequestModel:     provider, // 来源 provider 名,便于按家分组
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      prompt + completion,
		DurationMs:       time.Since(startedAt).Milliseconds(),
		APIKeyShort:      "provider:" + provider,
		Status:           status,
		ErrorDetail:      detail,
		Format:           "provider-relay",
	})
}

// newToolCallEmitter 返回一个按 tool_call index 维护状态的 emit 回调。
// OpenAI 流式里同一个工具调用的 id/name 只在首 chunk 出现,后续 chunk 只带
// arguments 增量;多个并行 tool_calls 用 index 区分。本 emitter 为每个 index
// 只在其首次出现时发一次"头帧"(id+name),其后只发 args 增量帧,避免多工具串台。
func newToolCallEmitter(botID string, seq *uint64, writeFrame func([]byte)) func(tc OpenAIToolCallDelta) {
	started := map[int]bool{}
	return func(tc OpenAIToolCallDelta) {
		utils.DLog("[Route] tool_call delta: idx=%d id=%q name=%q args=%q", tc.Index, tc.ID, tc.Name, tc.ArgsDelta)
		if (tc.ID != "" || tc.Name != "") && !started[tc.Index] {
			started[tc.Index] = true
			writeFrame(EncodeCascadeToolCallFrame(botID, *seq, tc.ID, tc.Name, ""))
			*seq++
		}
		if tc.ArgsDelta != "" {
			writeFrame(EncodeCascadeToolCallFrame(botID, *seq, "", "", tc.ArgsDelta))
			*seq++
		}
	}
}

// ──────────────────────────────────────────────
// buildProviderHTTPRequest: IR → 三家上游 *http.Request
// ──────────────────────────────────────────────

func buildProviderHTTPRequest(provider string, acc *models.ProviderAccount, model string, ir *CascadeChatRequest) (*http.Request, error) {
	provider = strings.ToLower(provider)
	switch provider {
	case "anthropic":
		return buildAnthropicRequest(acc, model, ir)
	case "google":
		return buildGeminiRequest(acc, model, ir)
	default:
		// openai / deepseek / moonshot / qwen / xai / zhipu / minimax / doubao
		return buildOpenAICompatRequest(acc, model, ir)
	}
}

type openAICompatPayload struct {
	Model    string            `json:"model"`
	Messages []json.RawMessage `json:"messages"`
	Stream   bool              `json:"stream"`
	Tools    []openAITool      `json:"tools,omitempty"`
}

type openAITool struct {
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

type openAIFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

func buildOpenAICompatRequest(acc *models.ProviderAccount, model string, ir *CascadeChatRequest) (*http.Request, error) {
	msgs := buildOpenAIMessages(ir)

	payload := openAICompatPayload{
		Model:    model,
		Messages: msgs,
		Stream:   true,
	}
	for _, t := range ir.Tools {
		var params json.RawMessage
		if t.Schema != "" {
			params = json.RawMessage(t.Schema)
		}
		payload.Tools = append(payload.Tools, openAITool{
			Type: "function",
			Function: openAIFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		})
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	endpoint := providerChatEndpoint(acc.BaseURL)
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+acc.AuthToken)
	req.Header.Set("Accept", "text/event-stream")
	return req, nil
}

// buildOpenAIMessages 将 CascadeMessage 列表转为 OpenAI Chat 格式的 JSON 消息数组。
func buildOpenAIMessages(ir *CascadeChatRequest) []json.RawMessage {
	var msgs []json.RawMessage
	if strings.TrimSpace(ir.System) != "" {
		m, _ := json.Marshal(map[string]string{"role": "system", "content": ir.System})
		msgs = append(msgs, m)
	}
	for _, cm := range ir.Messages {
		switch cm.Role {
		case "assistant":
			msg := map[string]interface{}{"role": "assistant"}
			if len(cm.ToolUses) > 0 {
				// 带 tool_calls 时,content 用空串而非 null —— 部分 OpenAI 兼容网关
				// (qwen/zhipu/minimax 等)不接受 content:null + tool_calls 组合会 400。
				msg["content"] = cm.Content
				var toolCalls []map[string]interface{}
				for _, tu := range cm.ToolUses {
					toolCalls = append(toolCalls, map[string]interface{}{
						"id":   tu.ID,
						"type": "function",
						"function": map[string]string{
							"name":      tu.Name,
							"arguments": tu.Input,
						},
					})
				}
				msg["tool_calls"] = toolCalls
			} else {
				msg["content"] = cm.Content
			}
			m, _ := json.Marshal(msg)
			msgs = append(msgs, m)
		case "tool":
			content := cm.Content
			if content == "" {
				// tool 消息 content 不能为空,否则部分网关拒收
				content = " "
			}
			msg := map[string]string{
				"role":         "tool",
				"content":      content,
				"tool_call_id": cm.ToolUseID,
			}
			m, _ := json.Marshal(msg)
			msgs = append(msgs, m)
		default:
			m, _ := json.Marshal(map[string]string{"role": "user", "content": cm.Content})
			msgs = append(msgs, m)
		}
	}
	return msgs
}

type anthropicPayload struct {
	Model    string          `json:"model"`
	System   string          `json:"system,omitempty"`
	Messages json.RawMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Tools    []anthropicTool `json:"tools,omitempty"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

func buildAnthropicRequest(acc *models.ProviderAccount, model string, ir *CascadeChatRequest) (*http.Request, error) {
	msgs := buildAnthropicMessages(ir)
	if len(msgs) == 0 {
		return nil, fmt.Errorf("anthropic: messages 解码后为空")
	}

	msgsJSON, err := json.Marshal(msgs)
	if err != nil {
		return nil, err
	}

	payload := anthropicPayload{
		Model:    model,
		System:   ir.System,
		Messages: msgsJSON,
		Stream:   true,
	}
	for _, t := range ir.Tools {
		schema := json.RawMessage(`{"type":"object","properties":{}}`)
		if t.Schema != "" {
			schema = json.RawMessage(t.Schema)
		}
		payload.Tools = append(payload.Tools, anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schema,
		})
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	endpoint := anthropicMessagesEndpoint(acc.BaseURL)
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", acc.AuthToken)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Accept", "text/event-stream")
	return req, nil
}

// buildAnthropicMessages 将 CascadeMessage 转为 Anthropic Messages API 格式。
// assistant: content=[{type:text},{type:tool_use,id,name,input}]
// user/tool_result: content=[{type:text}] 或 content=[{type:tool_result,tool_use_id,content}]
//
// Anthropic 要求 messages 首条必须是 user 且 role 严格交替,否则 400。cascade 历史
// 里常出现连续 user(多个 tool_result 拆成独立消息)或以 tool 结尾,这里把连续
// 同 role 的 content 合并进同一条消息,并保证首条为 user。
func buildAnthropicMessages(ir *CascadeChatRequest) []map[string]interface{} {
	type pending struct {
		role    string
		content []map[string]interface{}
	}
	var merged []pending
	push := func(role string, blocks ...map[string]interface{}) {
		if n := len(merged); n > 0 && merged[n-1].role == role {
			merged[n-1].content = append(merged[n-1].content, blocks...)
			return
		}
		merged = append(merged, pending{role: role, content: append([]map[string]interface{}{}, blocks...)})
	}

	for _, cm := range ir.Messages {
		if cm.Role == "system" {
			continue
		}
		switch cm.Role {
		case "assistant":
			var blocks []map[string]interface{}
			if cm.Content != "" {
				blocks = append(blocks, map[string]interface{}{"type": "text", "text": cm.Content})
			}
			for _, tu := range cm.ToolUses {
				var input interface{}
				if err := json.Unmarshal([]byte(tu.Input), &input); err != nil {
					input = map[string]interface{}{}
				}
				blocks = append(blocks, map[string]interface{}{
					"type":  "tool_use",
					"id":    tu.ID,
					"name":  tu.Name,
					"input": input,
				})
			}
			if len(blocks) == 0 {
				blocks = append(blocks, map[string]interface{}{"type": "text", "text": ""})
			}
			push("assistant", blocks...)
		case "tool":
			push("user", map[string]interface{}{
				"type":        "tool_result",
				"tool_use_id": cm.ToolUseID,
				"content":     cm.Content,
			})
		default:
			push("user", map[string]interface{}{"type": "text", "text": cm.Content})
		}
	}

	// 保证首条为 user(若历史以 assistant 起头,Anthropic 会拒)。
	if len(merged) > 0 && merged[0].role != "user" {
		merged = append([]pending{{
			role:    "user",
			content: []map[string]interface{}{{"type": "text", "text": ""}},
		}}, merged...)
	}

	msgs := make([]map[string]interface{}, 0, len(merged))
	for _, p := range merged {
		msgs = append(msgs, map[string]interface{}{"role": p.role, "content": p.content})
	}
	return msgs
}

type geminiPayload struct {
	Contents          []geminiContent       `json:"contents"`
	SystemInstruction *geminiSystemInstruct `json:"systemInstruction,omitempty"`
	Tools             []geminiTool          `json:"tools,omitempty"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDecl `json:"functionDeclarations"`
}

type geminiFunctionDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type geminiContent struct {
	Role  string              `json:"role"`
	Parts []geminiContentPart `json:"parts"`
}

// geminiContentPart 一个 part 只填其中一种:text / functionCall / functionResponse。
type geminiContentPart struct {
	Text             string                  `json:"text,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
}

type geminiFunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

type geminiFunctionResponse struct {
	Name     string      `json:"name"`
	Response interface{} `json:"response"`
}

type geminiSystemInstruct struct {
	Parts []geminiContentPart `json:"parts"`
}

func buildGeminiRequest(acc *models.ProviderAccount, model string, ir *CascadeChatRequest) (*http.Request, error) {
	// tool_use_id → function name 映射,供 tool result 还原 functionResponse.name 用
	// (Gemini functionResponse 要求 name,而 cascade tool 消息只带 tool_use_id)。
	toolName := map[string]string{}
	for _, m := range ir.Messages {
		for _, tu := range m.ToolUses {
			if tu.ID != "" {
				toolName[tu.ID] = tu.Name
			}
		}
	}

	// 合并连续同 role,并保证首条为 user(Gemini 要求 user/model 交替、首条 user)。
	contents := make([]geminiContent, 0, len(ir.Messages))
	pushPart := func(role string, part geminiContentPart) {
		if n := len(contents); n > 0 && contents[n-1].Role == role {
			contents[n-1].Parts = append(contents[n-1].Parts, part)
			return
		}
		contents = append(contents, geminiContent{Role: role, Parts: []geminiContentPart{part}})
	}

	for _, m := range ir.Messages {
		switch m.Role {
		case "system":
			continue
		case "assistant":
			if m.Content != "" {
				pushPart("model", geminiContentPart{Text: m.Content})
			}
			for _, tu := range m.ToolUses {
				var args json.RawMessage
				if strings.TrimSpace(tu.Input) != "" {
					args = json.RawMessage(tu.Input)
				}
				pushPart("model", geminiContentPart{
					FunctionCall: &geminiFunctionCall{Name: tu.Name, Args: args},
				})
			}
		case "tool":
			var respObj interface{}
			if err := json.Unmarshal([]byte(m.Content), &respObj); err != nil {
				respObj = map[string]interface{}{"result": m.Content}
			}
			name := toolName[m.ToolUseID]
			pushPart("user", geminiContentPart{
				FunctionResponse: &geminiFunctionResponse{Name: name, Response: respObj},
			})
		default:
			pushPart("user", geminiContentPart{Text: m.Content})
		}
	}
	if len(contents) == 0 {
		return nil, fmt.Errorf("gemini: contents 解码后为空")
	}
	if contents[0].Role != "user" {
		contents = append([]geminiContent{{Role: "user", Parts: []geminiContentPart{{Text: ""}}}}, contents...)
	}

	payload := geminiPayload{Contents: contents}
	if strings.TrimSpace(ir.System) != "" {
		payload.SystemInstruction = &geminiSystemInstruct{
			Parts: []geminiContentPart{{Text: ir.System}},
		}
	}
	if len(ir.Tools) > 0 {
		decls := make([]geminiFunctionDecl, 0, len(ir.Tools))
		for _, t := range ir.Tools {
			var params json.RawMessage
			if strings.TrimSpace(t.Schema) != "" {
				params = json.RawMessage(t.Schema)
			}
			decls = append(decls, geminiFunctionDecl{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			})
		}
		payload.Tools = []geminiTool{{FunctionDeclarations: decls}}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	// Gemini 流式必须用 streamGenerateContent + alt=sse
	endpoint := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?key=%s&alt=sse",
		strings.TrimRight(acc.BaseURL, "/"),
		url.PathEscape(model),
		url.QueryEscape(acc.AuthToken),
	)
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	return req, nil
}

// ──────────────────────────────────────────────
// streamSSEAsCascade: 三家流式响应 → 文本 delta 回调
// ──────────────────────────────────────────────

// streamSSEAsCascade 读上游 SSE 流, 调 emit(text) 把每段 delta 喂给 caller(由
// caller 编 cascade 帧 flush)。三家 SSE 字段名不同, 内部分支处理。
//
// emitToolCall 在识别到工具调用时被调用:
//   - 首帧(id+name 非空): 工具调用头
//   - 后续帧(argsDelta 非空): JSON input 增量
//
// 返回 hasToolCalls=true 表示本轮包含了工具调用。
//
// 终止信号(三家不同):
//   - OpenAI 兼容: data: [DONE]
//   - Anthropic: event: message_stop
//   - Gemini: 上游主动 close body(SSE 流自然结束)
func streamSSEAsCascade(body io.Reader, provider string, emit func(text string), emitToolCall func(tc OpenAIToolCallDelta), usage *usageStats) (hasToolCalls bool, err error) {
	provider = strings.ToLower(provider)
	scanner := bufio.NewScanner(body)
	// 单条 SSE data 可能很大(长 tool args / 大 JSON),上限给到 32MB 避免 ErrTooLong 截断流。
	scanner.Buffer(make([]byte, 0, 64*1024), 32*1024*1024)

	lineCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(line[5:])
		if data == "" || data == "[DONE]" {
			if data == "[DONE]" {
				utils.DLog("[SSE] [DONE] received, lineCount=%d hasToolCalls=%v", lineCount, hasToolCalls)
				return hasToolCalls, nil
			}
			continue
		}
		lineCount++
		// 前 5 条和含 tool_calls 的记录完整 data
		if lineCount <= 5 {
			utils.DLog("[SSE] line#%d data=%s", lineCount, truncateForLog(data, 500))
		}
		if usage != nil {
			usage.scan(provider, data)
		}

		switch provider {
		case "anthropic":
			if isAnthropicStopEvent(data) {
				utils.DLog("[SSE] anthropic message_stop, lineCount=%d", lineCount)
				return hasToolCalls, nil
			}
			d := parseAnthropicSSEDeltaFull(data)
			if d.Text != "" {
				emit(d.Text)
			}
			if d.Thinking != "" {
				// cascade 协议无独立 thinking 帧,已知限制:thinking 内容不透传给 IDE。
				utils.DLog("[SSE] anthropic thinking_delta 丢弃(cascade 无对应帧): len=%d", len(d.Thinking))
			}
			if d.ToolStart != nil {
				hasToolCalls = true
				utils.DLog("[SSE] anthropic tool_use start: id=%q name=%q", d.ToolStart.ID, d.ToolStart.Name)
				if emitToolCall != nil {
					emitToolCall(OpenAIToolCallDelta{ID: d.ToolStart.ID, Name: d.ToolStart.Name})
				}
			}
			if d.ToolDelta != "" {
				hasToolCalls = true
				if emitToolCall != nil {
					emitToolCall(OpenAIToolCallDelta{ArgsDelta: d.ToolDelta})
				}
			}
		case "google":
			text, toolCalls := parseGeminiSSEDeltaFull(data)
			if text != "" {
				emit(text)
			}
			for _, tc := range toolCalls {
				hasToolCalls = true
				utils.DLog("[SSE] gemini functionCall: name=%q argsLen=%d", tc.Name, len(tc.ArgsDelta))
				if emitToolCall != nil {
					// Gemini 一次给出完整 functionCall(非增量),头帧带 name + 全量 args。
					emitToolCall(OpenAIToolCallDelta{Index: tc.Index, ID: tc.ID, Name: tc.Name})
					if tc.ArgsDelta != "" {
						emitToolCall(OpenAIToolCallDelta{Index: tc.Index, ArgsDelta: tc.ArgsDelta})
					}
				}
			}
		default:
			d := parseOpenAISSEDeltaFull(data)
			if d.Content != "" {
				emit(d.Content)
			}
			for _, tc := range d.ToolCalls {
				hasToolCalls = true
				utils.DLog("[SSE] tool_call in SSE: idx=%d id=%q name=%q argsLen=%d", tc.Index, tc.ID, tc.Name, len(tc.ArgsDelta))
				if emitToolCall != nil {
					emitToolCall(tc)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		utils.DLog("[SSE] scanner error: %v", err)
		return hasToolCalls, err
	}
	utils.DLog("[SSE] stream ended naturally, lineCount=%d hasToolCalls=%v", lineCount, hasToolCalls)
	return hasToolCalls, nil
}

func truncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// usageStats 累积上游 SSE 流里的真实 token 用量(三家字段不同)。
type usageStats struct {
	prompt     int
	completion int
}

// scan 从一条 SSE data JSON 里抽取 usage,累积到 stats。
//   - Anthropic: message_start.message.usage.input_tokens(prompt) +
//     message_delta.usage.output_tokens(completion,流末尾给最终值)
//   - OpenAI 兼容: 末尾 chunk.usage.{prompt_tokens,completion_tokens}
//   - Gemini: usageMetadata.{promptTokenCount,candidatesTokenCount}
func (u *usageStats) scan(provider, data string) {
	switch provider {
	case "anthropic":
		var p struct {
			Type    string `json:"type"`
			Message struct {
				Usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			} `json:"message"`
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal([]byte(data), &p) != nil {
			return
		}
		if p.Message.Usage.InputTokens > 0 {
			u.prompt = p.Message.Usage.InputTokens
		}
		if p.Usage.InputTokens > 0 {
			u.prompt = p.Usage.InputTokens
		}
		// output_tokens 在 message_delta 里逐步给到最终值,取最大
		if p.Message.Usage.OutputTokens > u.completion {
			u.completion = p.Message.Usage.OutputTokens
		}
		if p.Usage.OutputTokens > u.completion {
			u.completion = p.Usage.OutputTokens
		}
	case "google":
		var p struct {
			UsageMetadata struct {
				PromptTokenCount     int `json:"promptTokenCount"`
				CandidatesTokenCount int `json:"candidatesTokenCount"`
			} `json:"usageMetadata"`
		}
		if json.Unmarshal([]byte(data), &p) != nil {
			return
		}
		if p.UsageMetadata.PromptTokenCount > 0 {
			u.prompt = p.UsageMetadata.PromptTokenCount
		}
		if p.UsageMetadata.CandidatesTokenCount > u.completion {
			u.completion = p.UsageMetadata.CandidatesTokenCount
		}
	default: // openai 兼容
		var p struct {
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal([]byte(data), &p) != nil || p.Usage == nil {
			return
		}
		if p.Usage.PromptTokens > 0 {
			u.prompt = p.Usage.PromptTokens
		}
		if p.Usage.CompletionTokens > u.completion {
			u.completion = p.Usage.CompletionTokens
		}
	}
}

// OpenAIToolCallDelta 表示流式 SSE 中的一个工具调用增量。
type OpenAIToolCallDelta struct {
	Index     int    // tool_calls 数组下标(通常 0)
	ID        string // 首帧携带 tool call id
	Name      string // 首帧携带函数名
	ArgsDelta string // 后续帧的 arguments 增量
}

// OpenAIDelta 表示一个 OpenAI SSE chunk 中的 delta 字段解析结果。
type OpenAIDelta struct {
	Content   string
	ToolCalls []OpenAIToolCallDelta
}

// parseOpenAISSEDeltaFull 解析 OpenAI Chat Completions SSE chunk，
// 同时提取 content 和 tool_calls。
func parseOpenAISSEDeltaFull(data string) OpenAIDelta {
	var payload struct {
		Choices []struct {
			Delta struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					Index    int    `json:"index"`
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return OpenAIDelta{}
	}
	if len(payload.Choices) == 0 {
		return OpenAIDelta{}
	}
	delta := payload.Choices[0].Delta
	result := OpenAIDelta{Content: delta.Content}
	for _, tc := range delta.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, OpenAIToolCallDelta{
			Index:     tc.Index,
			ID:        tc.ID,
			Name:      tc.Function.Name,
			ArgsDelta: tc.Function.Arguments,
		})
	}
	return result
}

// parseOpenAISSEDelta 向后兼容 — 仅返回 content 文本。
func parseOpenAISSEDelta(data string) string {
	d := parseOpenAISSEDeltaFull(data)
	return d.Content
}

// Anthropic Messages SSE: 多种 event 类型;只关心 content_block_delta.delta.text
// AnthropicDelta 表示 Anthropic SSE 事件解析结果。
type AnthropicDelta struct {
	Text      string              // text_delta 文本
	Thinking  string              // thinking_delta 文本
	ToolStart *AnthropicToolStart // content_block_start type=tool_use
	ToolDelta string              // input_json_delta 增量 JSON
}

type AnthropicToolStart struct {
	ID   string
	Name string
}

// parseAnthropicSSEDeltaFull 完整解析 Anthropic SSE 事件,
// 包括 text_delta / thinking_delta / tool_use start / input_json_delta。
func parseAnthropicSSEDeltaFull(data string) AnthropicDelta {
	var payload struct {
		Type         string `json:"type"`
		Index        int    `json:"index"`
		ContentBlock struct {
			Type string `json:"type"`
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"content_block"`
		Delta struct {
			Type        string `json:"type"`
			Text        string `json:"text"`
			Thinking    string `json:"thinking"`
			PartialJSON string `json:"partial_json"`
		} `json:"delta"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return AnthropicDelta{}
	}

	switch payload.Type {
	case "content_block_start":
		if payload.ContentBlock.Type == "tool_use" {
			return AnthropicDelta{
				ToolStart: &AnthropicToolStart{
					ID:   payload.ContentBlock.ID,
					Name: payload.ContentBlock.Name,
				},
			}
		}
	case "content_block_delta":
		switch payload.Delta.Type {
		case "text_delta":
			return AnthropicDelta{Text: payload.Delta.Text}
		case "thinking_delta":
			return AnthropicDelta{Thinking: payload.Delta.Thinking}
		case "input_json_delta":
			return AnthropicDelta{ToolDelta: payload.Delta.PartialJSON}
		}
	}
	return AnthropicDelta{}
}

func parseAnthropicSSEDelta(data string) string {
	d := parseAnthropicSSEDeltaFull(data)
	return d.Text
}

// isAnthropicStopEvent 检查 Anthropic SSE data 是否为 message_stop 终止帧。
// 流终止后即便上游不主动 close body 我们也能立即收尾, 避免 IDE 干等到超时。
func isAnthropicStopEvent(data string) bool {
	var payload struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return false
	}
	return payload.Type == "message_stop"
}

// Gemini: { "candidates": [ {"content": {"parts": [{"text": "..."}]}} ] }
func parseGeminiSSEDelta(data string) string {
	text, _ := parseGeminiSSEDeltaFull(data)
	return text
}

// parseGeminiSSEDeltaFull 解析 Gemini streamGenerateContent SSE chunk,
// 同时提取 text 与 functionCall(后者 Gemini 一次给全量,非增量)。
func parseGeminiSSEDeltaFull(data string) (string, []OpenAIToolCallDelta) {
	var payload struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text         string `json:"text"`
					FunctionCall *struct {
						Name string          `json:"name"`
						Args json.RawMessage `json:"args"`
					} `json:"functionCall"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return "", nil
	}
	if len(payload.Candidates) == 0 {
		return "", nil
	}
	parts := payload.Candidates[0].Content.Parts
	var sb strings.Builder
	var calls []OpenAIToolCallDelta
	idx := 0
	for _, p := range parts {
		if p.Text != "" {
			sb.WriteString(p.Text)
		}
		if p.FunctionCall != nil {
			args := ""
			if len(p.FunctionCall.Args) > 0 {
				args = string(p.FunctionCall.Args)
			}
			calls = append(calls, OpenAIToolCallDelta{
				Index:     idx,
				Name:      p.FunctionCall.Name,
				ArgsDelta: args,
			})
			idx++
		}
	}
	return sb.String(), calls
}

// ──────────────────────────────────────────────
// providerHTTPClient — 拿一个能复用 MitmProxy 上游代理的 client
// ──────────────────────────────────────────────

// routeTimeout 单次 IDE → provider 请求总超时(ctx 用)。
// 流式响应可能跑到 3min, 不能设太短。
const routeTimeout = 3 * time.Minute
