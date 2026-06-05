package domain

import "time"

// AuditLog records security-sensitive actions: login (success/fail), password
// change, 2FA enable/disable, withdrawal request, API key create/revoke, etc.
//
// Append-only by convention — never UPDATE rows, never DELETE except for
// retention pruning. Indexed by user_id + created_at for fast per-user history.
type AuditLog struct {
	ID         uint      `json:"id"`
	UserID     uint      `json:"userId"` // 0 if pre-auth (login fail w/ unknown email)
	Email      string    `json:"email,omitempty"`                          // captured even for pre-auth events
	Action     string    `json:"action"`             // see AuditAction* constants
	Outcome    string    `json:"outcome"`                  // success | failure
	IP         string    `json:"ip,omitempty"`
	UserAgent  string    `json:"userAgent,omitempty"`
	DeviceID   string    `json:"deviceId,omitempty"`  // 16-hex stable fingerprint
	NewDevice  bool      `json:"newDevice,omitempty"`                                     // first-time seen for this user
	Detail     string    `json:"detail,omitempty"`        // freeform JSON / short message
	CreatedAt  time.Time `json:"createdAt"`
}

// Audit action constants — keep stable; consumers rely on string equality.
const (
	AuditLoginSuccess    = "login.success"
	AuditLoginFailure    = "login.failure"
	AuditLogout          = "logout"
	AuditRegister        = "user.register"
	AuditTokenRefresh    = "token.refresh"
	AuditTokenReplay     = "token.replay_detected"
	AuditPasswordChange  = "password.change"
	Audit2FAEnable       = "2fa.enable"
	Audit2FADisable      = "2fa.disable"
	AuditAPIKeyCreate    = "apikey.create"
	AuditAPIKeyRevoke    = "apikey.revoke"
	AuditWithdrawCreate  = "withdraw.create"
	AuditWithdrawApprove = "withdraw.approve"
	AuditWithdrawReject  = "withdraw.reject"
	AuditAccountLock     = "account.lock"
	AuditAccountUnlock   = "account.unlock"
)

const (
	AuditOutcomeSuccess = "success"
	AuditOutcomeFailure = "failure"
)
