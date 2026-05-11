// Package ws 实现 /realtime WebSocket 中转（OpenAI Realtime API 兼容）。
//
// 设计：
//   - 客户端连我们：wss://nexus/realtime?model=gpt-4o-realtime-preview
//   - 我们按"模型 → 可用渠道"选一家，建到上游的 WebSocket 连接
//   - 两方向 goroutine 双向 pipe；任一端关闭另一端也关
//
// 本实现暂不做按时长计费（上游通常按 token + 分钟费计，业务侧可后补）；
// Usage 记录在会话结束时按 token 估算粗略落库（tokens=0 亦可）。
package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	keysapp "github.com/yishuiliunian/nexusapi/backend/internal/app/keys"
	relayapp "github.com/yishuiliunian/nexusapi/backend/internal/app/relay"
	domainchannel "github.com/yishuiliunian/nexusapi/backend/internal/domain/channel"
	"github.com/yishuiliunian/nexusapi/backend/internal/interface/http/middleware"
)

// Handler WebSocket 中转。
type Handler struct {
	Selector *relayapp.Selector
	ApiKey   *keysapp.Service
	Log      *zap.Logger
}

// Register 挂载路由到 /realtime（需前置 AuthApiKey 中间件）。
func (h *Handler) Register(g *gin.RouterGroup) {
	g.GET("/realtime", h.proxy)
}

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(*http.Request) bool { return true },
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
}

func (h *Handler) proxy(c *gin.Context) {
	key := middleware.CurrentApiKey(c)
	if key == nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	model := c.Query("model")
	if model == "" {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if !key.AllowModel(model) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	ctx := c.Request.Context()
	var groupID uint64
	if u := middleware.CurrentUser(c); u != nil {
		groupID = u.GroupID
	}
	candidates, err := h.Selector.Candidates(ctx, model, groupID, key.UserID, key.ID)
	if err != nil || len(candidates) == 0 {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	ch := candidates[0] // Realtime 不做 failover（流已开始就无法回退）

	upstreamURL, err := buildUpstreamURL(ch, model)
	if err != nil {
		c.AbortWithStatus(http.StatusBadGateway)
		return
	}

	// 升级客户端 → WS
	clientConn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return // 已响应 400，不再写
	}
	defer clientConn.Close()

	// 连接上游
	dialHeader := http.Header{}
	dialHeader.Set("Authorization", "Bearer "+ch.Credentials)
	dialHeader.Set("OpenAI-Beta", "realtime=v1")
	upConn, resp, err := websocket.DefaultDialer.DialContext(ctx, upstreamURL, dialHeader)
	if err != nil {
		_ = writeClose(clientConn, websocket.CloseInternalServerErr, "upstream dial: "+errString(err, resp))
		if h.Log != nil {
			h.Log.Warn("realtime upstream dial failed", zap.Error(err))
		}
		return
	}
	defer upConn.Close()

	// 记一笔占位 usage（token=0），便于用户看到会话发生过
	_ = h.ApiKey.TouchUsed(ctx, key.ID)

	// 双向 pipe
	pipeWS(ctx, clientConn, upConn, h.Log)
}

// buildUpstreamURL 从 channel 拼接 OpenAI Realtime wss:// 地址。
// 非 OpenAI 渠道可通过 BaseURL 指定。
func buildUpstreamURL(ch *domainchannel.Channel, model string) (string, error) {
	base := ch.BaseURL
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	// http/https → ws/wss
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/realtime"
	q := u.Query()
	q.Set("model", model)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// pipeWS 在两个 ws 连接之间做全双工消息转发。任一端关闭都触发另一端关闭。
func pipeWS(ctx context.Context, client, upstream *websocket.Conn, log *zap.Logger) {
	done := make(chan struct{}, 2)

	copy := func(dst, src *websocket.Conn, dir string) {
		defer func() { done <- struct{}{} }()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			t, msg, err := src.ReadMessage()
			if err != nil {
				return
			}
			_ = dst.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := dst.WriteMessage(t, msg); err != nil {
				return
			}
		}
	}

	go copy(upstream, client, "client->upstream")
	go copy(client, upstream, "upstream->client")
	<-done
}

func writeClose(c *websocket.Conn, code int, reason string) error {
	return c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason))
}

func errString(err error, resp *http.Response) string {
	if resp != nil {
		b, _ := json.Marshal(map[string]any{"status": resp.StatusCode, "err": err.Error()})
		return string(b)
	}
	return err.Error()
}
