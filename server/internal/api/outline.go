// Package api — outline preview handler (Plan 11A §6.1 Task 19).
package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/aibao/server/internal/api/userctx"
	"github.com/aibao/server/internal/model"
	apperr "github.com/aibao/server/internal/pkg/errors"
	"github.com/aibao/server/internal/service/outline"
	"github.com/aibao/server/internal/service/outlinecontract"
)

// OutlineHandler exposes POST /outlines/preview (and Task 20 will add refresh).
type OutlineHandler struct {
	svc      *outline.Service
	cache    *outline.Cache
	events   *outline.EventStore
	children ChildLookup
}

// NewOutlineHandler constructs the handler. cache/events are kept here so the
// Task 20 refresh handler can share the same wiring.
func NewOutlineHandler(svc *outline.Service, cache *outline.Cache, events *outline.EventStore, children ChildLookup) *OutlineHandler {
	return &OutlineHandler{svc: svc, cache: cache, events: events, children: children}
}

// RegisterRoutes mounts /outlines/* on an authenticated group. extra guards
// (e.g. shared per-user 5/min rate limit) are applied to BOTH preview and
// refresh so they share the same bucket (spec §6.4).
func (h *OutlineHandler) RegisterRoutes(g *gin.RouterGroup, extra ...gin.HandlerFunc) {
	previewChain := append(append([]gin.HandlerFunc{}, extra...), h.preview)
	refreshChain := append(append([]gin.HandlerFunc{}, extra...), h.refresh)
	g.POST("/outlines/preview", previewChain...)
	g.POST("/outlines/:id/refresh", refreshChain...)
}

type previewReq struct {
	ChildID     int64  `json:"child_id" binding:"required"`
	Prompt      string `json:"prompt" binding:"required,min=1,max=200"`
	DurationMin int    `json:"duration_min" binding:"required,oneof=3 5 8"`
}

type outlineJSON struct {
	Title                string   `json:"title"`
	Synopsis             string   `json:"synopsis"`
	Themes               []string `json:"themes"`
	Style                string   `json:"style"`
	EducationalValue     string   `json:"educational_value"`
	DurationMin          int      `json:"duration_min"`
	OutlineGroupID       string   `json:"outline_group_id,omitempty"`
	VariantIndex         int      `json:"variant_index,omitempty"`
	OutlinePromptVersion string   `json:"outline_prompt_version,omitempty"`
}

type previewResp struct {
	OutlineID string      `json:"outline_id"`
	Outline   outlineJSON `json:"outline"`
	ExpiresAt time.Time   `json:"expires_at"`
}

func outlineToJSON(o outlinecontract.Outline) outlineJSON {
	return outlineJSON{
		Title:                o.Title,
		Synopsis:             o.Synopsis,
		Themes:               o.Themes,
		Style:                o.Style,
		EducationalValue:     o.EducationalValue,
		DurationMin:          o.DurationMin,
		OutlineGroupID:       o.OutlineGroupID,
		VariantIndex:         o.VariantIndex,
		OutlinePromptVersion: o.OutlinePromptVersion,
	}
}

// preview handles POST /api/v1/outlines/preview.
func (h *OutlineHandler) preview(c *gin.Context) {
	uid, ok := userctx.FromContext(c.Request.Context())
	if !ok {
		RespondError(c, apperr.New(apperr.CodeUnauthenticated, "unauthorized", "请先登录"))
		return
	}
	var req previewReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"reason":   "invalid_argument",
			"user_msg": "请求参数不合法",
		})
		return
	}

	child, err := h.children.FindByID(c.Request.Context(), req.ChildID)
	if err != nil || child == nil {
		RespondError(c, apperr.New(apperr.CodePermissionDenied, "child_not_yours", "该孩子档案不存在或不属于你"))
		return
	}
	if child.UserID != uid {
		RespondError(c, apperr.New(apperr.CodePermissionDenied, "child_not_yours", "该孩子档案不属于你"))
		return
	}

	res, err := h.svc.Preview(c.Request.Context(), outline.PreviewInput{
		UserID:        uid,
		ChildID:       req.ChildID,
		ChildNickname: child.Nickname,
		ChildAge:      childAgeYears(child.Birthday),
		ChildFears:    nil, // TODO Task 23: wire from child profile / bootstrap-fears memory
		IPBlacklist:   nil, // TODO Task 23: wire from safety.RuleSet
		IPWhitelist:   nil,
		Prompt:        req.Prompt,
		DurationMin:   req.DurationMin,
	})
	if err != nil {
		var ae *apperr.AppError
		if errors.As(err, &ae) {
			RespondError(c, ae)
			return
		}
		RespondError(c, apperr.New(apperr.CodeInternal, "internal", "服务器开小差了"))
		return
	}

	c.JSON(http.StatusOK, previewResp{
		OutlineID: res.OutlineID,
		Outline:   outlineToJSON(res.Outline),
		ExpiresAt: res.ExpiresAt,
	})
}

// refresh handles POST /api/v1/outlines/:id/refresh.
// "换个角度" — invalidates the parent outline, appends a refreshed event, then
// regenerates a new outline in the same outline_group_id with VariantIndex++.
// Spec §6.2 + §10.3: ratelimit bucket shared with preview (5/min combined per user).
func (h *OutlineHandler) refresh(c *gin.Context) {
	uid, ok := userctx.FromContext(c.Request.Context())
	if !ok {
		RespondError(c, apperr.New(apperr.CodeUnauthenticated, "unauthorized", "请先登录"))
		return
	}
	parentID := c.Param("id")
	if parentID == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"reason":   "invalid_argument",
			"user_msg": "outline_id 缺失",
		})
		return
	}

	// Step 1: cache lookup for parent (404 if missed/expired — spec §5.2).
	parent, err := h.cache.Get(c.Request.Context(), parentID)
	if errors.Is(err, outline.ErrCacheMiss) {
		RespondError(c, apperr.New(apperr.CodeNotFound, "outline_not_found", "outline 不存在或已过期"))
		return
	}
	if err != nil {
		RespondError(c, apperr.New(apperr.CodeInternal, "cache_get", "服务器开小差了"))
		return
	}

	// Step 2: ownership check.
	if parent.UserID != uid {
		RespondError(c, apperr.New(apperr.CodePermissionDenied, "outline_not_yours", "outline 不属于你"))
		return
	}

	// Step 3: best-effort invalidate + refreshed event. Neither failure must
	// block Preview (spec §3.3): logical-only duplicate group_id is fine.
	_ = h.cache.Invalidate(c.Request.Context(), parentID)
	_ = h.events.Append(c.Request.Context(), model.OutlineEvent{
		OutlineID:            parentID,
		OutlineGroupID:       parent.OutlineGroupID,
		UserID:               uid,
		Outcome:              outline.OutcomeRefreshed,
		OutlinePromptVersion: parent.OutlinePromptVersion,
		DurationMin:          parent.DurationMin,
	})

	// Step 4: fetch child for Preview (own re-verification).
	child, err := h.children.FindByID(c.Request.Context(), parent.ChildID)
	if err != nil || child == nil || child.UserID != uid {
		RespondError(c, apperr.New(apperr.CodePermissionDenied, "child_not_yours", "孩子档案缺失"))
		return
	}

	// Step 5: regenerate; Service.Preview inherits group_id + bumps variant_index
	// when ParentOutlineID is set.
	res, err := h.svc.Preview(c.Request.Context(), outline.PreviewInput{
		UserID:          uid,
		ChildID:         parent.ChildID,
		ChildNickname:   child.Nickname,
		ChildAge:        childAgeYears(child.Birthday),
		ChildFears:      nil, // TODO Task 23
		IPBlacklist:     nil,
		IPWhitelist:     nil,
		Prompt:          parent.PromptText,
		DurationMin:     parent.DurationMin,
		ParentOutlineID: parentID,
	})
	if err != nil {
		var ae *apperr.AppError
		if errors.As(err, &ae) {
			RespondError(c, ae)
			return
		}
		RespondError(c, apperr.New(apperr.CodeInternal, "internal", "服务器开小差了"))
		return
	}

	c.JSON(http.StatusOK, previewResp{
		OutlineID: res.OutlineID,
		Outline:   outlineToJSON(res.Outline),
		ExpiresAt: res.ExpiresAt,
	})
}

// childAgeYears derives current age in completed years from birthday. Mirrors
// the helper in service/story so handlers don't depend on the orchestrator.
func childAgeYears(b time.Time) int {
	if b.IsZero() {
		return 0
	}
	now := time.Now()
	years := now.Year() - b.Year()
	if now.YearDay() < b.YearDay() {
		years--
	}
	if years < 0 {
		years = 0
	}
	return years
}

// compile-time guard: ChildLookup must satisfy our needs.
var _ = func(c ChildLookup) func(context.Context, int64) (*model.Child, error) { return c.FindByID }
