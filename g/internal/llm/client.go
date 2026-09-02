package llm

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"mcp-chat/internal/proxy"
)

// —— OpenAI 兼容类型 ——

// ChatMessage 聊天消息
type ChatMessage struct {
	Role       string       `json:"role"`
	Content    string       `json:"content,omitempty"`
	ToolCalls  []ToolCall   `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
}

// ToolCall 工具调用
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall 函数调用
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolDefinition 工具定义（OpenAI function calling 格式）
type ToolDefinition struct {
	Type     string              `json:"type"`
	Function FunctionDefinition  `json:"function"`
}

// FunctionDefinition 函数定义
type FunctionDefinition struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

// ChatCompletionRequest 非流式请求
type ChatCompletionRequest struct {
	Model     string           `json:"model"`
	Messages  []ChatMessage    `json:"messages"`
	Tools     []ToolDefinition `json:"tools,omitempty"`
	Stream    bool             `json:"stream"`
	MaxTokens int              `json:"max_tokens,omitempty"`
}

// ChatCompletionResponse 非流式响应
type ChatCompletionResponse struct {
	Choices []struct {
		FinishReason string      `json:"finish_reason"`
		Message      ChatMessage `json:"message"`
	} `json:"choices"`
}

// ChatCompletionChunk 流式响应块
type ChatCompletionChunk struct {
	Choices []struct {
		Delta struct {
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// Client LLM 客户端
type Client struct {
	BaseURL     string
	APIKey      string
	ModelName   string
	MaxTokens   int
	enableProxy bool
	httpCli     *http.Client
}

// New 创建 LLM 客户端
func New(baseURL, apiKey, modelName string, maxTokens int, enableProxy bool, verifySSL bool) *Client {
	tr := proxy.Transport(enableProxy)
	if !verifySSL {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	proxyURL, proxyEnabled := proxy.Resolve(enableProxy)
	if proxyEnabled {
		slog.Info("llm client using proxy", "proxy", proxyURL.String())
	} else if enableProxy {
		slog.Info("llm client proxy enabled but no proxy env var, connecting directly")
	} else {
		slog.Info("llm client proxy disabled, connecting directly")
	}

	return &Client{
		BaseURL:     strings.TrimRight(baseURL, "/"),
		APIKey:      apiKey,
		ModelName:   modelName,
		MaxTokens:   maxTokens,
		enableProxy: enableProxy,
		httpCli: &http.Client{
			Transport: tr,
			Timeout:   120 * time.Second,
		},
	}
}

// ChatStream 流式聊天，返回 channel 逐块输出文本
func (c *Client) ChatStream(messages []ChatMessage, tools []ToolDefinition) (<-chan string, <-chan error) {
	chunkCh := make(chan string, 100)
	errCh := make(chan error, 1)

	go func() {
		defer close(chunkCh)
		defer close(errCh)

		reqBody := ChatCompletionRequest{
			Model:     c.ModelName,
			Messages:  messages,
			Tools:     tools,
			Stream:    true,
			MaxTokens: c.MaxTokens,
		}

		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			errCh <- fmt.Errorf("failed to marshal request: %w", err)
			return
		}

		url := c.BaseURL + "/chat/completions"
		req, err := http.NewRequest("POST", url, bytes.NewReader(jsonData))
		if err != nil {
			errCh <- fmt.Errorf("failed to create http request: %w", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.APIKey)

		start := time.Now()
		slog.Info("llm stream request start", "model", c.ModelName, "url", url)
		resp, err := c.httpCli.Do(req)
		if err != nil {
			slog.Error("llm stream request failed", "model", c.ModelName, "error", err, "duration_ms", time.Since(start).Milliseconds())
			errCh <- fmt.Errorf("llm request failed: %w", err)
			return
		}
		defer resp.Body.Close()

		slog.Info("llm stream response received", "model", c.ModelName, "status", resp.StatusCode, "duration_ms", time.Since(start).Milliseconds())

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			bodyStr := string(body)
			if len([]rune(bodyStr)) > 300 {
				runes := []rune(bodyStr)
				bodyStr = string(runes[:300]) + "..."
			}
			slog.Error("llm stream response error", "status", resp.StatusCode, "body", bodyStr)
			errCh <- fmt.Errorf("llm api returned %d: %s", resp.StatusCode, bodyStr)
			return
		}

		reader := bufio.NewReaderSize(resp.Body, 64*1024)
		chunkCount := 0
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					break
				}
				slog.Warn("llm stream read error", "error", err)
				break
			}

			line = strings.TrimSpace(line)
			if line == "" || !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}

			var chunk ChatCompletionChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				chunkCount++
				chunkCh <- chunk.Choices[0].Delta.Content
			}
		}

		slog.Info("llm stream done", "model", c.ModelName, "chunks", chunkCount, "duration_ms", time.Since(start).Milliseconds())
	}()

	return chunkCh, errCh
}

// Chat 非流式聊天（用于 tool calling 循环）
func (c *Client) Chat(messages []ChatMessage, tools []ToolDefinition) (*ChatCompletionResponse, error) {
	reqBody := ChatCompletionRequest{
		Model:     c.ModelName,
		Messages:  messages,
		Tools:     tools,
		Stream:    false,
		MaxTokens: c.MaxTokens,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.BaseURL + "/chat/completions"
	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	start := time.Now()
	slog.Info("llm sync request start", "model", c.ModelName, "url", url)
	resp, err := c.httpCli.Do(req)
	if err != nil {
		slog.Error("llm sync request failed", "model", c.ModelName, "error", err, "duration_ms", time.Since(start).Milliseconds())
		return nil, fmt.Errorf("llm request failed: %w", err)
	}
	defer resp.Body.Close()

	slog.Info("llm sync response received", "model", c.ModelName, "status", resp.StatusCode, "duration_ms", time.Since(start).Milliseconds())

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		bodyStr := string(body)
		if len([]rune(bodyStr)) > 300 {
			runes := []rune(bodyStr)
			bodyStr = string(runes[:300]) + "..."
		}
		slog.Error("llm sync response error", "status", resp.StatusCode, "body", bodyStr)
		return nil, fmt.Errorf("llm api returned %d: %s", resp.StatusCode, bodyStr)
	}

	var result ChatCompletionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	slog.Info("llm sync request done", "model", c.ModelName, "duration_ms", time.Since(start).Milliseconds())
	return &result, nil
}