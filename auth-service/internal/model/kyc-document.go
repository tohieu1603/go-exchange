package model

import "time"

// KYCDocument stores uploaded identity documents (Step 3 of KYC)
type KYCDocument struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index;not null" json:"userId"`
	DocType   string    `gorm:"not null" json:"docType"`            // CCCD_FRONT, CCCD_BACK, SELFIE
	FilePath  string    `gorm:"not null" json:"filePath"`           // local path or S3 URL
	Status    string    `gorm:"default:PENDING" json:"status"`      // PENDING, APPROVED, REJECTED
	AdminNote string    `json:"adminNote,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}
