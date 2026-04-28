package handler

import (
	"strconv"

	"github.com/cryptox/shared/response"
	"github.com/cryptox/auth-service/internal/service"
	"github.com/gin-gonic/gin"
)

type AdminHandler struct {
	admin *service.AdminService
}

func NewAdminHandler(admin *service.AdminService) *AdminHandler {
	return &AdminHandler{admin: admin}
}

func pageParams(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return page, size
}

// GET /api/admin/users
func (h *AdminHandler) Users(c *gin.Context) {
	page, size := pageParams(c)
	search := c.Query("search")
	users, total, err := h.admin.GetUsers(page, size, search)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Page(c, users, total, page, size)
}

// PUT /api/admin/users/:id/kyc
func (h *AdminHandler) UpdateKYC(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid user id")
		return
	}
	var body struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := h.admin.UpdateKYC(uint(id), body.Status); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "KYC status updated"})
}

// GET /api/admin/users/:id
func (h *AdminHandler) UserDetail(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid user id")
		return
	}
	user, err := h.admin.GetUserByID(uint(id))
	if err != nil {
		response.Error(c, 404, "user not found")
		return
	}
	response.OK(c, user)
}

// GET /api/admin/users/:id/kyc (read KYC status + profile + documents)
func (h *AdminHandler) UserKycDetail(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid user id")
		return
	}
	data, err := h.admin.GetUserKYCDetail(uint(id))
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	response.OK(c, data)
}

// GET /api/admin/stats
func (h *AdminHandler) Stats(c *gin.Context) {
	stats, err := h.admin.GetStats()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, stats)
}

// GET /api/admin/charts
func (h *AdminHandler) Charts(c *gin.Context) {
	data, err := h.admin.GetChartData()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, data)
}
