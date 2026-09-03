package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"mcp-chat/internal/config"
	"mcp-chat/internal/llm"
	"mcp-chat/internal/mcp"

	"github.com/gin-gonic/gin"
)

// sessionStore 内存会话历史存储（单用户，无需跨进程持久化）
type sessionStore struct {
	mu   sync.Mutex
	data map[string][]llm.ChatMessage
}

func newSessionStore() *sessionStore {
	return &sessionStore{data: make(map[string][]llm.ChatMessage)}
}

// get 返回会话历史（user/assistant 消息，不含 system）
func (s *sessionStore) get(sessionID string) []llm.ChatMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 返回副本，避免外部修改影响存储
	h := s.data[sessionID]
	if h == nil {
		return nil
	}
	out := make([]llm.ChatMessage, len(h))
	copy(out, h)
	return out
}

// append 向会话历史追加一条消息并保存
func (s *sessionStore) append(sessionID string, msg llm.ChatMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 限制单会话历史长度，防止无限增长
	h := s.data[sessionID]
	if len(h) >= 40 {
		h = h[len(h)-38:]
	}
	s.data[sessionID] = append(h, msg)
}

// delete 删除指定会话的历史
func (s *sessionStore) delete(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, sessionID)
}

// newSessionID 生成随机会话 ID
func newSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// ChatHandler 聊天处理器
type ChatHandler struct {
	Cfg       *config.Config
	MCPSrvs   []*mcp.Client
	LLMClient *llm.Client
	Tools     []llm.ToolDefinition
	sessions  *sessionStore
}

// NewChatHandler 创建聊天处理器
// serversJSON 为 embed 的 mcp_servers.json 字节内容
func NewChatHandler(cfg *config.Config, serversJSON []byte) *ChatHandler {
	llmEnableProxy := true
	if cfg.LLM.EnableProxy != nil {
		llmEnableProxy = *cfg.LLM.EnableProxy
	}
	llmVerifySSL := true
	if cfg.LLM.VerifySSL != nil {
		llmVerifySSL = *cfg.LLM.VerifySSL
	}

	mcpSrvs, err := mcp.NewClientFromConfig(cfg, serversJSON)
	if err != nil {
		slog.Error("handler_mcp_servers_load_failed", "error", err)
		mcpSrvs = []*mcp.Client{}
	}

	h := &ChatHandler{
		Cfg:       cfg,
		MCPSrvs:   mcpSrvs,
		LLMClient: llm.New(cfg.LLM.APIURI, cfg.LLM.APIKey, cfg.LLM.ModelName, cfg.LLM.MaxTokens, llmEnableProxy, llmVerifySSL),
		sessions:  newSessionStore(),
	}

	// 启动时初始化所有 MCP 连接并获取工具列表
	h.RefreshTools()
	return h
}

// RefreshTools 刷新工具列表
func (h *ChatHandler) RefreshTools() error {
	allTools := make([]llm.ToolDefinition, 0)
	for i, srv := range h.MCPSrvs {
		if err := srv.Initialize(); err != nil {
			slog.Error("handler_mcp_init_failed", "index", i, "server", srv.Name, "url", srv.URL, "error", err)
			continue
		}
		tools, err := srv.ListTools()
		if err != nil {
			slog.Error("handler_mcp_list_tools_failed", "index", i, "server", srv.Name, "url", srv.URL, "error", err)
			continue
		}
		for _, tool := range tools {
			uniqueName := fmt.Sprintf("s%d_%s", i, tool.Name)
				desc := fmt.Sprintf("[MCP端点: %s] %s", srv.Name, tool.Description)
			params := tool.InputSchema
			if params == nil {
				params = map[string]interface{}{
					"type":                 "object",
					"properties":           map[string]interface{}{},
					"additionalProperties": true,
				}
			}
			allTools = append(allTools, llm.ToolDefinition{
				Type: "function",
				Function: llm.FunctionDefinition{
					Name:        uniqueName,
					Description: desc,
					Parameters:  params,
				},
			})
		}
		slog.Info("handler_mcp_tools_loaded", "index", i, "server", srv.Name, "count", len(tools))
	}
	h.Tools = allTools
	slog.Info("handler_total_tools_loaded", "count", len(allTools))
	return nil
}

// findMCPServer 根据工具名找到对应的 MCP 客户端
// 工具名格式: s{index}_{originalName}
func (h *ChatHandler) findMCPServer(toolName string) (*mcp.Client, string, error) {
	parts := strings.SplitN(toolName, "_", 2)
	if len(parts) != 2 {
		return nil, "", fmt.Errorf("invalid tool name format: %s", toolName)
	}

	var idx int
	if _, err := fmt.Sscanf(parts[0], "s%d", &idx); err != nil {
		return nil, "", fmt.Errorf("failed to parse server index: %s", toolName)
	}

	if idx < 0 || idx >= len(h.MCPSrvs) {
		return nil, "", fmt.Errorf("server index %d out of range", idx)
	}

	return h.MCPSrvs[idx], parts[1], nil
}

// Chat 处理聊天请求（SSE 流式）
func (h *ChatHandler) Chat(c *gin.Context) {
	var req struct {
		Msg       string `json:"msg" binding:"required"`
		SessionID string `json:"session_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "缺少消息参数"})
		return
	}

	// 如果没有 session_id，生成新的
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = newSessionID()
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(500, gin.H{"error": "不支持流式响应"})
		return
	}
	w := c.Writer

	// 首先发送 session_id 给前端
	sseSession(w, flusher, sessionID)

	// 如果没有工具可用，刷新一次
	if len(h.Tools) == 0 {
		h.RefreshTools()
	}

	// 构建 system prompt
	systemPrompt := "你是一个智能助手。你必须严格遵守以下规则：\n" +
		"1. 如果用户的问题需要查询数据，且系统提供了可用的工具，请使用工具获取数据后再回答。\n" +
		"2. 如果系统没有提供任何工具，或者工具调用全部失败，你必须直接回复'未查询到相关信息。'，不得编造、猜测或依据训练数据给出任何具体数据。\n" +
		"3. 如果用户的问题不需要工具（如闲聊、常识问答），可以直接回答。\n" +
		"4. 回答必须简洁、准确。\n" +
		"5. 当你的回答有明确的 MCP 工具查询结果作为依据时，必须在回答末尾附上数据来源说明，格式如下：\n" +
		"   ---\n" +
		"   📡 数据来源：\n" +
		"   - MCP端点「{server_name}」→ 工具「{tool_name}」\n" +
		"   如果调用了多个工具，请逐一列出。如果未调用任何工具，则不需要添加此说明。"

	if len(h.Tools) == 0 {
		systemPrompt += "\n\n当前系统未加载任何 MCP 工具，无法查询数据。对于任何需要查询的问题，请回复'未查询到相关信息，请确认 MCP 服务配置是否正确。'"
	}

	// 从内存加载历史并构建 messages
	history := h.sessions.get(sessionID)
	messages := make([]llm.ChatMessage, 0, len(history)+2)
	messages = append(messages, llm.ChatMessage{Role: "system", Content: systemPrompt})
	messages = append(messages, history...)
	messages = append(messages, llm.ChatMessage{Role: "user", Content: req.Msg})
	// 保存用户消息
	h.sessions.append(sessionID, llm.ChatMessage{Role: "user", Content: req.Msg})

	maxIter := 10

	for i := 0; i < maxIter; i++ {
		// 检查客户端是否断开
		select {
		case <-c.Request.Context().Done():
			return
		default:
		}

		if i == maxIter-1 {
			sseStatus(w, flusher, "生成回答")
			resp, err := h.LLMClient.Chat(messages, nil)
			if err != nil {
				sseStatus(w, flusher, fmt.Sprintf("LLM错误: %s", err.Error()))
				sseDone(w, flusher)
				return
			}
			if len(resp.Choices) > 0 {
				content := resp.Choices[0].Message.Content
				sseContent(w, flusher, content)
				if content != "" {
					h.sessions.append(sessionID, llm.ChatMessage{Role: "assistant", Content: content})
				}
			}
			sseDone(w, flusher)
			return
		}

		// 非流式调用 LLM（带工具），判断是否需要 tool calling
		if i == 0 {
			sseStatus(w, flusher, "请求分析")
		}
		resp, err := h.LLMClient.Chat(messages, h.Tools)
		if err != nil {
			sseStatus(w, flusher, fmt.Sprintf("LLM错误: %s", err.Error()))
			sseDone(w, flusher)
			return
		}

		if len(resp.Choices) == 0 {
			sseContent(w, flusher, "抱歉，LLM 返回了空响应。")
			sseDone(w, flusher)
			return
		}

		choice := resp.Choices[0]

		// 如果 LLM 直接返回了答案（stop），直接输出完整回答
		if choice.FinishReason == "stop" {
			if choice.Message.Content != "" {
				sseStatus(w, flusher, "生成回答")
				sseContent(w, flusher, choice.Message.Content)
				sseDone(w, flusher)
				h.sessions.append(sessionID, llm.ChatMessage{Role: "assistant", Content: choice.Message.Content})
				return
			}
			// 内容为空时回退到流式输出
			sseStatus(w, flusher, "生成回答")
			h.streamAssistant(w, flusher, sessionID, messages)
			return
		}

		// 处理 tool calls
		if choice.FinishReason == "tool_calls" && len(choice.Message.ToolCalls) > 0 {
			messages = append(messages, choice.Message)

			// 通知用户即将调用哪些工具
			for _, tc := range choice.Message.ToolCalls {
				srv, originalName, err := h.findMCPServer(tc.Function.Name)
				if err != nil {
					sseStatus(w, flusher, fmt.Sprintf("⚠️ 无法找到工具: %s", tc.Function.Name))
					continue
				}
				sseStatus(w, flusher, fmt.Sprintf("mcp__%s__%s", srv.Name, originalName))
			}

			for _, tc := range choice.Message.ToolCalls {
				srv, originalName, err := h.findMCPServer(tc.Function.Name)
				if err != nil {
					slog.Error("handler_find_mcp_server_failed", "tool", tc.Function.Name, "error", err)
					messages = append(messages, llm.ChatMessage{
						Role:       "tool",
						ToolCallID: tc.ID,
						Content:    fmt.Sprintf("error: %s", err.Error()),
					})
					continue
				}

				var args map[string]interface{}
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					args = make(map[string]interface{})
				}

				slog.Info("handler_calling_mcp_tool", "tool", originalName, "server", srv.Name, "url", srv.URL)
				result, err := srv.CallTool(originalName, args)
				if err != nil {
					slog.Error("handler_mcp_tool_call_failed", "tool", originalName, "server", srv.Name, "error", err)
					sseStatus(w, flusher, fmt.Sprintf("❌ mcp__%s__%s 失败", srv.Name, originalName))
					messages = append(messages, llm.ChatMessage{
						Role:       "tool",
						ToolCallID: tc.ID,
						Content:    fmt.Sprintf("tool execution error: %s", err.Error()),
					})
					continue
				}

				slog.Info("handler_mcp_tool_result", "tool", originalName, "server", srv.Name, "result_len", len(result))
				sseStatus(w, flusher, fmt.Sprintf("✅ mcp__%s__%s 完成", srv.Name, originalName))
				messages = append(messages, llm.ChatMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    result,
				})
			}

			continue
		}

		// 其他情况
		if choice.Message.Content != "" {
			sseStatus(w, flusher, "生成回答")
			h.streamAssistant(w, flusher, sessionID, messages)
			return
		}
		sseDone(w, flusher)
		return
	}

	sseStatus(w, flusher, "处理超时")
	sseDone(w, flusher)
}

// ClearSession 清空会话历史（新对话）
func (h *ChatHandler) ClearSession(c *gin.Context) {
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "参数错误"})
		return
	}
	if req.SessionID != "" {
		h.sessions.delete(req.SessionID)
	}
	c.JSON(200, gin.H{"status": "ok"})
}

// Health 健康检查
func (h *ChatHandler) Health(c *gin.Context) {
	c.JSON(200, gin.H{
		"status":      "ok",
		"tools_count": len(h.Tools),
		"mcp_servers": len(h.MCPSrvs),
	})
}

// Refresh 刷新工具列表
func (h *ChatHandler) Refresh(c *gin.Context) {
	if err := h.RefreshTools(); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{
		"status":      "ok",
		"tools_count": len(h.Tools),
	})
}

// streamAssistant 流式输出 assistant 回答，并写入会话历史
func (h *ChatHandler) streamAssistant(w io.Writer, flusher http.Flusher, sessionID string, messages []llm.ChatMessage) {
	streamCh, errCh := h.LLMClient.ChatStream(messages, nil)
	accumulated := ""
	for {
		select {
		case chunk, ok := <-streamCh:
			if !ok {
				// streamCh 关闭，正常结束
				sseDone(w, flusher)
				if accumulated != "" {
					h.sessions.append(sessionID, llm.ChatMessage{Role: "assistant", Content: accumulated})
				}
				return
			}
			accumulated += chunk
			sseContent(w, flusher, chunk)
		case err, ok := <-errCh:
			if ok {
				slog.Warn("handler_stream_assistant_error", "error", err)
			}
			// 收到错误或 errCh 关闭，不退出，继续排空 streamCh
		}
	}
}

// sseWrite 写入 SSE 数据（保留旧名，兼容可能存在的引用）
func sseWrite(w io.Writer, flusher http.Flusher, data string) {
	if data == "[DONE]" {
		sseDone(w, flusher)
		return
	}
	sseContent(w, flusher, data)
}

// sseSession 发送会话 ID 给前端（前端用于后续请求携带 session_id）
func sseSession(w io.Writer, flusher http.Flusher, sessionID string) {
	fmt.Fprintf(w, "event: session\ndata: %s\n\n", sessionID)
	flusher.Flush()
}

// sseStatus 写入进度状态（前端用独立样式渲染，不进入正文）
func sseStatus(w io.Writer, flusher http.Flusher, msg string) {
	fmt.Fprintf(w, "event: status\ndata: %s\n\n", msg)
	flusher.Flush()
	time.Sleep(10 * time.Millisecond)
}

// sseContent 写入正文内容（markdown）。用 JSON 编码确保换行符等特殊字符正确传输
func sseContent(w io.Writer, flusher http.Flusher, data string) {
	encoded, err := json.Marshal(data)
	if err != nil {
		encoded = []byte(data)
	}
	fmt.Fprintf(w, "data: %s\n\n", encoded)
	flusher.Flush()
	time.Sleep(10 * time.Millisecond)
}

// sseDone 发送结束标记
func sseDone(w io.Writer, flusher http.Flusher) {
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}