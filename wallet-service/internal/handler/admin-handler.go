package handler

import (
	"strconv"

	"github.com/cryptox/shared/response"
	"github.com/cryptox/wallet-service/internal/service"
	"github.com/gin-gonic/gin"
)

// AdminWalletHandler handles admin wallet endpoints.
type AdminWalletHandler struct {
	wallet *service.WalletService
}

func NewAdminWalletHandler(wallet *service.WalletService) *AdminWalletHandler {
	return &AdminWalletHandler{wallet: wallet}
}

func adminPageParams(c *gin.Context) (int, int) {
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

// GET /api/admin/users/:id/wallets
func (h *AdminWalletHandler) UserWallets(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid user id")
		return
	}
	wallets, err := h.wallet.GetBalances(uint(id))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, wallets)
}

// GET /api/admin/deposits?page=&size=&search=&status=
func (h *AdminWalletHandler) Deposits(c *gin.Context) {
	page, size := adminPageParams(c)
	search := c.Query("search")
	status := c.Query("status")

	deposits, total, err := h.wallet.AdminListDeposits(page, size, search, status)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Page(c, deposits, total, page, size)
}

// POST /api/admin/deposits/:id/confirm
func (h *AdminWalletHandler) ConfirmDeposit(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid deposit id")
		return
	}
	var body struct {
		OrderCode string `json:"orderCode" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	_ = id
	if err := h.wallet.ConfirmDeposit(body.OrderCode); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "deposit confirmed"})
}

// POST /api/admin/deposits/:id/reject
func (h *AdminWalletHandler) RejectDeposit(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid deposit id")
		return
	}
	if err := h.wallet.AdminRejectDeposit(uint(id)); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "deposit rejected"})
}

// GET /api/admin/withdrawals?page=&size=&search=&status=
func (h *AdminWalletHandler) Withdrawals(c *gin.Context) {
	page, size := adminPageParams(c)
	search := c.Query("search")
	status := c.Query("status")

	withdrawals, total, err := h.wallet.AdminListWithdrawals(page, size, search, status)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Page(c, withdrawals, total, page, size)
}

// POST /api/admin/withdrawals/:id/approve
func (h *AdminWalletHandler) ApproveWithdrawal(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid withdrawal id")
		return
	}
	if err := h.wallet.AdminApproveWithdrawal(uint(id)); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "withdrawal approved"})
}

// POST /api/admin/withdrawals/:id/reject
func (h *AdminWalletHandler) RejectWithdrawal(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid withdrawal id")
		return
	}
	if err := h.wallet.AdminRejectWithdrawal(uint(id)); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "withdrawal rejected"})
}
