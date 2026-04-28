package types

// Order side / type / status — string enums shared across producer/consumer.
const (
	OrderSideBuy  = "BUY"
	OrderSideSell = "SELL"

	OrderTypeMarket    = "MARKET"
	OrderTypeLimit     = "LIMIT"
	OrderTypeStopLimit = "STOP_LIMIT"

	OrderStatusOpen      = "OPEN"
	OrderStatusPartial   = "PARTIAL"
	OrderStatusFilled    = "FILLED"
	OrderStatusCancelled = "CANCELLED"
)

// Position side / status (perpetual futures).
const (
	PositionSideLong  = "LONG"
	PositionSideShort = "SHORT"

	PositionStatusOpen        = "OPEN"
	PositionStatusClosed      = "CLOSED"
	PositionStatusLiquidated  = "LIQUIDATED"
)

// KYC status & step.
const (
	KYCStatusNone     = "NONE"
	KYCStatusPending  = "PENDING"
	KYCStatusVerified = "VERIFIED"
	KYCStatusRejected = "REJECTED"

	KYCStepNone           = 0
	KYCStepEmailVerified  = 1
	KYCStepProfileDone    = 2
	KYCStepDocsUploaded   = 3
	KYCStepApproved       = 4
)

// User roles.
const (
	RoleUser   = "USER"
	RoleAdmin  = "ADMIN"
	RoleSystem = "SYSTEM" // platform-internal accounts (fee wallet, etc.)
)

// Withdrawal status.
const (
	WithdrawStatusPending  = "PENDING"
	WithdrawStatusApproved = "APPROVED"
	WithdrawStatusRejected = "REJECTED"
)

// Deposit status.
const (
	DepositStatusPending   = "PENDING"
	DepositStatusConfirmed = "CONFIRMED"
	DepositStatusFailed    = "FAILED"
)

// Notification types — keep loose; consumers can add more.
const (
	NotifTypeOrderFilled         = "ORDER_FILLED"
	NotifTypePositionOpened      = "POSITION_OPENED"
	NotifTypePositionClosed      = "POSITION_CLOSED"
	NotifTypePositionLiquidated  = "POSITION_LIQUIDATED"
	NotifTypeMarginCall          = "MARGIN_CALL"
	NotifTypeDepositConfirmed    = "DEPOSIT_CONFIRMED"
	NotifTypeWithdrawalApproved  = "WITHDRAWAL_APPROVED"
)
