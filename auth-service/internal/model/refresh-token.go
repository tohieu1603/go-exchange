package model

import "time"

// RefreshToken stores hashed refresh tokens with rotation chain (token family) tracking.
// Family = chain of tokens issued from one login. If a stolen token is reused after
// rotation, the entire family is revoked (replay/theft detection).
type RefreshToken struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	UserID        uint       `gorm:"not null;index:idx_rt_user" json:"userId"`
	TokenHash     string     `gorm:"uniqueIndex;not null" json:"-"` // SHA-256 of raw token
	FamilyID      string     `gorm:"not null;index:idx_rt_family" json:"familyId"` // UUID of token family
	ParentID      *uint      `json:"parentId,omitempty"`            // Previous token in family (null = root)
	UserAgent     string     `json:"userAgent"`
	IP            string     `json:"ip"`
	IssuedAt      time.Time  `gorm:"not null" json:"issuedAt"`
	ExpiresAt     time.Time  `gorm:"not null;index" json:"expiresAt"`
	UsedAt        *time.Time `json:"usedAt,omitempty"`             // Marked when rotated
	RevokedAt     *time.Time `json:"revokedAt,omitempty"`
	RevokedReason string     `json:"revokedReason,omitempty"`       // logout, rotated, replay_detected, password_change, admin
}

// RevokeReason enum constants
const (
	RevokeReasonLogout         = "logout"
	RevokeReasonRotated        = "rotated"
	RevokeReasonReplayDetected = "replay_detected"
	RevokeReasonPasswordChange = "password_change"
	RevokeReasonAdmin          = "admin"
	RevokeReasonExpired        = "expired"
)
