package http

import (
	"github.com/cryptox/auth-service/internal/usecase"
	"github.com/cryptox/shared/response"
	"github.com/cryptox/shared/types"
	"github.com/gin-gonic/gin"
)

// SettingsHandler handles platform settings admin endpoints.
type SettingsHandler struct {
	svc *usecase.SettingsService
}

func NewSettingsHandler(svc *usecase.SettingsService) *SettingsHandler {
	return &SettingsHandler{svc: svc}
}

// GET /api/admin/settings
func (h *SettingsHandler) GetSettings(c *gin.Context) {
	settings, err := h.svc.Get()
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.OK(c, settings)
}

// PUT /api/admin/settings
func (h *SettingsHandler) UpdateSettings(c *gin.Context) {
	var req types.PlatformSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := h.svc.Update(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	updated, err := h.svc.Get()
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.OK(c, updated)
}
