package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/aibao/server/internal/api/userctx"
	"github.com/aibao/server/internal/model"
	apperr "github.com/aibao/server/internal/pkg/errors"
	"github.com/aibao/server/internal/pkg/logger"
	"github.com/aibao/server/internal/repository"
)

// HeartbeatChildReader is the minimal child-repo surface the handler needs.
type HeartbeatChildReader interface {
	FindByID(ctx context.Context, id int64) (*model.Child, error)
}

// HeartbeatStorylineReader is the minimal storyline-repo surface the handler needs.
type HeartbeatStorylineReader interface {
	ListActiveByChild(ctx context.Context, childID int64, limit int) ([]*model.Storyline, error)
}

// HeartbeatHandler serves the GET /heartbeat pseudo-push endpoint.
type HeartbeatHandler struct {
	children    HeartbeatChildReader
	storylines  HeartbeatStorylineReader
	now         func() time.Time
	housekeeper OutlineHousekeeper // nil-safe; injected via WithHousekeeper
}

// NewHeartbeatHandler constructs a HeartbeatHandler.
func NewHeartbeatHandler(c HeartbeatChildReader, sr HeartbeatStorylineReader, now func() time.Time) *HeartbeatHandler {
	if now == nil {
		now = time.Now
	}
	return &HeartbeatHandler{children: c, storylines: sr, now: now}
}

// WithHousekeeper wires the outline housekeeper used by GET /heartbeat to
// opportunistically expire abandoned pending outlines for the active user
// (Plan 11A §5.5 A2). Returns the receiver for chaining at construction time.
func (h *HeartbeatHandler) WithHousekeeper(hk OutlineHousekeeper) *HeartbeatHandler {
	h.housekeeper = hk
	return h
}

// RegisterRoutes mounts /heartbeat on the given authenticated group.
func (h *HeartbeatHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/heartbeat", h.heartbeat)
}

func (h *HeartbeatHandler) heartbeat(c *gin.Context) {
	uid, ok := userctx.FromContext(c.Request.Context())
	if !ok {
		RespondError(c, apperr.New(apperr.CodeUnauthenticated, "unauthorized", "请先登录"))
		return
	}
	childIDStr := c.Query("child_id")
	if childIDStr == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_argument", "user_msg": "缺少 child_id"})
		return
	}
	childID, err := strconv.ParseInt(childIDStr, 10, 64)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_argument", "user_msg": "child_id 不合法"})
		return
	}

	child, err := h.children.FindByID(c.Request.Context(), childID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			RespondError(c, apperr.New(apperr.CodeNotFound, "child_not_found", "未找到该孩子档案"))
			return
		}
		RespondError(c, apperr.Wrap(err, apperr.CodeInternal, "child_lookup_failed", "服务暂时不可用，请稍后再试"))
		return
	}
	if child.UserID != uid {
		RespondError(c, apperr.New(apperr.CodePermissionDenied, "not_owner", "无权访问该孩子档案"))
		return
	}

	// Plan 11A §5.5 A2: opportunistic outline housekeeping on every heartbeat.
	if h.housekeeper != nil {
		h.housekeeper.SweepUser(c.Request.Context(), uid)
	}

	lines, lErr := h.storylines.ListActiveByChild(c.Request.Context(), childID, 5)
	if lErr != nil {
		logger.FromCtx(c.Request.Context()).Warn("heartbeat.list_storylines.fail", "child_id", childID, "err", lErr.Error())
		lines = nil
	}

	hour := h.now().Hour()
	greeting := buildGreeting(child.Nickname, hour, len(lines) > 0)

	items := make([]gin.H, 0, len(lines))
	for _, sl := range lines {
		items = append(items, gin.H{
			"id":              sl.ID,
			"title":           sl.Title,
			"episode_count":   sl.EpisodeCount,
			"next_hint":       sl.NextEpisodeHint,
			"last_episode_at": sl.LastEpisodeAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"greeting":          greeting,
		"active_storylines": items,
	})
}

// buildGreeting composes the time-of-day greeting and (optionally) appends a
// continuation prompt when the child has active storylines AND the hour is
// outside the late-night band.
func buildGreeting(nickname string, hour int, hasStorylines bool) string {
	var base string
	lateNight := false
	switch {
	case hour >= 5 && hour < 11:
		base = nickname + "早上好呀～"
	case hour >= 11 && hour < 14:
		base = nickname + "中午好呀～"
	case hour >= 14 && hour < 18:
		base = nickname + "下午好呀～"
	case hour >= 18 && hour < 23:
		base = nickname + "晚上好呀～"
	default:
		base = nickname + "夜深了，今天还想听一个故事吗？"
		lateNight = true
	}
	if hasStorylines && !lateNight {
		base += "想继续之前的冒险吗？"
	}
	return base
}
