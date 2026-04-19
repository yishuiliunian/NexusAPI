package provider

import (
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

// defaultClient 是给 task adaptor 等"非流式"provider 使用的共享 HTTP 客户端。
//
// 包提供 init 时的默认值（30s dial / 90s idle），main.go 启动后可通过
// SetHTTPClient 覆盖为带超时/连接池的实例（typically 走 pkg/httpclient.New）。
var defaultClient atomic.Pointer[http.Client]

func init() {
	defaultClient.Store(&http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
		},
		Timeout: 30 * time.Second,
	})
}

// HTTPClient 返回 task provider 等应使用的共享 HTTP 客户端。
func HTTPClient() *http.Client { return defaultClient.Load() }

// SetHTTPClient 替换共享客户端。通常在 main 启动时调用，把 pkg/httpclient.New() 传入。
func SetHTTPClient(c *http.Client) {
	if c == nil {
		return
	}
	defaultClient.Store(c)
}
