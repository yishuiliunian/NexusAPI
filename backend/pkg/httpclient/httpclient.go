// Package httpclient 提供带超时和重试的 HTTP 客户端。
package httpclient

import (
	"net"
	"net/http"
	"time"
)

// New 构造默认 HTTP 客户端。适合中等 AI 模型调用场景（可能需要长时间等待流式响应）。
func New() *http.Client {
	return &http.Client{
		// 不设顶层 Timeout（流式可能跑很久），由每次 request 的 ctx 控制。
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}
