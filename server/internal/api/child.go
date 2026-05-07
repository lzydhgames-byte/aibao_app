package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/aibao/server/internal/api/userctx"
	"github.com/aibao/server/internal/model"
	apperr "github.com/aibao/server/internal/pkg/errors"
	"github.com/aibao/server/internal/service/child"
)

// ChildHandler serves /children endpoints.
type ChildHandler struct {
	svc *child.Service
}

// NewChildHandler constructs a ChildHandler.
func NewChildHandler(svc *child.Service) *ChildHandler { return &ChildHandler{svc: svc} }

// RegisterRoutes mounts the /children routes on an authenticated group.
func (h *ChildHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.POST("/children", h.create)
	g.GET("/children", h.list)
	g.PATCH("/children/:id", h.update)
}

type createChildReq struct {
	Nickname string `json:"nickname" binding:"required"`
	Gender   string `json:"gender" binding:"required"`
	Birthday string `json:"birthday" binding:"required"`
}

type updateChildReq struct {
	Nickname *string `json:"nickname,omitempty"`
	Gender   *string `json:"gender,omitempty"`
	Birthday *string `json:"birthday,omitempty"`
}

func (h *ChildHandler) requireUser(c *gin.Context) (int64, bool) {
	uid, ok := userctx.FromContext(c.Request.Context())
	if !ok {
		RespondError(c, apperr.New(apperr.CodeUnauthenticated, "unauthorized", "请先登录"))
		return 0, false
	}
	return uid, true
}

func (h *ChildHandler) create(c *gin.Context) {
	uid, ok := h.requireUser(c)
	if !ok {
		return
	}
	var req createChildReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_argument", "user_msg": "请求参数不合法"})
		return
	}
	bday, err := time.Parse("2006-01-02", req.Birthday)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_birthday", "user_msg": "生日格式应为 YYYY-MM-DD"})
		return
	}
	out, err := h.svc.Create(c.Request.Context(), uid, child.CreateInput{
		Nickname: req.Nickname, Gender: req.Gender, Birthday: bday,
	})
	if err != nil {
		if ae, ok := apperr.AsAppError(err); ok && ae.Reason == "child_already_exists" {
			c.AbortWithStatusJSON(http.StatusConflict, gin.H{"reason": ae.Reason, "user_msg": ae.UserMsg})
			return
		}
		RespondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, childJSON(out))
}

func (h *ChildHandler) list(c *gin.Context) {
	uid, ok := h.requireUser(c)
	if !ok {
		return
	}
	items, err := h.svc.ListByUser(c.Request.Context(), uid)
	if err != nil {
		RespondError(c, err)
		return
	}
	out := make([]gin.H, 0, len(items))
	for _, it := range items {
		out = append(out, childJSON(it))
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func (h *ChildHandler) update(c *gin.Context) {
	uid, ok := h.requireUser(c)
	if !ok {
		return
	}
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_id", "user_msg": "id 不合法"})
		return
	}
	var req updateChildReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_argument", "user_msg": "请求参数不合法"})
		return
	}
	in := child.UpdateInput{Nickname: req.Nickname, Gender: req.Gender}
	if req.Birthday != nil {
		t, err := time.Parse("2006-01-02", *req.Birthday)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_birthday", "user_msg": "生日格式应为 YYYY-MM-DD"})
			return
		}
		in.Birthday = &t
	}
	out, err := h.svc.Update(c.Request.Context(), uid, id, in)
	if err != nil {
		RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, childJSON(out))
}

// childJSON shapes the JSON response for a Child object.
func childJSON(c *model.Child) gin.H {
	return gin.H{
		"id":       c.ID,
		"user_id":  c.UserID,
		"nickname": c.Nickname,
		"gender":   c.Gender,
		"birthday": c.Birthday.Format("2006-01-02"),
		"profile":  string(c.Profile),
	}
}
