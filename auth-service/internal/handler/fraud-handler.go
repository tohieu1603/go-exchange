package handler

import (
	"strconv"

	"github.com/cryptox/auth-service/internal/service"
	"github.com/cryptox/shared/response"
	"github.com/gin-gonic/gin"
)

type FraudHandler struct {
	fraudSvc *service.FraudService
}

func NewFraudHandler(fraudSvc *service.FraudService) *FraudHandler {
	return &FraudHandler{fraudSvc: fraudSvc}
}

// GET /api/admin/fraud
func (h *FraudHandler) ListFraudLogs(c *gin.Context) {
	page, size := pageParams(c)
	search := c.Query("search")
	logs, total, err := h.fraudSvc.GetFraudLogs(page, size, search)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Page(c, logs, total, page, size)
}

// PUT /api/admin/fraud/:id
func (h *FraudHandler) UpdateFraudAction(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid fraud log id")
		return
	}
	var body struct {
		Action string `json:"action" binding:"required"`
		Note   string `json:"note"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := h.fraudSvc.UpdateFraudAction(uint(id), body.Action, body.Note); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "fraud log updated"})
}

// POST /api/admin/users/:id/lock
func (h *FraudHandler) LockAccount(c *gin.Context) {
	uid, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid user id")
		return
	}
	var body struct {
		Reason string `json:"reason" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := h.fraudSvc.LockAccount(uint(uid), body.Reason); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "account locked"})
}

// POST /api/admin/users/:id/unlock
func (h *FraudHandler) UnlockAccount(c *gin.Context) {
	uid, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid user id")
		return
	}
	if err := h.fraudSvc.UnlockAccount(uint(uid)); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "account unlocked"})
}
