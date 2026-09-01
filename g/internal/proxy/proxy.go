package proxy

import (
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

// Resolve 根据 enableProxy 开关和环境变量，决定是否启用代理。
// 返回 (proxyURL, enabled)。
//   - enableProxy=false 时：忽略环境变量，直接连接，返回 (nil, false)
//   - enableProxy=true 时：读取环境变量 HTTP_PROXY/HTTPS_PROXY，若有则返回代理地址，否则返回 (nil, false)
//
// 不在此处打日志，由调用方（mcp / llm）自行打印，以便区分是哪个组件的代理。
func Resolve(enableProxy bool) (*url.URL, bool) {
	if !enableProxy {
		return nil, false
	}

	proxyURL := getEnvProxy()
	if proxyURL == "" {
		return nil, false
	}

	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return nil, false
	}

	return parsed, true
}

// getEnvProxy 从环境变量读取代理地址
func getEnvProxy() string {
	// 优先读取大写，其次小写
	for _, key := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}

// Transport 根据代理配置创建 HTTP Transport
func Transport(enableProxy bool) *http.Transport {
	tr := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
	}

	proxyURL, enabled := Resolve(enableProxy)
	if enabled && proxyURL != nil {
		tr.Proxy = http.ProxyURL(proxyURL)
	}

	return tr
}