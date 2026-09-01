package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config 应用配置
type Config struct {
	Server ServerConfig `yaml:"server"`
	MCP    MCPConfig    `yaml:"mcp"`
	LLM    LLMConfig    `yaml:"llm"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Port  int    `yaml:"port"`
	Debug bool   `yaml:"debug"`
	Title string `yaml:"title"`
}

// MCPConfig MCP 全局配置
type MCPConfig struct {
	VerifySSL   *bool  `yaml:"verify_ssl"`
	EnableProxy *bool  `yaml:"enable_proxy"`
	AuthToken   string `yaml:"auth_token"`
}

// MCPServerJSON MCP 服务器条目
type MCPServerJSON struct {
	Name    string            `json:"name"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

// mcpServersFileJSON 标准 MCP 配置文件结构
type mcpServersFileJSON struct {
	MCPServers map[string]struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	} `json:"mcpServers"`
}

// LLMConfig LLM 配置
type LLMConfig struct {
	APIURI      string `yaml:"api_uri"`
	APIKey      string `yaml:"api_key"`
	ModelName   string `yaml:"model_name"`
	MaxTokens   int    `yaml:"max_tokens"`
	EnableProxy *bool  `yaml:"enable_proxy"`
	VerifySSL   *bool  `yaml:"verify_ssl"`
}

// Load 从 YAML 文件加载配置
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// 默认值
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 19002
	}
	if cfg.Server.Title == "" {
		cfg.Server.Title = "AI 助手"
	}

	// 自动 trim 用户输入的字符串，避免多余空格/制表符
	cfg.Server.Title = strings.TrimSpace(cfg.Server.Title)
	cfg.MCP.AuthToken = strings.TrimSpace(cfg.MCP.AuthToken)
	cfg.LLM.APIURI = strings.TrimSpace(cfg.LLM.APIURI)
	cfg.LLM.APIKey = strings.TrimSpace(cfg.LLM.APIKey)
	cfg.LLM.ModelName = strings.TrimSpace(cfg.LLM.ModelName)

	return &cfg, nil
}

// LoadMCPServers 从 JSON 字节解析 MCP 服务器列表
func LoadMCPServers(data []byte) ([]MCPServerJSON, error) {
	var file mcpServersFileJSON
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("failed to parse MCP servers config: %w", err)
	}

	servers := make([]MCPServerJSON, 0, len(file.MCPServers))
	for name, srv := range file.MCPServers {
		servers = append(servers, MCPServerJSON{
			Name:    name,
			URL:     srv.URL,
			Headers: srv.Headers,
		})
	}

	return servers, nil
}