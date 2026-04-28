package handler

import (
	"strconv"
	"time"

	"github.com/cryptox/auth-service/internal/service"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/response"
	"github.com/gin-gonic/gin"
)

type APIKeyHandler struct {
	svc *service.APIKeyService
}

func NewAPIKeyHandler(svc *service.APIKeyService) *APIKeyHandler {
	return &APIKeyHandler{svc: svc}
}

type createAPIKeyReq struct {
	Label       string  `json:"label" binding:"required"`
	Permissions string  `json:"permissions"` // CSV: read,trade,withdraw
	IPWhitelist string  `json:"ipWhitelist"` // CSV
	ExpiresInDays *int  `json:"expiresInDays"`
}

func (h *APIKeyHandler) Create(c *gin.Context) {
	var req createAPIKeyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	var expires *time.Time
	if req.ExpiresInDays != nil && *req.ExpiresInDays > 0 {
		t := time.Now().AddDate(0, 0, *req.ExpiresInDays)
		expires = &t
	}
	userID := middleware.GetUserID(c)
	res, err := h.svc.Create(userID, req.Label, req.Permissions, req.IPWhitelist, expires)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	// secret is shown ONCE — clients must save it now.
	response.Created(c, res)
}

func (h *APIKeyHandler) List(c *gin.Context) {
	userID := middleware.GetUserID(c)
	keys, err := h.svc.List(userID)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, keys)
}

func (h *APIKeyHandler) Revoke(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid id")
		return
	}
	userID := middleware.GetUserID(c)
	if err := h.svc.Revoke(userID, uint(id)); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "api key revoked"})
}
