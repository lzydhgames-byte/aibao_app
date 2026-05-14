package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/aibao/server/internal/service/auth"
)

// AuthHandler exposes the SMS / login_or_register endpoints.
type AuthHandler struct {
	svc *auth.Service
}

// NewAuthHandler constructs an AuthHandler.
func NewAuthHandler(svc *auth.Service) *AuthHandler { return &AuthHandler{svc: svc} }

// RegisterRoutes attaches /auth/* routes under the supplied router group.
func (h *AuthHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.POST("/auth/sms/send", h.smsSend)
	g.POST("/auth/login_or_register", h.loginOrRegister)
}

type smsSendReq struct {
	Phone string `json:"phone" binding:"required"`
}

func (h *AuthHandler) smsSend(c *gin.Context) {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_argument", "user_msg": "请求参数不合法"})
		return
	}
	if !utf8.Valid(raw) {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_nickname", "user_msg": "昵称包含非法字节，请确保为 UTF-8"})
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	var req smsSendReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_argument", "user_msg": "请求参数不合法"})
		return
	}
	if err := h.svc.SendSMS(c.Request.Context(), req.Phone); err != nil {
		RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"sent": true})
}

type loginOrRegisterReq struct {
	Phone    string `json:"phone" binding:"required"`
	Code     string `json:"code" binding:"required"`
	Nickname string `json:"nickname"`
}

func (h *AuthHandler) loginOrRegister(c *gin.Context) {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_argument", "user_msg": "请求参数不合法"})
		return
	}
	if !utf8.Valid(raw) {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_nickname", "user_msg": "昵称包含非法字节，请确保为 UTF-8"})
		return
	}
	var req loginOrRegisterReq
	if err := json.Unmarshal(raw, &req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_argument", "user_msg": "请求参数不合法"})
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	if req.Phone == "" || req.Code == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"reason": "invalid_argument", "user_msg": "请求参数不合法"})
		return
	}
	out, err := h.svc.LoginOrRegister(c.Request.Context(), req.Phone, req.Code, req.Nickname)
	if err != nil {
		RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"access_token":  out.AccessToken,
		"refresh_token": out.RefreshToken,
		"user": gin.H{
			"id":                out.User.ID,
			"nickname":          out.User.Nickname,
			"subscription_tier": out.User.SubscriptionTier,
		},
	})
}
