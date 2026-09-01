package main

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"mcp-chat/internal/config"
	"mcp-chat/internal/handler"
	"mcp-chat/internal/logger"

	"github.com/gin-gonic/gin"
)

//go:embed web
var webFS embed.FS

func main() {
	// 配置路径
	cfgPath := "cfg.yml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置文件失败: %v\n", err)
		os.Exit(1)
	}

	// 初始化日志（自定义格式 + 按小时滚动文件）
	if err := logger.Init(cfg.Server.Debug); err != nil {
		fmt.Fprintf(os.Stderr, "初始化日志失败: %v\n", err)
		os.Exit(1)
	}

	slog.Info("启动 MCP 自然语言查询客户端", "port", cfg.Server.Port, "mcp_servers_file", cfg.MCP.ServersFile)

	// 创建聊天处理器
	chatHandler := handler.NewChatHandler(cfg)
	pageHandler := handler.NewPageHandler("MCP 智能助手")

	// 设置 Gin
	if !cfg.Server.Debug {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(logger.GinLogger(), gin.Recovery())

	// 加载 HTML 模板（从 embed.FS）
	tmpl, err := template.ParseFS(webFS, "web/templates/*")
	if err != nil {
		slog.Error("加载模板失败", "error", err)
		os.Exit(1)
	}
	r.SetHTMLTemplate(tmpl)

	// 静态文件（从 embed.FS）
	staticFS, err := fs.Sub(webFS, "web/static")
	if err != nil {
		slog.Error("加载静态文件失败", "error", err)
		os.Exit(1)
	}
	r.StaticFS("/static", http.FS(staticFS))

	// 路由
	r.GET("/", pageHandler.Index)
	r.GET("/health", chatHandler.Health)
	r.POST("/api/chat", chatHandler.Chat)
	r.POST("/api/refresh", chatHandler.Refresh)

	// 优雅退出
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("正在关闭服务...")
		os.Exit(0)
	}()

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	slog.Info("服务启动", "addr", addr, "url", fmt.Sprintf("http://localhost%s", addr))
	if err := r.Run(addr); err != nil {
		slog.Error("服务启动失败", "error", err)
		os.Exit(1)
	}
}