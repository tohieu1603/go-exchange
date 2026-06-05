package domain

import "time"

// KYCProfile stores user's identity verification data (Step 2 of KYC)
type KYCProfile struct {
	ID          uint      `json:"id"`
	UserID      uint      `json:"userId"`
	FirstName   string    `json:"firstName"`
	LastName    string    `json:"lastName"`
	DateOfBirth string    `json:"dateOfBirth"`  // YYYY-MM-DD
	Phone       string    `json:"phone"`
	Address     string    `json:"address"`      // Street/house number
	Ward        string    `json:"ward"`         // Phuong
	District    string    `json:"district"`     // Quan/Huyen
	City        string    `json:"city"`         // Thanh pho
	PostalCode  string    `json:"postalCode"`   // nullable
	Country     string    `json:"country"`
	// Survey fields
	Occupation string    `json:"occupation"`  // Job title
	Income     string    `json:"income"`      // Income range: <10M, 10-30M, 30-50M, >50M VND
	TradingExp string    `json:"tradingExp"`  // none, <1year, 1-3years, >3years
	Purpose    string    `json:"purpose"`     // investment, trading, hedging, learning
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}
