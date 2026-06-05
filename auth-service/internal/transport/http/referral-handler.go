package http

import (
	"strconv"

	"github.com/cryptox/auth-service/internal/usecase"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/response"
	"github.com/gin-gonic/gin"
)

type ReferralHandler struct {
	svc *usecase.ReferralService
}

func NewReferralHandler(svc *usecase.ReferralService) *ReferralHandler {
	return &ReferralHandler{svc: svc}
}

func (h *ReferralHandler) MyCode(c *gin.Context) {
	userID := middleware.GetUserID(c)
	code, err := h.svc.MyCode(userID)
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.OK(c, code)
}

func (h *ReferralHandler) MyStats(c *gin.Context) {
	userID := middleware.GetUserID(c)
	stats, err := h.svc.MyStats(userID)
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.OK(c, stats)
}

func (h *ReferralHandler) MyReferees(c *gin.Context) {
	userID := middleware.GetUserID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	rows, total, err := h.svc.ListReferees(userID, page, size)
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.Page(c, rows, total, page, size)
}

func (h *ReferralHandler) MyCommissions(c *gin.Context) {
	userID := middleware.GetUserID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	rows, total, err := h.svc.ListCommissions(userID, page, size)
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.Page(c, rows, total, page, size)
}
