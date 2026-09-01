package mcp

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"mcp-chat/internal/config"
	"mcp-chat/internal/proxy"
)

// Client MCP Streamable HTTP 客户端
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
func NewClientFromConfig(cfg *config.Config) ([]*Client, error) {
	servers, err := config.LoadMCPServers(cfg.MCP.ServersFile)
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
		// 从 cfg.yml 读取全局认证 token，代码自动加 Bearer 前缀
		if cfg.MCP.AuthToken != "" {
			if headers == nil {
				headers = make(map[string]string)
			}
			if _, exists := headers["Authorization"]; !exists {
				headers["Authorization"] = "Bearer " + cfg.MCP.AuthToken
			}
		}
		clients = append(clients, NewClient(srv.Name, srv.URL, headers, verifySSL, enableProxy))
	}

	slog.Info("mcp servers loaded from json", "file", cfg.MCP.ServersFile, "count", len(clients))
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
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.URL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	if c.sessionID != "" {
		httpReq.Header.Set("mcp-session-id", c.sessionID)
	}
	for k, v := range c.headers {
		httpReq.Header.Set(k, v)
	}

	slog.Info("mcp request", "url", c.URL, "method", method)
	start := time.Now()
	resp, err := c.httpCli.Do(httpReq)
	if err != nil {
		slog.Error("mcp request error", "url", c.URL, "method", method, "error", err)
		return nil, fmt.Errorf("MCP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 保存 session ID（MCP Streamable HTTP 协议要求）
	if sid := resp.Header.Get("mcp-session-id"); sid != "" {
		c.sessionID = sid
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	slog.Info("mcp response", "url", c.URL, "method", method, "status", resp.StatusCode, "duration_ms", time.Since(start).Milliseconds())

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MCP 返回 HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var rpcResp jsonRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("MCP 错误 [%d]: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return &rpcResp, nil
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
	resp, err := c.call("tools/list", nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("解析工具列表失败: %w", err)
	}

	return result.Tools, nil
}

// CallTool 调用 MCP 工具
func (c *Client) CallTool(name string, args map[string]interface{}) (string, error) {
	params := map[string]interface{}{
		"name":      name,
		"arguments": args,
	}

	resp, err := c.call("tools/call", params)
	if err != nil {
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
		return "", fmt.Errorf("工具执行出错: %v", texts)
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
	return joined, nil
}