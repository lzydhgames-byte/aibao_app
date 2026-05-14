package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/aibao/server/internal/api/userctx"
	"github.com/aibao/server/internal/model"
	apperr "github.com/aibao/server/internal/pkg/errors"
	"github.com/aibao/server/internal/service/story"
)

// ChildLookup is the minimal child-repo surface StoryHandler needs for
// ownership checks (ISP: depend on what we use, not the full repo).
type ChildLookup interface {
	FindByID(ctx context.Context, id int64) (*model.Child, error)
}

// StoryQuery is the StoryHandler-local view of the story repo. Wider than
// story.StoryRepo because the handler also needs ListByChild for GET /stories.
type StoryQuery interface {
	FindByID(ctx context.Context, id int64) (*model.Story, error)
	ListByChild(ctx context.Context, childID int64, limit int) ([]*model.Story, error)
}

// StoryHandler exposes the story generation + lookup endpoints.
type StoryHandler struct {
	orch     *story.Orchestrator
	repo     StoryQuery
	children ChildLookup
}

// NewStoryHandler constructs a StoryHandler.
func NewStoryHandler(orch *story.Orchestrator, repo StoryQuery, children ChildLookup) *StoryHandler {
	return &StoryHandler{orch: orch, repo: repo, children: children}
}

// RegisterRoutes mounts /stories/* on an authenticated group.
//
// IMPORTANT: GET /stories MUST be registered BEFORE GET /stories/:id; gin
// matches routes in registration order and `:id` would otherwise shadow the
// list endpoint.
func (h *StoryHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.POST("/stories/generate", h.generate)
	g.GET("/stories", h.list)
	g.GET("/stories/:id", h.get)
}

type generateReq struct {
	ChildID        int64  `json:"child_id" binding:"required"`
	Prompt         string `json:"prompt" binding:"required"`
	Duration       int    `json:"duration" binding:"required"`
	Style          string `json:"style" binding:"required"`
	Topic          string `json:"topic"`
	StartStoryline bool   `json:"start_storyline"`
	StorylineID    *int64 `json:"storyline_id,omitempty"`
}

func (h *StoryHandler) generate(c *gin.Context) {
	uid, ok := userctx.FromContext(c.Request.Context())
	if !ok {
		RespondError(c, apperr.New(apperr.CodeUnauthenticated, "unauthorized", "请先登录"))
		return
	}
	var req generateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_argument", "user_msg": "请求参数不合法"})
		return
	}
	if req.Duration != 5 && req.Duration != 10 && req.Duration != 15 {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_duration", "user_msg": "duration 必须是 5/10/15"})
		return
	}
	if req.StartStoryline && req.StorylineID != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"reason":   "invalid_argument",
			"user_msg": "不能同时启动新连续剧和续接已有连续剧",
		})
		return
	}
	out, err := h.orch.Generate(c.Request.Context(), story.GenerateParams{
		ChildID: req.ChildID, UserID: uid,
		Prompt: req.Prompt, Duration: req.Duration, Style: req.Style, Topic: req.Topic,
		StartStoryline: req.StartStoryline, StorylineID: req.StorylineID,
	})
	if err != nil {
		RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, storyJSON(out))
}

func (h *StoryHandler) get(c *gin.Context) {
	uid, ok := userctx.FromContext(c.Request.Context())
	if !ok {
		RespondError(c, apperr.New(apperr.CodeUnauthenticated, "unauthorized", "请先登录"))
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_id", "user_msg": "id 不合法"})
		return
	}
	s, err := h.repo.FindByID(c.Request.Context(), id)
	if err != nil {
		RespondError(c, apperr.New(apperr.CodeNotFound, "story_not_found", "未找到该故事"))
		return
	}
	_ = uid
	c.JSON(http.StatusOK, storyJSON(s))
}

func (h *StoryHandler) list(c *gin.Context) {
	uid, ok := userctx.FromContext(c.Request.Context())
	if !ok {
		RespondError(c, apperr.New(apperr.CodeUnauthenticated, "unauthorized", "请先登录"))
		return
	}
	childIDStr := c.Query("child_id")
	if childIDStr == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "missing_child_id", "user_msg": "缺少 child_id"})
		return
	}
	childID, err := strconv.ParseInt(childIDStr, 10, 64)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_child_id", "user_msg": "child_id 不合法"})
		return
	}
	limit := 5
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 50 {
		limit = 50
	}

	ch, err := h.children.FindByID(c.Request.Context(), childID)
	if err != nil || ch == nil {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"reason": "child_not_found", "user_msg": "未找到该孩子"})
		return
	}
	if ch.UserID != uid {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"reason": "forbidden", "user_msg": "无权访问"})
		return
	}

	items, err := h.repo.ListByChild(c.Request.Context(), childID, limit)
	if err != nil {
		RespondError(c, err)
		return
	}
	out := make([]gin.H, 0, len(items))
	for _, s := range items {
		out = append(out, storyJSON(s))
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func storyJSON(s *model.Story) gin.H {
	return gin.H{
		"id":               s.ID,
		"title":            s.Title,
		"text":             s.TextContent,
		"audio_object_key": s.AudioObjectKey,
		"audio_status":     s.AudioStatus,
		"duration_minutes": s.DurationMinutes,
		"style":            s.Style,
		"topic":            s.Topic,
		"storyline_id":     s.StorylineID,
		"episode_no":       s.EpisodeNo,
		"created_at":       s.CreatedAt,
	}
}
