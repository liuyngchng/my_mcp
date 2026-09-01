package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"mcp-chat/internal/config"
	"mcp-chat/internal/llm"
	"mcp-chat/internal/mcp"

	"github.com/gin-gonic/gin"
)

// ChatHandler 聊天处理器
type ChatHandler struct {
	Cfg       *config.Config
	MCPSrvs   []*mcp.Client
	LLMClient *llm.Client
	Tools     []llm.ToolDefinition
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
		slog.Error("failed to load mcp servers", "error", err)
		mcpSrvs = []*mcp.Client{}
	}

	h := &ChatHandler{
		Cfg:       cfg,
		MCPSrvs:   mcpSrvs,
		LLMClient: llm.New(cfg.LLM.APIURI, cfg.LLM.APIKey, cfg.LLM.ModelName, cfg.LLM.MaxTokens, llmEnableProxy, llmVerifySSL),
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
			slog.Error("mcp init failed", "index", i, "server", srv.Name, "url", srv.URL, "error", err)
			continue
		}
		tools, err := srv.ListTools()
		if err != nil {
			slog.Error("mcp list tools failed", "index", i, "server", srv.Name, "url", srv.URL, "error", err)
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
		slog.Info("mcp tools loaded", "index", i, "server", srv.Name, "count", len(tools))
	}
	h.Tools = allTools
	slog.Info("total tools loaded", "count", len(allTools))
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
		Msg string `json:"msg" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "缺少消息参数"})
		return
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

	// 如果没有工具可用，刷新一次
	if len(h.Tools) == 0 {
		h.RefreshTools()
	}

	// 构建 system prompt：严格禁止 LLM 编造数据
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

	messages := []llm.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: req.Msg},
	}

	maxIter := 10
	for i := 0; i < maxIter; i++ {
		// 检查客户端是否断开
		select {
		case <-c.Request.Context().Done():
			return
		default:
		}

		if i == maxIter-1 {
			sseStatus(w, flusher, "正在生成最终回答...")
			resp, err := h.LLMClient.Chat(messages, nil)
			if err != nil {
				sseStatus(w, flusher, fmt.Sprintf("请求 LLM 出错: %s", err.Error()))
				sseDone(w, flusher)
				return
			}
			if len(resp.Choices) > 0 {
				sseContent(w, flusher, resp.Choices[0].Message.Content)
			}
			sseDone(w, flusher)
			return
		}

		// 非流式调用 LLM（带工具），判断是否需要 tool calling
		if i == 0 {
			sseStatus(w, flusher, "正在分析您的问题...")
		}
		resp, err := h.LLMClient.Chat(messages, h.Tools)
		if err != nil {
			sseStatus(w, flusher, fmt.Sprintf("请求 LLM 出错: %s", err.Error()))
			sseDone(w, flusher)
			return
		}

		if len(resp.Choices) == 0 {
			sseContent(w, flusher, "抱歉，LLM 返回了空响应。")
			sseDone(w, flusher)
			return
		}

		choice := resp.Choices[0]

		// 如果 LLM 直接返回了答案（stop），则流式输出
		if choice.FinishReason == "stop" {
			sseStatus(w, flusher, "正在生成回答...")
			streamCh, errCh := h.LLMClient.ChatStream(messages, nil)
			for {
				select {
				case chunk, ok := <-streamCh:
					if !ok {
						sseDone(w, flusher)
						return
					}
					sseContent(w, flusher, chunk)
				case err := <-errCh:
					if err != nil {
						slog.Warn("stream final answer error", "error", err)
						sseContent(w, flusher, choice.Message.Content)
					}
					sseDone(w, flusher)
					return
				}
			}
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
				sseStatus(w, flusher, fmt.Sprintf("📡 正在查询 %s → %s ...", srv.Name, originalName))
			}

			for _, tc := range choice.Message.ToolCalls {
				srv, originalName, err := h.findMCPServer(tc.Function.Name)
				if err != nil {
					slog.Error("find mcp server failed", "tool", tc.Function.Name, "error", err)
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

				slog.Info("calling mcp tool", "tool", originalName, "server", srv.Name, "url", srv.URL)
				result, err := srv.CallTool(originalName, args)
				if err != nil {
					slog.Error("mcp tool call failed", "tool", originalName, "server", srv.Name, "error", err)
					sseStatus(w, flusher, fmt.Sprintf("❌ %s → %s 查询失败", srv.Name, originalName))
					messages = append(messages, llm.ChatMessage{
						Role:       "tool",
						ToolCallID: tc.ID,
						Content:    fmt.Sprintf("tool execution error: %s", err.Error()),
					})
					continue
				}

				slog.Info("mcp tool result", "tool", originalName, "server", srv.Name, "result_len", len(result))
				sseStatus(w, flusher, fmt.Sprintf("✅ %s 查询完成，正在分析结果...", srv.Name))
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
			sseStatus(w, flusher, "正在生成回答...")
			streamCh, errCh := h.LLMClient.ChatStream(messages, nil)
			for {
				select {
				case chunk, ok := <-streamCh:
					if !ok {
						sseDone(w, flusher)
						return
					}
					sseContent(w, flusher, chunk)
				case err := <-errCh:
					if err != nil {
						sseContent(w, flusher, choice.Message.Content)
					}
					sseDone(w, flusher)
					return
				}
			}
		}
		sseDone(w, flusher)
		return
	}

	sseStatus(w, flusher, "处理超时，未能生成完整回答。")
	sseDone(w, flusher)
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

// sseWrite 写入 SSE 数据（保留旧名，兼容可能存在的引用）
func sseWrite(w io.Writer, flusher http.Flusher, data string) {
	if data == "[DONE]" {
		sseDone(w, flusher)
		return
	}
	sseContent(w, flusher, data)
}

// sseStatus 写入进度状态（前端用独立样式渲染，不进入正文）
func sseStatus(w io.Writer, flusher http.Flusher, msg string) {
	fmt.Fprintf(w, "event: status\ndata: %s\n\n", msg)
	flusher.Flush()
	time.Sleep(10 * time.Millisecond)
}

// sseContent 写入正文内容（markdown）
func sseContent(w io.Writer, flusher http.Flusher, data string) {
	if strings.Contains(data, "\n") {
		lines := strings.Split(data, "\n")
		for _, line := range lines {
			if line == "" {
				fmt.Fprintf(w, "data: \n")
			} else {
				fmt.Fprintf(w, "data: %s\n", line)
			}
		}
	} else {
		fmt.Fprintf(w, "data: %s\n", data)
	}
	fmt.Fprintf(w, "\n")
	flusher.Flush()
	time.Sleep(10 * time.Millisecond)
}

// sseDone 发送结束标记
func sseDone(w io.Writer, flusher http.Flusher) {
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}