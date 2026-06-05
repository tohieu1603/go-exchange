package domain

import "time"

// WithdrawalStatus is the lifecycle state of a USDT→VND bank withdrawal.
type WithdrawalStatus string

const (
	WithdrawalPending  WithdrawalStatus = "PENDING"
	WithdrawalApproved WithdrawalStatus = "APPROVED"
	WithdrawalRejected WithdrawalStatus = "REJECTED"
)

// Withdrawal is the aggregate for an outbound payout. On creation the user's
// USDT is locked; approval releases the payout, rejection unlocks the funds.
type Withdrawal struct {
	ID          uint
	UserID      uint
	Amount      float64 // VND payout amount
	Currency    string
	BankCode    string
	BankAccount string
	AccountName string
	Status      WithdrawalStatus
	AdminNote   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NewWithdrawal builds a PENDING withdrawal request.
func NewWithdrawal(userID uint, vndAmount float64, bankCode, bankAccount, accountName string) (*Withdrawal, error) {
	if err := requirePositive(vndAmount); err != nil {
		return nil, err
	}
	if bankCode == "" || bankAccount == "" || accountName == "" {
		return nil, ErrInvalidAmount
	}
	return &Withdrawal{
		UserID:      userID,
		Amount:      vndAmount,
		Currency:    "VND",
		BankCode:    bankCode,
		BankAccount: bankAccount,
		AccountName: accountName,
		Status:      WithdrawalPending,
	}, nil
}

// Approve transitions PENDING → APPROVED (funds paid out, lock consumed).
func (w *Withdrawal) Approve() error {
	if w.Status != WithdrawalPending {
		return ErrWithdrawalNotPending
	}
	w.Status = WithdrawalApproved
	return nil
}

// Reject transitions PENDING → REJECTED; the caller must unlock the reserved
// balance in the same transaction.
func (w *Withdrawal) Reject() error {
	if w.Status != WithdrawalPending {
		return ErrWithdrawalNotPending
	}
	w.Status = WithdrawalRejected
	return nil
}
