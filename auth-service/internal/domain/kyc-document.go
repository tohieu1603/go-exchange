package domain

import "time"

// KYCDocument stores uploaded identity documents (Step 3 of KYC)
type KYCDocument struct {
	ID        uint      `json:"id"`
	UserID    uint      `json:"userId"`
	DocType   string    `json:"docType"`            // CCCD_FRONT, CCCD_BACK, SELFIE
	FilePath  string    `json:"filePath"`           // local path or S3 URL
	Status    string    `json:"status"`      // PENDING, APPROVED, REJECTED
	AdminNote string    `json:"adminNote,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}
