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

// refresh handles POST /api/v1/outlines/:id/refresh. Task 20 will implement
// the full pipeline (parent outline lookup → group inheritance → new variant).
// For now we return 501 so the route exists for shared-bucket rate-limit wiring.
func (h *OutlineHandler) refresh(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusNotImplemented, gin.H{
		"reason":   "not_implemented",
		"user_msg": "刷新功能开发中",
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
