// Package http is the inbound HTTP (gin) adapter for the wallet service. It does
// only transport concerns: decode requests, invoke application use cases, map
// domain errors to status codes, and present domain objects as the stable JSON
// contract via the DTOs below (domain entities stay free of json tags).
package http

import (
	"errors"
	"net/http"

	"github.com/cryptox/shared/response"
	"github.com/cryptox/wallet-service/internal/domain"
	"github.com/gin-gonic/gin"
)

type walletDTO struct {
	UserID        uint    `json:"userId"`
	Currency      string  `json:"currency"`
	Balance       float64 `json:"balance"`
	LockedBalance float64 `json:"lockedBalance"`
	Available     float64 `json:"available"`
}

func toWalletDTO(w domain.Wallet) walletDTO {
	return walletDTO{UserID: w.UserID, Currency: w.Currency, Balance: w.Balance, LockedBalance: w.Locked, Available: w.Available()}
}

func toWalletDTOs(ws []domain.Wallet) []walletDTO {
	out := make([]walletDTO, len(ws))
	for i, w := range ws {
		out[i] = toWalletDTO(w)
	}
	return out
}

type depositDTO struct {
	ID           uint    `json:"id"`
	UserID       uint    `json:"userId"`
	Amount       float64 `json:"amount"`
	AmountUSDT   float64 `json:"amountUsdt"`
	ExchangeRate float64 `json:"exchangeRate"`
	Currency     string  `json:"currency"`
	Method       string  `json:"method"`
	Status       string  `json:"status"`
	OrderCode    string  `json:"orderCode"`
	QRCodeURL    string  `json:"qrCodeUrl"`
	SepayRef     string  `json:"sepayRef"`
	CreatedAt    string  `json:"createdAt"`
}

func toDepositDTO(d domain.Deposit) depositDTO {
	return depositDTO{
		ID: d.ID, UserID: d.UserID, Amount: d.Amount, AmountUSDT: d.AmountUSDT,
		ExchangeRate: d.ExchangeRate, Currency: d.Currency, Method: d.Method,
		Status: string(d.Status), OrderCode: d.OrderCode, QRCodeURL: d.QRCodeURL,
		SepayRef: d.SepayRef, CreatedAt: d.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func toDepositDTOs(ds []domain.Deposit) []depositDTO {
	out := make([]depositDTO, len(ds))
	for i, d := range ds {
		out[i] = toDepositDTO(d)
	}
	return out
}

type withdrawalDTO struct {
	ID          uint    `json:"id"`
	UserID      uint    `json:"userId"`
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	BankCode    string  `json:"bankCode"`
	BankAccount string  `json:"bankAccount"`
	AccountName string  `json:"accountName"`
	Status      string  `json:"status"`
	AdminNote   string  `json:"adminNote"`
	CreatedAt   string  `json:"createdAt"`
}

func toWithdrawalDTO(w domain.Withdrawal) withdrawalDTO {
	return withdrawalDTO{
		ID: w.ID, UserID: w.UserID, Amount: w.Amount, Currency: w.Currency,
		BankCode: w.BankCode, BankAccount: w.BankAccount, AccountName: w.AccountName,
		Status: string(w.Status), AdminNote: w.AdminNote,
		CreatedAt: w.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func toWithdrawalDTOs(ws []domain.Withdrawal) []withdrawalDTO {
	out := make([]withdrawalDTO, len(ws))
	for i, w := range ws {
		out[i] = toWithdrawalDTO(w)
	}
	return out
}

// statusFor maps a domain error to an HTTP status code.
func statusFor(err error) int {
	switch {
	case errors.Is(err, domain.ErrWalletNotFound),
		errors.Is(err, domain.ErrDepositNotFound),
		errors.Is(err, domain.ErrWithdrawalNotFound):
		return http.StatusNotFound
	default:
		// Business-rule violations (insufficient balance, locked account, bad
		// amount, invalid state transition) are client errors.
		return http.StatusBadRequest
	}
}

// fail writes a domain error as the appropriate status + message. It routes
// through response.FailClassified so an unexpected (apperr.Internal / 5xx) error
// is reduced to a generic "internal error" instead of leaking its detail.
func fail(c *gin.Context, err error) {
	response.FailClassified(c, err, statusFor)
}
