package domain

import "time"

// HasPassword is a derived flag (PasswordHash != "") populated by the postgres
// adapter's mapper, letting the FE pick "Set Password" vs "Change Password"
// without exposing the hash. It used to be a gorm AfterFind hook; with pgx the
// repository sets it on read.

type User struct {
	ID               uint      `json:"id"`
	Email            string    `json:"email"`
	PasswordHash     string    `json:"-"`
	FullName         string    `json:"fullName"`
	Phone            string    `json:"phone"`
	KYCStatus        string    `json:"kycStatus"` // NONE, PENDING, VERIFIED, REJECTED
	Is2FA            bool      `json:"is2FA"`
	TwoFASecret      string    `json:"-"` // TOTP secret, never exposed in API
	Role             string    `json:"role"` // USER, ADMIN
	EmailVerified    bool      `json:"emailVerified"`
	EmailVerifyToken string    `json:"-"` // not persisted, used in-memory only
	KYCStep          int       `json:"kycStep"` // 0=none, 1=email_verified, 2=profile_done, 3=docs_uploaded, 4=approved
	IsLocked         bool      `json:"isLocked"`
	LockReason       string    `json:"lockReason,omitempty"`
	LastLoginIP      string    `json:"lastLoginIp,omitempty"`
	RegisterIP       string    `json:"registerIp,omitempty"`
	// Google OAuth — `sub` is the immutable Google account id (preferred over
	// email since users can change Gmail addresses on Workspace domains).
	// Empty for password-only users.
	GoogleSub string `json:"-"`
	AvatarURL string `json:"avatarUrl,omitempty"`
	// Derived (NOT persisted) — populated by AfterFind. Lets the FE pick
	// "Set Password" vs "Change Password" without exposing the hash.
	HasPassword bool `json:"hasPassword"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}
