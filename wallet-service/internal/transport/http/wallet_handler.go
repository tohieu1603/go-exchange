package http

import (
	"strconv"

	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/response"
	"github.com/cryptox/wallet-service/internal/usecase"
	"github.com/gin-gonic/gin"
)

// WalletHandler exposes the user-facing wallet endpoints.
type WalletHandler struct {
	uc       *usecase.WalletUseCase
	twoFA    usecase.TwoFAVerifier
}

func NewWalletHandler(uc *usecase.WalletUseCase, twoFA usecase.TwoFAVerifier) *WalletHandler {
	return &WalletHandler{uc: uc, twoFA: twoFA}
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

// Balances godoc
// @Summary  Get wallet balances
// @Tags     wallet
// @Produce  json
// @Security CookieAuth
// @Success  200 {object} map[string]interface{}
// @Router   /wallet/balances [get]
func (h *WalletHandler) Balances(c *gin.Context) {
	wallets, err := h.uc.GetBalances(c.Request.Context(), middleware.GetUserID(c))
	if err != nil {
		response.Error(c, 500, "failed to load balances")
		return
	}
	response.OK(c, toWalletDTOs(wallets))
}

// CreateDeposit godoc
// @Summary  Initiate a VND deposit
// @Tags     wallet
// @Accept   json
// @Produce  json
// @Security CookieAuth
// @Param    body body object{amount=number} true "Deposit amount in VND"
// @Success  201 {object} map[string]interface{}
// @Router   /wallet/deposit [post]
func (h *WalletHandler) CreateDeposit(c *gin.Context) {
	var req struct {
		Amount float64 `json:"amount" binding:"required,gt=0"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	deposit, err := h.uc.CreateDeposit(c.Request.Context(), middleware.GetUserID(c), req.Amount)
	if err != nil {
		fail(c, err)
		return
	}
	response.Created(c, toDepositDTO(*deposit))
}

// Deposits godoc
// @Summary  List deposit history
// @Tags     wallet
// @Produce  json
// @Security CookieAuth
// @Success  200 {object} map[string]interface{}
// @Router   /wallet/deposits [get]
func (h *WalletHandler) Deposits(c *gin.Context) {
	page, size := pageParams(c)
	deposits, total, err := h.uc.GetDepositHistory(c.Request.Context(), middleware.GetUserID(c), page, size)
	if err != nil {
		response.Error(c, 500, "failed to load deposits")
		return
	}
	response.Page(c, toDepositDTOs(deposits), total, page, size)
}

type withdrawReq struct {
	AmountUSDT  float64 `json:"amountUsdt" binding:"required,gt=0"`
	BankCode    string  `json:"bankCode" binding:"required"`
	BankAccount string  `json:"bankAccount" binding:"required"`
	AccountName string  `json:"accountName" binding:"required"`
	TwoFACode   string  `json:"twoFaCode"`
}

// Withdraw godoc
// @Summary  Withdraw USDT to bank (2FA-gated)
// @Tags     wallet
// @Accept   json
// @Produce  json
// @Security CookieAuth
// @Param    body body withdrawReq true "Withdrawal payload"
// @Success  201 {object} map[string]interface{}
// @Router   /wallet/withdraw [post]
func (h *WalletHandler) Withdraw(c *gin.Context) {
	var req withdrawReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	userID := middleware.GetUserID(c)

	if h.twoFA != nil {
		valid, required, err := h.twoFA.Verify2FA(c.Request.Context(), userID, req.TwoFACode)
		if err != nil {
			response.Error(c, 502, "2FA service unavailable")
			return
		}
		if required && !valid {
			response.Error(c, 401, "2FA code required and must be valid")
			return
		}
	}

	withdrawal, err := h.uc.CreateWithdrawal(c.Request.Context(), userID, req.AmountUSDT, req.BankCode, req.BankAccount, req.AccountName)
	if err != nil {
		fail(c, err)
		return
	}
	response.Created(c, toWithdrawalDTO(*withdrawal))
}

// Withdrawals godoc
// @Summary  List withdrawal history
// @Tags     wallet
// @Produce  json
// @Security CookieAuth
// @Success  200 {object} map[string]interface{}
// @Router   /wallet/withdrawals [get]
func (h *WalletHandler) Withdrawals(c *gin.Context) {
	page, size := pageParams(c)
	ws, total, err := h.uc.GetWithdrawalHistory(c.Request.Context(), middleware.GetUserID(c), page, size)
	if err != nil {
		response.Error(c, 500, "failed to load withdrawals")
		return
	}
	response.Page(c, toWithdrawalDTOs(ws), total, page, size)
}
