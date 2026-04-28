package model

import "time"

type User struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	Email            string    `gorm:"uniqueIndex;not null" json:"email"`
	PasswordHash     string    `gorm:"not null" json:"-"`
	FullName         string    `json:"fullName"`
	Phone            string    `json:"phone"`
	KYCStatus        string    `gorm:"default:NONE" json:"kycStatus"` // NONE, PENDING, VERIFIED, REJECTED
	Is2FA            bool      `gorm:"column:is2_fa;default:false" json:"is2FA"`
	TwoFASecret      string    `gorm:"" json:"-"` // TOTP secret, never exposed in API
	Role             string    `gorm:"default:USER" json:"role"` // USER, ADMIN
	EmailVerified    bool      `gorm:"default:false" json:"emailVerified"`
	EmailVerifyToken string    `gorm:"-" json:"-"` // not persisted, used in-memory only
	KYCStep          int       `gorm:"default:0" json:"kycStep"` // 0=none, 1=email_verified, 2=profile_done, 3=docs_uploaded, 4=approved
	IsLocked         bool      `gorm:"default:false" json:"isLocked"`
	LockReason       string    `json:"lockReason,omitempty"`
	LastLoginIP      string    `json:"lastLoginIp,omitempty"`
	RegisterIP       string    `json:"registerIp,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}
