package handler

import (
	"strconv"

	"github.com/cryptox/auth-service/internal/service"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/response"
	"github.com/gin-gonic/gin"
)

type AuditLogHandler struct {
	svc *service.AuditLogService
}

func NewAuditLogHandler(svc *service.AuditLogService) *AuditLogHandler {
	return &AuditLogHandler{svc: svc}
}

// GET /api/auth/audit — current user's audit history.
func (h *AuditLogHandler) MyAudit(c *gin.Context) {
	userID := middleware.GetUserID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	rows, total, err := h.svc.ListByUser(userID, page, size)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Page(c, rows, total, page, size)
}

// GET /api/admin/audit?action=login.failure — admin-only platform-wide query.
func (h *AuditLogHandler) AdminAudit(c *gin.Context) {
	action := c.Query("action")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "50"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 50
	}
	rows, total, err := h.svc.ListAll(action, page, size)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Page(c, rows, total, page, size)
}
