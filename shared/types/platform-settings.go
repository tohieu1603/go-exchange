package types

// PlatformSettings is admin-configurable platform-wide config.
// Auth-service is the system of record (table + admin endpoints);
// other services read it from Redis key "platform_settings" (JSON).
//
// Lives in shared/types because it crosses service boundaries as a Redis-cached DTO.
type PlatformSettings struct {
	ID                 uint    `gorm:"primaryKey" json:"id"`
	DepositFeePercent  float64 `gorm:"type:decimal(5,2);default:2.0" json:"depositFeePercent"`
	WithdrawFeePercent float64 `gorm:"type:decimal(5,2);default:0" json:"withdrawFeePercent"`
	MinDeposit         float64 `gorm:"type:decimal(20,2);default:100000" json:"minDeposit"`
	MaxDeposit         float64 `gorm:"type:decimal(20,2);default:500000000" json:"maxDeposit"`
	MinWithdraw        float64 `gorm:"type:decimal(20,2);default:200000" json:"minWithdraw"`
	MaxWithdraw        float64 `gorm:"type:decimal(20,2);default:100000000" json:"maxWithdraw"`
	TradingFeePercent  float64 `gorm:"type:decimal(5,4);default:0.1" json:"tradingFeePercent"`
	KYCRequired        bool    `gorm:"default:true" json:"kycRequired"`
}

// DefaultPlatformSettings returns conservative defaults used as fallback
// when Redis lookup fails (e.g. cold start).
func DefaultPlatformSettings() PlatformSettings {
	return PlatformSettings{
		ID: 1, DepositFeePercent: 2.0,
		MinDeposit: 100000, MaxDeposit: 500000000,
		MinWithdraw: 200000, MaxWithdraw: 100000000,
		TradingFeePercent: 0.1, KYCRequired: true,
	}
}
