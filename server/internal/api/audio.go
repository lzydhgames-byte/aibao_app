package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/aibao/server/internal/api/userctx"
	"github.com/aibao/server/internal/gateway/storage"
	"github.com/aibao/server/internal/model"
	apperr "github.com/aibao/server/internal/pkg/errors"
	"github.com/aibao/server/internal/repository"
)

// AudioStoryReader is the minimal story-read surface this handler needs.
type AudioStoryReader interface {
	FindByID(ctx context.Context, id int64) (*model.Story, error)
}

// AudioChildReader is the minimal child-read surface this handler needs.
type AudioChildReader interface {
	FindByID(ctx context.Context, id int64) (*model.Child, error)
}

// AudioHandler serves GET /stories/:id/audio_url.
type AudioHandler struct {
	stories  AudioStoryReader
	children AudioChildReader
	storage  storage.Client
	ttl      time.Duration
}

// NewAudioHandler constructs the handler.
func NewAudioHandler(s AudioStoryReader, c AudioChildReader, st storage.Client, ttl time.Duration) *AudioHandler {
	if ttl == 0 {
		ttl = 15 * time.Minute
	}
	return &AudioHandler{stories: s, children: c, storage: st, ttl: ttl}
}

// RegisterRoutes hooks the handler into a gin group.
func (h *AudioHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/stories/:id/audio_url", h.GetAudioURL)
}

type audioURLResponse struct {
	AudioStatus string    `json:"audio_status"`
	URL         string    `json:"url,omitempty"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
	RetryAfter  int       `json:"retry_after,omitempty"`
}

// GetAudioURL is the gin handler.
func (h *AudioHandler) GetAudioURL(c *gin.Context) {
	uid, ok := userctx.FromContext(c.Request.Context())
	if !ok {
		RespondError(c, apperr.New(apperr.CodeUnauthenticated, "unauthorized", "请先登录"))
		return
	}

	storyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		RespondError(c, apperr.New(apperr.CodeInvalidArgument, "invalid_id", "故事 ID 非法"))
		return
	}

	story, err := h.stories.FindByID(c.Request.Context(), storyID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			RespondError(c, apperr.New(apperr.CodeNotFound, "story_not_found", "故事不存在"))
			return
		}
		RespondError(c, apperr.Wrap(err, apperr.CodeInternal, "story_load_failed", "服务暂时不可用"))
		return
	}

	child, err := h.children.FindByID(c.Request.Context(), story.ChildID)
	if err != nil || child.UserID != uid {
		RespondError(c, apperr.New(apperr.CodePermissionDenied, "not_owner", "无权访问该故事"))
		return
	}

	switch story.AudioStatus {
	case model.AudioStatusReady:
		if story.AudioObjectKey == "" {
			c.JSON(http.StatusOK, audioURLResponse{AudioStatus: model.AudioStatusPending, RetryAfter: 5})
			return
		}
		url, exp, err := h.storage.GetPresignedURL(c.Request.Context(), story.AudioObjectKey, h.ttl)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"code":    "audio_failed",
				"message": "音频生成失败，请稍后重新生成故事",
			})
			return
		}
		c.JSON(http.StatusOK, audioURLResponse{
			AudioStatus: model.AudioStatusReady,
			URL:         url,
			ExpiresAt:   exp,
		})
	case model.AudioStatusFailed:
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"code":    "audio_failed",
			"message": "音频生成失败，请稍后重新生成故事",
		})
	default:
		c.JSON(http.StatusOK, audioURLResponse{
			AudioStatus: model.AudioStatusPending,
			RetryAfter:  5,
		})
	}
}
