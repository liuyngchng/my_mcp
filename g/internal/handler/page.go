package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// PageHandler 页面处理器
type PageHandler struct {
	Title string
}

// NewPageHandler 创建页面处理器
func NewPageHandler(title string) *PageHandler {
	return &PageHandler{Title: title}
}

// Index 首页
func (h *PageHandler) Index(c *gin.Context) {
	c.HTML(http.StatusOK, "index.html", gin.H{
		"page_title": h.Title,
	})
}