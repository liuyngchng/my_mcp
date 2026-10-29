package main

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net"
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
	// 初始化日志（先于配置加载，确保启动错误可记录）
	if err := logger.Init(true); err != nil {
		fmt.Fprintf(os.Stderr, "failed to init logger: %v\n", err)
		os.Exit(1)
	}

	// 配置路径
	cfgPath := "cfg.yml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		slog.Error("main_config_load_failed", "error", err, "path", cfgPath)
		os.Exit(1)
	}

	// 按配置重新设置日志级别
	if err := logger.Init(cfg.Server.Debug); err != nil {
		slog.Error("main_logger_reinit_failed", "error", err)
		os.Exit(1)
	}

	slog.Info("main_starting_mcp_chat", "port", cfg.Server.Port)

	// 获取本机局域网 IP
	localIP := getLocalIP()
	slog.Info("main_access_url", "url", fmt.Sprintf("http://%s:%d", localIP, cfg.Server.Port))

	// 创建聊天处理器
	chatHandler := handler.NewChatHandler(cfg, mcpServersJSON)
	pageHandler := handler.NewPageHandler(cfg.Server.Title)

	// 设置 Gin
	if !cfg.Server.Debug {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(logger.GinLogger(), gin.Recovery())

	// 加载 HTML 模板
	tmpl, err := template.ParseFS(webFS, "web/templates/*")
	if err != nil {
		slog.Error("main_template_load_failed", "error", err)
		os.Exit(1)
	}
	r.SetHTMLTemplate(tmpl)

	// 静态文件
	staticFS, err := fs.Sub(webFS, "web/static")
	if err != nil {
		slog.Error("main_static_files_load_failed", "error", err)
		os.Exit(1)
	}
	r.StaticFS("/static", http.FS(staticFS))

	// 路由
	r.GET("/", pageHandler.Index)
	r.GET("/health", chatHandler.Health)
	r.POST("/api/chat", chatHandler.Chat)
	r.POST("/api/chat/clear", chatHandler.ClearSession)

	// 优雅退出
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("main_shutting_down")
		os.Exit(0)
	}()

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	slog.Info("main_server_started", "addr", addr, "url", fmt.Sprintf("http://localhost%s", addr))
	if err := r.Run(addr); err != nil {
		slog.Error("main_server_start_failed", "error", err)
		os.Exit(1)
	}
}

// getLocalIP 获取本机首选局域网 IPv4 地址，获取失败时返回 localhost
func getLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "localhost"
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}