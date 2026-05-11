package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/aibao/server/internal/api/userctx"
	apperr "github.com/aibao/server/internal/pkg/errors"
	"github.com/aibao/server/internal/service/bootstrap"
)

// BootstrapSubmitter is the minimal service surface the handler needs.
// Kept as an interface so tests can inject a fake.
type BootstrapSubmitter interface {
	Submit(ctx context.Context, userID, childID int64, answers []bootstrap.Answer) (*bootstrap.Profile, error)
}

// BootstrapHandler exposes BOOTSTRAP form endpoints.
type BootstrapHandler struct {
	svc BootstrapSubmitter
}

// NewBootstrapHandler constructs.
func NewBootstrapHandler(svc BootstrapSubmitter) *BootstrapHandler {
	return &BootstrapHandler{svc: svc}
}

// RegisterRoutes mounts under v1 (caller is the JWT-auth group).
func (h *BootstrapHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/bootstrap/questions", h.questions)
	g.POST("/bootstrap/answers", h.answers)
}

type questionsResp struct {
	Version   int                  `json:"version"`
	Questions []bootstrap.Question `json:"questions"`
}

func (h *BootstrapHandler) questions(c *gin.Context) {
	c.JSON(http.StatusOK, questionsResp{
		Version:   bootstrap.Version,
		Questions: bootstrap.Questions(),
	})
}

type answersReq struct {
	ChildID int64              `json:"child_id" binding:"required"`
	Answers []bootstrap.Answer `json:"answers"  binding:"required"`
}

func (h *BootstrapHandler) answers(c *gin.Context) {
	uid, ok := userctx.FromContext(c.Request.Context())
	if !ok {
		RespondError(c, apperr.New(apperr.CodeUnauthenticated, "unauthorized", "请先登录"))
		return
	}
	var req answersReq
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, apperr.New(apperr.CodeInvalidArgument, "invalid_argument", err.Error()))
		return
	}
	profile, err := h.svc.Submit(c.Request.Context(), uid, req.ChildID, req.Answers)
	if err != nil {
		RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"child_id":    req.ChildID,
		"version":     profile.Version,
		"description": profile.Description,
	})
}
