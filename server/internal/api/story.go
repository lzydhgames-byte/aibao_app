package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/aibao/server/internal/api/userctx"
	"github.com/aibao/server/internal/model"
	apperr "github.com/aibao/server/internal/pkg/errors"
	"github.com/aibao/server/internal/service/story"
)

// StoryHandler exposes the story generation + lookup endpoints.
type StoryHandler struct {
	orch *story.Orchestrator
	repo story.StoryRepo
}

// NewStoryHandler constructs a StoryHandler.
func NewStoryHandler(orch *story.Orchestrator, repo story.StoryRepo) *StoryHandler {
	return &StoryHandler{orch: orch, repo: repo}
}

// RegisterRoutes mounts /stories/* on an authenticated group.
func (h *StoryHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.POST("/stories/generate", h.generate)
	g.GET("/stories/:id", h.get)
}

type generateReq struct {
	ChildID  int64  `json:"child_id" binding:"required"`
	Prompt   string `json:"prompt" binding:"required"`
	Duration int    `json:"duration" binding:"required"`
	Style    string `json:"style" binding:"required"`
	Topic    string `json:"topic"`
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
	out, err := h.orch.Generate(c.Request.Context(), story.GenerateParams{
		ChildID: req.ChildID, UserID: uid,
		Prompt: req.Prompt, Duration: req.Duration, Style: req.Style, Topic: req.Topic,
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
		"created_at":       s.CreatedAt,
	}
}
