package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/aibao/server/internal/api/userctx"
	"github.com/aibao/server/internal/model"
	apperr "github.com/aibao/server/internal/pkg/errors"
)

// MeUserLookup is the surface MeHandler needs to load a user by id.
type MeUserLookup interface {
	FindByID(ctx context.Context, id int64) (*model.User, error)
}

// MeHandler serves /me.
type MeHandler struct {
	users MeUserLookup
}

// NewMeHandler constructs a MeHandler.
func NewMeHandler(users MeUserLookup) *MeHandler { return &MeHandler{users: users} }

// RegisterRoutes mounts /me on the supplied authenticated group.
func (h *MeHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/me", h.me)
}

func (h *MeHandler) me(c *gin.Context) {
	uid, ok := userctx.FromContext(c.Request.Context())
	if !ok {
		RespondError(c, apperr.New(apperr.CodeUnauthenticated, "unauthorized", "请先登录"))
		return
	}
	u, err := h.users.FindByID(c.Request.Context(), uid)
	if err != nil {
		RespondError(c, apperr.Wrap(err, apperr.CodeNotFound, "user_not_found", "用户不存在"))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":                u.ID,
		"nickname":          u.Nickname,
		"subscription_tier": u.SubscriptionTier,
	})
}
