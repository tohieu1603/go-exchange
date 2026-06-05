package domain

import "time"

// APIKey allows users to authenticate algorithmic trading bots via HMAC signature.
// Secret is shown ONCE at creation, only the hash is stored.
type APIKey struct {
	ID          uint       `json:"id"`
	UserID      uint       `json:"userId"`
	Label       string     `json:"label"`            // user-friendly name
	KeyID       string     `json:"keyId"` // public, sent in header X-API-Key
	SecretHash  string     `json:"-"`                 // SHA-256 of secret
	Permissions string     `json:"permissions"` // CSV: read, trade, withdraw
	IPWhitelist string     `json:"ipWhitelist"`                        // CSV of allowed IPs/CIDR; empty = any
	LastUsedAt  *time.Time `json:"lastUsedAt,omitempty"`
	LastUsedIP  string     `json:"lastUsedIp,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`                // optional expiry
	RevokedAt   *time.Time `json:"revokedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// APIKey permission constants
const (
	APIPermRead     = "read"
	APIPermTrade    = "trade"
	APIPermWithdraw = "withdraw"
)
