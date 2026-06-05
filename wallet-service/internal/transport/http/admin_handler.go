package http

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/response"
	"github.com/cryptox/wallet-service/internal/usecase"
	"github.com/gin-gonic/gin"
)

// adjustMaxAmount caps a single admin balance adjustment (default 100k, override
// via ADMIN_ADJUST_MAX_AMOUNT; <=0 disables the cap).
func adjustMaxAmount() float64 {
	v, _ := strconv.ParseFloat(os.Getenv("ADMIN_ADJUST_MAX_AMOUNT"), 64)
	if v == 0 {
		return 100000
	}
	return v
}

// AdminWalletHandler exposes the admin wallet endpoints.
type AdminWalletHandler struct {
	uc  *usecase.WalletUseCase
	bus usecase.EventPublisher
}

func NewAdminWalletHandler(uc *usecase.WalletUseCase, bus usecase.EventPublisher) *AdminWalletHandler {
	return &AdminWalletHandler{uc: uc, bus: bus}
}

func (h *AdminWalletHandler) publishAudit(c *gin.Context, subjectUserID uint, action, outcome, detail string) {
	if h.bus == nil {
		return
	}
	adminID := middleware.GetUserID(c)
	_ = h.bus.Publish(c.Request.Context(), eventbus.TopicAuditRequest, eventbus.AuditRequestEvent{
		UserID:  subjectUserID,
		Action:  action,
		Outcome: outcome,
		IP:      c.ClientIP(),
		Detail:  fmt.Sprintf("admin=%d %s", adminID, detail),
	})
}

// AdjustBalance applies a signed delta to a user's balance (audited, capped).
// POST /api/admin/users/:id/wallets/:currency/adjust
func (h *AdminWalletHandler) AdjustBalance(c *gin.Context) {
	uid64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid user id")
		return
	}
	userID := uint(uid64)
	currency := strings.ToUpper(c.Param("currency"))
	if currency == "" {
		response.Error(c, 400, "currency required")
		return
	}
	var body struct {
		Amount float64 `json:"amount"`
		Reason string  `json:"reason"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if body.Amount == 0 {
		response.Error(c, 400, "amount cannot be zero")
		return
	}
	if strings.TrimSpace(body.Reason) == "" {
		response.Error(c, 400, "reason is required")
		return
	}
	if cap := adjustMaxAmount(); cap > 0 && math.Abs(body.Amount) > cap {
		h.publishAudit(c, userID, "admin.balance.adjust", "failure",
			fmt.Sprintf("currency=%s amount=%.4f reason=%q err=exceeds-cap(%.0f)", currency, body.Amount, body.Reason, cap))
		response.Error(c, 400, fmt.Sprintf("amount exceeds cap (%.0f)", cap))
		return
	}
	if err := h.uc.AdjustBalance(c.Request.Context(), userID, currency, body.Amount); err != nil {
		h.publishAudit(c, userID, "admin.balance.adjust", "failure",
			fmt.Sprintf("currency=%s amount=%.4f reason=%q err=%s", currency, body.Amount, body.Reason, err.Error()))
		fail(c, err)
		return
	}
	h.publishAudit(c, userID, "admin.balance.adjust", "success",
		fmt.Sprintf("currency=%s amount=%+.4f reason=%q", currency, body.Amount, body.Reason))
	response.OK(c, gin.H{"message": "balance adjusted"})
}

// AdjustBalanceBatch applies up to 20 independent adjustments.
// POST /api/admin/users/:id/wallets/adjust-batch
func (h *AdminWalletHandler) AdjustBalanceBatch(c *gin.Context) {
	uid64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid user id")
		return
	}
	userID := uint(uid64)
	var body struct {
		Items []struct {
			Currency string  `json:"currency"`
			Amount   float64 `json:"amount"`
			Reason   string  `json:"reason"`
		} `json:"items"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if len(body.Items) == 0 {
		response.Error(c, 400, "items required")
		return
	}
	if len(body.Items) > 20 {
		response.Error(c, 400, "max 20 items per batch")
		return
	}

	maxAmt := adjustMaxAmount()
	results := make([]gin.H, 0, len(body.Items))
	successCount := 0
	for i, item := range body.Items {
		currency := strings.ToUpper(strings.TrimSpace(item.Currency))
		reason := strings.TrimSpace(item.Reason)
		row := gin.H{"index": i, "currency": currency, "amount": item.Amount}

		var rowErr string
		switch {
		case currency == "":
			rowErr = "currency required"
		case item.Amount == 0:
			rowErr = "amount cannot be zero"
		case reason == "":
			rowErr = "reason required"
		case maxAmt > 0 && math.Abs(item.Amount) > maxAmt:
			rowErr = fmt.Sprintf("exceeds cap (%.0f)", maxAmt)
		default:
			if err := h.uc.AdjustBalance(c.Request.Context(), userID, currency, item.Amount); err != nil {
				rowErr = err.Error()
			}
		}

		if rowErr != "" {
			row["error"] = rowErr
			h.publishAudit(c, userID, "admin.balance.adjust", "failure",
				fmt.Sprintf("currency=%s amount=%.4f reason=%q err=%s (batch)", currency, item.Amount, reason, rowErr))
		} else {
			row["ok"] = true
			successCount++
			h.publishAudit(c, userID, "admin.balance.adjust", "success",
				fmt.Sprintf("currency=%s amount=%+.4f reason=%q (batch)", currency, item.Amount, reason))
		}
		results = append(results, row)
	}
	response.OK(c, gin.H{"applied": successCount, "total": len(body.Items), "results": results})
}

// GET /api/admin/users/:id/wallets
func (h *AdminWalletHandler) UserWallets(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid user id")
		return
	}
	wallets, err := h.uc.GetBalances(c.Request.Context(), uint(id))
	if err != nil {
		response.Error(c, 500, "failed to load wallets")
		return
	}
	response.OK(c, toWalletDTOs(wallets))
}

// GET /api/admin/deposits
func (h *AdminWalletHandler) Deposits(c *gin.Context) {
	page, size := pageParams(c)
	deposits, total, err := h.uc.AdminListDeposits(c.Request.Context(), page, size, c.Query("search"), c.Query("status"))
	if err != nil {
		response.Error(c, 500, "failed to load deposits")
		return
	}
	response.Page(c, toDepositDTOs(deposits), total, page, size)
}

// POST /api/admin/deposits/:id/confirm
func (h *AdminWalletHandler) ConfirmDeposit(c *gin.Context) {
	var body struct {
		OrderCode string `json:"orderCode" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := h.uc.ConfirmDeposit(c.Request.Context(), body.OrderCode); err != nil {
		fail(c, err)
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
	if err := h.uc.AdminRejectDeposit(c.Request.Context(), uint(id)); err != nil {
		fail(c, err)
		return
	}
	response.OK(c, gin.H{"message": "deposit rejected"})
}

// GET /api/admin/withdrawals
func (h *AdminWalletHandler) Withdrawals(c *gin.Context) {
	page, size := pageParams(c)
	ws, total, err := h.uc.AdminListWithdrawals(c.Request.Context(), page, size, c.Query("search"), c.Query("status"))
	if err != nil {
		response.Error(c, 500, "failed to load withdrawals")
		return
	}
	response.Page(c, toWithdrawalDTOs(ws), total, page, size)
}

// POST /api/admin/withdrawals/:id/approve
func (h *AdminWalletHandler) ApproveWithdrawal(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid withdrawal id")
		return
	}
	if err := h.uc.AdminApproveWithdrawal(c.Request.Context(), uint(id)); err != nil {
		fail(c, err)
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
	if err := h.uc.AdminRejectWithdrawal(c.Request.Context(), uint(id)); err != nil {
		fail(c, err)
		return
	}
	response.OK(c, gin.H{"message": "withdrawal rejected"})
}
