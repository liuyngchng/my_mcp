package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// SourceWidth 源码位置显示宽度
const SourceWidth = 24

// modulePrefix 模块名前缀
const modulePrefix = "mcp-chat/"

// customHandler 自定义日志格式
// 格式: 2026-09-01 21:00:00.000 INFO [file.go:123] 消息 key=value
type customHandler struct {
	w      io.Writer
	mu     sync.Mutex
	level  slog.Leveler
	attrs  []slog.Attr
	groups []string
}

func (h *customHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *customHandler) Handle(_ context.Context, r slog.Record) error {
	buf := make([]byte, 0, 256)

	// 1. 时间
	buf = append(buf, r.Time.Format("2006-01-02 15:04:05.000")...)

	// 2. 级别（补齐到 5 字符）
	buf = append(buf, ' ')
	buf = append(buf, padLevel(r.Level.String())...)

	// 3. 源码位置
	if r.PC != 0 {
		frame, _ := runtime.CallersFrames([]uintptr{r.PC}).Next()
		loc := sourceLocation(frame)
		buf = append(buf, ' ')
		buf = append(buf, '[')
		if len(loc) < SourceWidth {
			buf = append(buf, strings.Repeat(" ", SourceWidth-len(loc))...)
		}
		buf = append(buf, loc...)
		buf = append(buf, ']')
	}

	// 4. 消息
	buf = append(buf, ' ')
	buf = append(buf, r.Message...)

	// 5. 属性
	r.Attrs(func(a slog.Attr) bool {
		buf = append(buf, ' ')
		buf = append(buf, a.Key...)
		buf = append(buf, '=')
		buf = append(buf, a.Value.String()...)
		return true
	})

	buf = append(buf, '\n')

	h.mu.Lock()
	_, err := h.w.Write(buf)
	h.mu.Unlock()
	return err
}

func (h *customHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &customHandler{w: h.w, level: h.level, attrs: append(h.attrs, attrs...)}
}

func (h *customHandler) WithGroup(name string) slog.Handler {
	return &customHandler{w: h.w, level: h.level, attrs: h.attrs, groups: append(h.groups, name)}
}

func sourceLocation(frame runtime.Frame) string {
	pkg := strings.TrimPrefix(frame.Function, modulePrefix)
	if i := strings.Index(pkg, "("); i > 0 {
		pkg = pkg[:i]
	} else if i := strings.LastIndex(pkg, "."); i > 0 {
		pkg = pkg[:i]
	}
	pkg = strings.TrimSuffix(pkg, ".")
	return abbreviatePath(pkg) + "/" + filepath.Base(frame.File) + ":" + strconv.Itoa(frame.Line)
}

func abbreviatePath(pkg string) string {
	segs := strings.Split(pkg, "/")
	for i, seg := range segs {
		if seg == "" {
			continue
		}
		for _, r := range seg {
			segs[i] = string(r)
			break
		}
	}
	return strings.Join(segs, "/")
}

func padLevel(level string) string {
	if len(level) < 5 {
		return level + strings.Repeat(" ", 5-len(level))
	}
	return level
}

// hourlyWriter 按小时滚动，同时输出到控制台
type hourlyWriter struct {
	dir        string
	mu         sync.Mutex
	file       *os.File
	currentHour string
}

func (w *hourlyWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	hour := time.Now().Format("2006-01-02-15")
	if w.file == nil || hour != w.currentHour {
		if w.file != nil {
			w.file.Close()
		}
		os.MkdirAll(w.dir, 0755)
		fname := filepath.Join(w.dir, "app."+hour+".log")
		f, err := os.OpenFile(fname, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			// 回退到 stdout
			os.Stdout.Write(p)
			return len(p), nil
		}
		w.file = f
		w.currentHour = hour

		// 更新软链接
		link := filepath.Join(w.dir, "app.log")
		os.Remove(link)
		os.Symlink(fname, link)
	}

	// 同时写文件和控制台
	os.Stdout.Write(p)
	return w.file.Write(p)
}

func (w *hourlyWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

// Init 初始化日志
func Init(debug bool) error {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	hw := &hourlyWriter{dir: "log"}
	handler := &customHandler{w: hw, level: level}
	logger := slog.New(handler)
	slog.SetDefault(logger)

	return nil
}

// GinLogger Gin 请求日志中间件（只记录 4xx/5xx）
func GinLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		statusCode := c.Writer.Status()
		if statusCode < 400 {
			return
		}

		latency := time.Since(start)
		method := c.Request.Method
		clientIP := c.ClientIP()

		level := slog.LevelWarn
		if statusCode >= 500 {
			level = slog.LevelError
		}

		slog.LogAttrs(c.Request.Context(), level,
			"GIN",
			slog.Int("status", statusCode),
			slog.String("method", method),
			slog.String("path", path),
			slog.String("query", query),
			slog.String("ip", clientIP),
			slog.Duration("latency", latency),
			slog.Int("size", c.Writer.Size()),
		)
	}
}