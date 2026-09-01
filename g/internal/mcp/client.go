package mcp

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"mcp-chat/internal/config"
	"mcp-chat/internal/proxy"
)

// JSON-RPC 消息结构
type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Tool MCP 工具信息
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

// Client MCP Streamable HTTP 客户端
type Client struct {
	URL         string
	Name        string
	httpCli     *http.Client
	headers     map[string]string
	enableProxy bool
	sessionID   string
	reqID       int
}

// NewClient 创建 MCP 客户端
func NewClient(name, url string, headers map[string]string, verifySSL bool, enableProxy bool) *Client {
	tr := proxy.Transport(enableProxy)
	if !verifySSL {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	proxyURL, proxyEnabled := proxy.Resolve(enableProxy)
	if proxyEnabled {
		slog.Info("mcp client using proxy", "name", name, "url", url, "proxy", proxyURL.String())
	} else if enableProxy {
		slog.Info("mcp client proxy enabled but no proxy env var, connecting directly", "name", name, "url", url)
	} else {
		slog.Info("mcp client proxy disabled, connecting directly", "name", name, "url", url)
	}

	return &Client{
		URL:         url,
		Name:        name,
		headers:     headers,
		enableProxy: enableProxy,
		httpCli: &http.Client{
			Transport: tr,
			Timeout:   60 * time.Second,
		},
	}
}

// NewClientFromConfig 从配置创建多个 MCP 客户端
// serversJSON 为 embed 的 mcp_servers.json 字节内容
func NewClientFromConfig(cfg *config.Config, serversJSON []byte) ([]*Client, error) {
	servers, err := config.LoadMCPServers(serversJSON)
	if err != nil {
		return nil, err
	}

	verifySSL := true
	if cfg.MCP.VerifySSL != nil {
		verifySSL = *cfg.MCP.VerifySSL
	}
	enableProxy := true
	if cfg.MCP.EnableProxy != nil {
		enableProxy = *cfg.MCP.EnableProxy
	}

	clients := make([]*Client, 0, len(servers))
	for _, srv := range servers {
		headers := srv.Headers
		// 从 cfg.yml 读取全局认证 token，自动加 Bearer 前缀
		if cfg.MCP.AuthToken != "" {
			if headers == nil {
				headers = make(map[string]string)
			}
			if _, exists := headers["Authorization"]; !exists {
				token := strings.TrimSpace(cfg.MCP.AuthToken)
				if !strings.HasPrefix(token, "Bearer ") {
					token = "Bearer " + token
				}
				headers["Authorization"] = token
			}
		}
		clients = append(clients, NewClient(srv.Name, srv.URL, headers, verifySSL, enableProxy))
	}

	slog.Info("mcp servers loaded from embedded json", "count", len(clients))
	return clients, nil
}

// call 发送 JSON-RPC 请求
func (c *Client) call(method string, params interface{}) (*jsonRPCResponse, error) {
	c.reqID++
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      c.reqID,
		Method:  method,
		Params:  params,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.URL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	if c.sessionID != "" {
		httpReq.Header.Set("mcp-session-id", c.sessionID)
	}
	for k, v := range c.headers {
		httpReq.Header.Set(k, v)
	}

	// 请求前日志
	slog.Info("mcp request start", "server", c.Name, "url", c.URL, "method", method, "req_id", c.reqID)
	start := time.Now()
	resp, err := c.httpCli.Do(httpReq)
	if err != nil {
		slog.Error("mcp request failed", "server", c.Name, "url", c.URL, "method", method, "error", err, "duration_ms", time.Since(start).Milliseconds())
		return nil, fmt.Errorf("mcp request failed: %w", err)
	}
	defer resp.Body.Close()

	// 保存 session ID（MCP Streamable HTTP 协议要求）
	if sid := resp.Header.Get("mcp-session-id"); sid != "" {
		c.sessionID = sid
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("mcp read response failed", "server", c.Name, "method", method, "error", err)
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// 请求后日志
	slog.Info("mcp request done", "server", c.Name, "url", c.URL, "method", method, "status", resp.StatusCode, "duration_ms", time.Since(start).Milliseconds())

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mcp returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	payload := extractJSON(respBody)

	var rpcResp jsonRPCResponse
	if err := json.Unmarshal(payload, &rpcResp); err != nil {
		// 打印响应体前 500 字符，方便排查格式问题
		preview := string(respBody)
		if len([]rune(preview)) > 500 {
			runes := []rune(preview)
			preview = string(runes[:500]) + "..."
		}
		slog.Error("mcp response parse failed", "server", c.Name, "method", method, "body_preview", preview)
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("mcp error [%d]: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return &rpcResp, nil
}

// extractJSON 从响应体中提取 JSON。
// MCP Streamable HTTP 服务器可能返回纯 JSON，也可能返回 SSE 格式
// （形如 "event: message\ndata: {...}"）。这里做兼容处理：
//   - 若响应体本身是合法 JSON，直接使用；
//   - 否则扫描所有 "data:" 行，取第一个以 "{" 开头的内容作为 JSON 负载。
func extractJSON(body []byte) []byte {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		return trimmed
	}

	for _, line := range bytes.Split(trimmed, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		data := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(data) > 0 && (data[0] == '{' || data[0] == '[') {
			return data
		}
	}

	return trimmed
}

// Initialize 初始化 MCP 会话
func (c *Client) Initialize() error {
	params := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]string{
			"name":    "mcp-chat",
			"version": "1.0.0",
		},
	}
	_, err := c.call("initialize", params)
	return err
}

// ListTools 获取工具列表
func (c *Client) ListTools() ([]Tool, error) {
	slog.Info("mcp list tools start", "server", c.Name, "url", c.URL)
	resp, err := c.call("tools/list", nil)
	if err != nil {
		slog.Error("mcp list tools failed", "server", c.Name, "error", err)
		return nil, err
	}

	var result struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		slog.Error("mcp parse tools failed", "server", c.Name, "error", err)
		return nil, fmt.Errorf("failed to parse tools list: %w", err)
	}

	slog.Info("mcp list tools done", "server", c.Name, "count", len(result.Tools))
	return result.Tools, nil
}

// CallTool 调用 MCP 工具
func (c *Client) CallTool(name string, args map[string]interface{}) (string, error) {
	params := map[string]interface{}{
		"name":      name,
		"arguments": args,
	}

	slog.Info("mcp call tool start", "server", c.Name, "tool", name)
	resp, err := c.call("tools/call", params)
	if err != nil {
		slog.Error("mcp call tool failed", "server", c.Name, "tool", name, "error", err)
		return "", err
	}

	// 解析结果
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		// 尝试作为纯文本解析
		var rawResult map[string]interface{}
		json.Unmarshal(resp.Result, &rawResult)
		return fmt.Sprintf("%v", rawResult), nil
	}

	if result.IsError {
		var texts []string
		for _, c := range result.Content {
			texts = append(texts, c.Text)
		}
		return "", fmt.Errorf("tool execution error: %v", texts)
	}

	var texts []string
	for _, c := range result.Content {
		if c.Type == "text" {
			texts = append(texts, c.Text)
		}
	}
	if len(texts) == 0 {
		return string(resp.Result), nil
	}

	// 合并多个文本块
	joined := ""
	for i, t := range texts {
		if i > 0 {
			joined += "\n"
		}
		joined += t
	}

	slog.Info("mcp call tool done", "server", c.Name, "tool", name, "result_len", len(joined))
	return joined, nil
}