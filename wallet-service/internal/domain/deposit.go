package domain

import "time"

// DepositStatus is the lifecycle state of a fiat→USDT deposit.
type DepositStatus string

const (
	DepositPending   DepositStatus = "PENDING"
	DepositConfirmed DepositStatus = "CONFIRMED"
	DepositFailed    DepositStatus = "FAILED"
)

// Deposit is the aggregate for an inbound VND bank transfer that credits USDT
// once the bank webhook (or an admin) confirms receipt.
type Deposit struct {
	ID           uint
	UserID       uint
	Amount       float64 // VND tendered
	AmountUSDT   float64 // USDT to credit on confirmation
	ExchangeRate float64 // VND per USDT at creation time
	Currency     string  // funding currency, always VND today
	Method       string
	Status       DepositStatus
	OrderCode    string // unique idempotency key, surfaced in the bank memo
	QRCodeURL    string
	SepayRef     string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// NewDeposit builds a PENDING deposit. The use case is responsible for
// computing amountUSDT (rate, fees) and generating orderCode/qrURL.
func NewDeposit(userID uint, amountVND, amountUSDT, rate float64, orderCode, qrURL string) (*Deposit, error) {
	if err := requirePositive(amountVND); err != nil {
		return nil, err
	}
	return &Deposit{
		UserID:       userID,
		Amount:       amountVND,
		AmountUSDT:   amountUSDT,
		ExchangeRate: rate,
		Currency:     "VND",
		Method:       "BANK_TRANSFER",
		Status:       DepositPending,
		OrderCode:    orderCode,
		QRCodeURL:    qrURL,
	}, nil
}

// Confirm transitions PENDING → CONFIRMED. It is the only path that authorises
// crediting the user's USDT balance.
func (d *Deposit) Confirm() error {
	if d.Status != DepositPending {
		return ErrDepositNotPending
	}
	d.Status = DepositConfirmed
	return nil
}

// Fail transitions PENDING → FAILED (admin rejection / timeout).
func (d *Deposit) Fail() error {
	if d.Status != DepositPending {
		return ErrDepositNotPending
	}
	d.Status = DepositFailed
	return nil
}
