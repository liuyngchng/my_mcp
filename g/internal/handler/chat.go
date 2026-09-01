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
		LLMClient: llm.New(cfg.LLM.APIURI, cfg.LLM.APIKey, cfg.LLM.ModelName, llmEnableProxy, llmVerifySSL),
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
					Description: tool.Description,
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

	// 如果没有工具可用，刷新一次
	if len(h.Tools) == 0 {
		h.RefreshTools()
	}

	messages := []llm.ChatMessage{
		{Role: "system", Content: "你是一个智能助手，可以根据用户需要选择合适的工具来完成任务。如果用户的问题需要查询数据，请使用可用的工具；如果不需要工具，直接回答即可。"},
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
			// 最后一次迭代，去掉工具，强制 LLM 直接回答
			resp, err := h.LLMClient.Chat(messages, nil)
			if err != nil {
				sseWrite(c.Writer, flusher, fmt.Sprintf("抱歉，请求 LLM 出错: %s", err.Error()))
				sseWrite(c.Writer, flusher, "[DONE]")
				return
			}
			if len(resp.Choices) > 0 {
				sseWrite(c.Writer, flusher, resp.Choices[0].Message.Content)
			}
			sseWrite(c.Writer, flusher, "[DONE]")
			return
		}

		// 非流式调用 LLM（带工具），判断是否需要 tool calling
		resp, err := h.LLMClient.Chat(messages, h.Tools)
		if err != nil {
			sseWrite(c.Writer, flusher, fmt.Sprintf("抱歉，请求 LLM 出错: %s", err.Error()))
			sseWrite(c.Writer, flusher, "[DONE]")
			return
		}

		if len(resp.Choices) == 0 {
			sseWrite(c.Writer, flusher, "抱歉，LLM 返回了空响应。")
			sseWrite(c.Writer, flusher, "[DONE]")
			return
		}

		choice := resp.Choices[0]

		// 如果 LLM 直接返回了答案（stop），则流式输出
		if choice.FinishReason == "stop" {
			// 用流式重新获取最终答案
			streamCh, errCh := h.LLMClient.ChatStream(messages, nil)
			for {
				select {
				case chunk, ok := <-streamCh:
					if !ok {
						sseWrite(c.Writer, flusher, "[DONE]")
						return
					}
					sseWrite(c.Writer, flusher, chunk)
				case err := <-errCh:
					if err != nil {
						slog.Warn("stream final answer error", "error", err)
						// 用非流式结果兜底
						sseWrite(c.Writer, flusher, choice.Message.Content)
					}
					sseWrite(c.Writer, flusher, "[DONE]")
					return
				}
			}
		}

		// 处理 tool calls
		if choice.FinishReason == "tool_calls" && len(choice.Message.ToolCalls) > 0 {
			// 添加 assistant 消息
			messages = append(messages, choice.Message)

			// 发送进度通知
			toolNames := make([]string, len(choice.Message.ToolCalls))
			for j, tc := range choice.Message.ToolCalls {
				toolNames[j] = tc.Function.Name
			}
			sseWrite(c.Writer, flusher, fmt.Sprintf("\n\n🔧 *正在调用工具: %s...*\n\n", strings.Join(toolNames, ", ")))

			// 执行每个工具调用
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

				// 解析参数
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					args = make(map[string]interface{})
				}

				slog.Info("calling mcp tool", "tool", originalName, "server", srv.Name, "url", srv.URL)
				result, err := srv.CallTool(originalName, args)
				if err != nil {
					slog.Error("mcp tool call failed", "tool", originalName, "server", srv.Name, "error", err)
					messages = append(messages, llm.ChatMessage{
						Role:       "tool",
						ToolCallID: tc.ID,
						Content:    fmt.Sprintf("tool execution error: %s", err.Error()),
					})
					continue
				}

				slog.Info("mcp tool result", "tool", originalName, "server", srv.Name, "result_len", len(result))
				messages = append(messages, llm.ChatMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    result,
				})
			}

			// 继续循环，让 LLM 处理工具结果
			continue
		}

		// 其他情况（可能是 length 等），用流式输出
		if choice.Message.Content != "" {
			streamCh, errCh := h.LLMClient.ChatStream(messages, nil)
			for {
				select {
				case chunk, ok := <-streamCh:
					if !ok {
						sseWrite(c.Writer, flusher, "[DONE]")
						return
					}
					sseWrite(c.Writer, flusher, chunk)
				case err := <-errCh:
					if err != nil {
						sseWrite(c.Writer, flusher, choice.Message.Content)
					}
					sseWrite(c.Writer, flusher, "[DONE]")
					return
				}
			}
		}
		sseWrite(c.Writer, flusher, "[DONE]")
		return
	}

	sseWrite(c.Writer, flusher, "处理超时，未能生成完整回答。")
	sseWrite(c.Writer, flusher, "[DONE]")
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

// sseWrite 写入 SSE 数据
func sseWrite(w io.Writer, flusher http.Flusher, data string) {
	if data == "[DONE]" {
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
		return
	}
	// 按换行分割，逐行发送 SSE
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

	// 对抗浏览器缓冲：在流式场景下，每输出一小段就 sleep 极短时间，让浏览器有时间渲染
	time.Sleep(10 * time.Millisecond)
}