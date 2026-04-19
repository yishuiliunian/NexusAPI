// Package taskhttp 实现 /mj /suno /video 等异步任务 HTTP 入口。
package taskhttp

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	keysapp "github.com/yishuiliunian/nexusapi/backend/internal/app/keys"
	taskapp "github.com/yishuiliunian/nexusapi/backend/internal/app/task"
	"github.com/yishuiliunian/nexusapi/backend/internal/interface/http/middleware"
	"github.com/yishuiliunian/nexusapi/backend/pkg/errors"
	"github.com/yishuiliunian/nexusapi/backend/pkg/httperr"
)

// Handler 任务接口。
type Handler struct {
	Tasks  *taskapp.Service
	ApiKey *keysapp.Service
}

// RegisterRelay 挂载 /mj /suno /video 入口（需 AuthApiKey 前置）。
func (h *Handler) RegisterRelay(g *gin.RouterGroup) {
	g.POST("/mj/submit/:action", h.submit("midjourney"))
	g.GET("/mj/task/:id/fetch", h.fetchExternal)

	g.POST("/suno/submit/:action", h.submit("suno"))
	g.GET("/suno/fetch/:id", h.fetchExternal)

	g.POST("/video/submit/:provider/:action", h.submitVideo)
}

// RegisterAPI 挂载 /api/user/tasks 列表（Session 鉴权）。
func (h *Handler) RegisterAPI(g *gin.RouterGroup) {
	g.GET("/tasks", h.listUserTasks)
	g.GET("/tasks/:id", h.getTask)
}

// submit 通用提交逻辑。
func (h *Handler) submit(providerName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := middleware.CurrentApiKey(c)
		if key == nil {
			httperr.Unauthorized(c, "")
			return
		}
		raw, _ := io.ReadAll(c.Request.Body)
		var input any
		_ = json.Unmarshal(raw, &input)

		t, err := h.Tasks.Submit(c.Request.Context(), taskapp.SubmitInput{
			UserID:   key.UserID,
			ApiKeyID: key.ID,
			Provider: providerName,
			Action:   c.Param("action"),
			Input:    input,
		})
		if err != nil {
			httperr.Abort(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code":        1,
			"result":      t.ID,
			"task_id":     t.ID,
			"description": "submitted",
		})
	}
}

func (h *Handler) submitVideo(c *gin.Context) {
	key := middleware.CurrentApiKey(c)
	if key == nil {
		httperr.Unauthorized(c, "")
		return
	}
	raw, _ := io.ReadAll(c.Request.Body)
	var input any
	_ = json.Unmarshal(raw, &input)
	t, err := h.Tasks.Submit(c.Request.Context(), taskapp.SubmitInput{
		UserID:   key.UserID,
		ApiKeyID: key.ID,
		Provider: c.Param("provider"),
		Action:   c.Param("action"),
		Input:    input,
	})
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"task_id": t.ID})
}

// fetchExternal 按 task id 返回任务信息，兼容 midjourney-proxy 风格。
func (h *Handler) fetchExternal(c *gin.Context) {
	t, err := h.Tasks.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *Handler) listUserTasks(c *gin.Context) {
	u := middleware.CurrentUser(c)
	if u == nil {
		httperr.Unauthorized(c, "")
		return
	}
	page, _ := strconv.Atoi(c.Query("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(c.Query("size"))
	if size < 1 || size > 200 {
		size = 20
	}
	items, total, err := h.Tasks.ListByUser(c.Request.Context(), u.ID, (page-1)*size, size)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total})
}

func (h *Handler) getTask(c *gin.Context) {
	t, err := h.Tasks.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	u := middleware.CurrentUser(c)
	if u == nil || t.UserID != u.ID {
		httperr.Abort(c, errors.ErrPermissionDenied)
		return
	}
	c.JSON(http.StatusOK, t)
}
