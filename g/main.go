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

//go:embed mcp_servers.json
var mcpServersJSON []byte

func main() {
	// 配置路径
	cfgPath := "cfg.yml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 初始化日志
	if err := logger.Init(cfg.Server.Debug); err != nil {
		fmt.Fprintf(os.Stderr, "failed to init logger: %v\n", err)
		os.Exit(1)
	}

	slog.Info("starting mcp-chat", "port", cfg.Server.Port)

	// 创建聊天处理器
	chatHandler := handler.NewChatHandler(cfg, mcpServersJSON)
	pageHandler := handler.NewPageHandler("MCP Smart Assistant")

	// 设置 Gin
	if !cfg.Server.Debug {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(logger.GinLogger(), gin.Recovery())

	// 加载 HTML 模板
	tmpl, err := template.ParseFS(webFS, "web/templates/*")
	if err != nil {
		slog.Error("failed to load templates", "error", err)
		os.Exit(1)
	}
	r.SetHTMLTemplate(tmpl)

	// 静态文件
	staticFS, err := fs.Sub(webFS, "web/static")
	if err != nil {
		slog.Error("failed to load static files", "error", err)
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
		slog.Info("shutting down...")
		os.Exit(0)
	}()

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	slog.Info("server started", "addr", addr, "url", fmt.Sprintf("http://localhost%s", addr))
	if err := r.Run(addr); err != nil {
		slog.Error("server failed to start", "error", err)
		os.Exit(1)
	}
}