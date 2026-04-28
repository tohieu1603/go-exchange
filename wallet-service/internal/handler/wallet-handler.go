package handler

import (
	"strconv"

	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/response"
	walletgrpc "github.com/cryptox/wallet-service/internal/grpc"
	"github.com/cryptox/wallet-service/internal/service"
	"github.com/gin-gonic/gin"
)

type WalletHandler struct {
	wallet     *service.WalletService
	authClient *walletgrpc.AuthClient // for cross-service 2FA verification
}

func NewWalletHandler(wallet *service.WalletService, authClient *walletgrpc.AuthClient) *WalletHandler {
	return &WalletHandler{wallet: wallet, authClient: authClient}
}

// Balances godoc
// @Summary      Get wallet balances
// @Tags         wallet
// @Produce      json
// @Security     CookieAuth
// @Success      200  {object}  map[string]interface{}
// @Failure      500  {object}  map[string]interface{}
// @Router       /wallet/balances [get]
func (h *WalletHandler) Balances(c *gin.Context) {
	userID := middleware.GetUserID(c)
	wallets, err := h.wallet.GetBalances(userID)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, wallets)
}

// CreateDeposit godoc
// @Summary      Initiate a VND deposit
// @Tags         wallet
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        body  body  object{amount=number}  true  "Deposit amount in VND"
// @Success      201  {object}  map[string]interface{}
// @Failure      400  {object}  map[string]interface{}
// @Router       /wallet/deposit [post]
func (h *WalletHandler) CreateDeposit(c *gin.Context) {
	var req struct {
		Amount float64 `json:"amount" binding:"required,gt=0"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	userID := middleware.GetUserID(c)
	deposit, err := h.wallet.CreateDeposit(userID, req.Amount)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.Created(c, deposit)
}

// Deposits godoc
// @Summary      List deposit history
// @Tags         wallet
// @Produce      json
// @Security     CookieAuth
// @Param        page  query  int  false  "Page (default 1)"
// @Param        size  query  int  false  "Page size (default 20, max 100)"
// @Success      200  {object}  map[string]interface{}
// @Router       /wallet/deposits [get]
func (h *WalletHandler) Deposits(c *gin.Context) {
	userID := middleware.GetUserID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}

	deposits, total, err := h.wallet.GetDepositHistory(userID, page, size)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Page(c, deposits, total, page, size)
}

type WithdrawReq struct {
	AmountUSDT  float64 `json:"amountUsdt" binding:"required,gt=0"`
	BankCode    string  `json:"bankCode" binding:"required"`
	BankAccount string  `json:"bankAccount" binding:"required"`
	AccountName string  `json:"accountName" binding:"required"`
	TwoFACode   string  `json:"twoFaCode"` // required if user has 2FA enabled
}

// Withdraw godoc
// @Summary      Withdraw USDT to bank
// @Description  Requires 2FA code if enabled. Gated by "withdraw" API-key permission.
// @Tags         wallet
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        body  body  WithdrawReq  true  "Withdrawal payload"
// @Success      201  {object}  map[string]interface{}
// @Failure      400  {object}  map[string]interface{}
// @Failure      401  {object}  map[string]interface{}
// @Router       /wallet/withdraw [post]
func (h *WalletHandler) Withdraw(c *gin.Context) {
	var req WithdrawReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	userID := middleware.GetUserID(c)

	if h.authClient != nil {
		valid, required, err := h.authClient.Verify2FA(c.Request.Context(), userID, req.TwoFACode)
		if err != nil {
			response.Error(c, 502, "2FA service unavailable")
			return
		}
		if required && !valid {
			response.Error(c, 401, "2FA code required and must be valid")
			return
		}
	}

	withdrawal, err := h.wallet.CreateWithdrawal(userID, req.AmountUSDT, req.BankCode, req.BankAccount, req.AccountName)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.Created(c, withdrawal)
}

func (h *WalletHandler) Withdrawals(c *gin.Context) {
	userID := middleware.GetUserID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}

	withdrawals, total, err := h.wallet.GetWithdrawalHistory(userID, page, size)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Page(c, withdrawals, total, page, size)
}
