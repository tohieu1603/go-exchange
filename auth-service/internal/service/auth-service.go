package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/cryptox/auth-service/internal/model"
	"github.com/cryptox/auth-service/internal/repository"
	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/shared/mailer"
	"github.com/cryptox/shared/metrics"
	"github.com/cryptox/shared/types"
	"github.com/cryptox/shared/utils"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// AuthService is the only place User rows are mutated. It NEVER touches
// wallets, orders, etc. — those services maintain their own state via the
// user.registered event and gRPC.
type AuthService struct {
	db       *gorm.DB
	userRepo repository.UserRepo
	rtSvc    *RefreshTokenService
	audit    *AuditLogService
	throttle *LoginThrottle
	stepUp   *StepUpService
	anomaly  *AnomalyDetector
	mail     mailer.Mailer
	secret   string
	rdb      *redis.Client
	totp     *TOTPService
	bus      eventbus.EventPublisher
}

// SetAnomalyDetector wires the detector after construction (avoids ctor explosion).
func (s *AuthService) SetAnomalyDetector(a *AnomalyDetector) { s.anomaly = a }

func NewAuthService(
	userRepo repository.UserRepo,
	rtSvc *RefreshTokenService,
	audit *AuditLogService,
	throttle *LoginThrottle,
	stepUp *StepUpService,
	mail mailer.Mailer,
	secret string,
	rdb *redis.Client,
	bus eventbus.EventPublisher,
) *AuthService {
	if mail == nil {
		mail = mailer.NoopMailer{}
	}
	return &AuthService{
		userRepo: userRepo,
		rtSvc:    rtSvc,
		audit:    audit,
		throttle: throttle,
		stepUp:   stepUp,
		mail:     mail,
		secret:   secret,
		rdb:      rdb,
		totp:     NewTOTPService(),
		bus:      bus,
	}
}

func (s *AuthService) SetDB(db *gorm.DB) { s.db = db }

type RegisterReq struct {
	Email        string `json:"email" binding:"required,email"`
	Password     string `json:"password" binding:"required,min=6"`
	FullName     string `json:"fullName" binding:"required"`
	ReferralCode string `json:"referralCode"`
	ClientIP     string `json:"-"`
	UserAgent    string `json:"-"`
}

type LoginReq struct {
	Email          string `json:"email" binding:"required"`
	Password       string `json:"password" binding:"required"`
	ClientIP       string `json:"-"`
	UserAgent      string `json:"-"`
	AcceptLanguage string `json:"-"` // device-fingerprint input
}

type LoginResult struct {
	AccessToken  string      `json:"-"` // never serialized — cookie only
	RefreshToken string      `json:"-"`
	User         *model.User `json:"user,omitempty"`
	Requires2FA  bool        `json:"requires2FA,omitempty"`
	TempToken    string      `json:"tempToken,omitempty"`

	// Step-up challenge fields (new-device email-OTP flow when 2FA NOT enabled).
	RequiresStepUp bool   `json:"requiresStepUp,omitempty"`
	StepUpToken    string `json:"stepUpToken,omitempty"`
}

func (s *AuthService) Register(req RegisterReq) (*model.User, string, string, error) {
	existing, err := s.userRepo.FindByEmail(req.Email)
	if err == nil && existing != nil && existing.ID != 0 {
		return nil, "", "", errors.New("email already registered")
	}

	// Password policy: HIBP breach check (fail-open by default,
	// fail-closed when HIBP_FAIL_CLOSED=true env is set).
	if err := utils.CheckPasswordPolicy(req.Password); err != nil {
		return nil, "", "", err
	}

	hash, err := utils.HashPassword(req.Password)
	if err != nil {
		return nil, "", "", err
	}

	user := &model.User{
		Email: req.Email, PasswordHash: hash,
		FullName: req.FullName, Role: "USER", KYCStatus: "NONE",
		RegisterIP: req.ClientIP,
	}
	if err := s.userRepo.Create(nil, user); err != nil {
		return nil, "", "", err
	}

	// Publish event for wallet-service to create wallets in its own DB
	s.bus.Publish(context.Background(), eventbus.TopicUserRegistered, eventbus.UserRegisteredEvent{
		UserID: user.ID, Email: user.Email, FullName: user.FullName,
		ReferralCode: req.ReferralCode, ClientIP: req.ClientIP,
	})

	// Record register with device fingerprint so the very next login from the
	// same device is not flagged as "new device" by the step-up gate.
	regDeviceID := utils.DeviceFingerprint(req.ClientIP, req.UserAgent, "")
	s.audit.SuccessDevice(user.ID, user.Email, model.AuditRegister, req.ClientIP, req.UserAgent, regDeviceID, "")

	access, refresh, err := s.issueTokenPair(user, req.UserAgent, req.ClientIP)
	if err != nil {
		return nil, "", "", err
	}
	return user, access, refresh, nil
}

func (s *AuthService) Login(req LoginReq) (*LoginResult, error) {
	ctx := context.Background()

	// Lockout check (account OR IP) first — saves bcrypt/argon CPU under attack.
	if ttl := s.throttle.CheckLocked(ctx, req.Email, req.ClientIP); ttl > 0 {
		s.audit.Failure(0, req.Email, model.AuditLoginFailure, req.ClientIP, req.UserAgent,
			fmt.Sprintf("locked %s remaining", ttl.Round(time.Second)))
		return nil, fmt.Errorf("account temporarily locked, try again in %s", ttl.Round(time.Second))
	}

	user, err := s.userRepo.FindByEmail(req.Email)
	if err != nil || user == nil || user.ID == 0 {
		s.throttle.RecordFailure(ctx, req.Email, req.ClientIP)
		s.audit.Failure(0, req.Email, model.AuditLoginFailure, req.ClientIP, req.UserAgent, "user not found")
		return nil, errors.New("invalid credentials")
	}
	// SYSTEM accounts cannot log in. Reject before bcrypt to leak no signal.
	if user.Role == types.RoleSystem {
		s.audit.Failure(user.ID, req.Email, model.AuditLoginFailure, req.ClientIP, req.UserAgent, "system account")
		return nil, errors.New("invalid credentials")
	}
	if !utils.CheckPassword(req.Password, user.PasswordHash) {
		_, remaining := s.throttle.RecordFailure(ctx, req.Email, req.ClientIP)
		s.audit.Failure(user.ID, req.Email, model.AuditLoginFailure, req.ClientIP, req.UserAgent,
			fmt.Sprintf("bad password (remaining=%d)", remaining))
		metrics.LoginTotal.WithLabelValues("failure").Inc()
		return nil, errors.New("invalid credentials")
	}
	if user.IsLocked {
		return nil, errors.New("account is locked: " + user.LockReason)
	}
	// Lazy migrate legacy (bcrypt) hashes to Argon2id on first successful login.
	if utils.IsLegacyHash(user.PasswordHash) {
		if newHash, err := utils.HashPassword(req.Password); err == nil {
			_ = s.userRepo.UpdateField(nil, user.ID, "password_hash", newHash)
		}
	}

	if user.Is2FA {
		tempToken, err := utils.GenerateTempToken(user.ID, user.Email, s.secret)
		if err != nil {
			return nil, err
		}
		return &LoginResult{Requires2FA: true, TempToken: tempToken}, nil
	}

	// Step-up gate: if device fingerprint is new for this user AND 2FA is NOT
	// enabled, send a 6-digit OTP via email and require confirmation before
	// issuing real tokens. Skipped entirely on first-ever login (no device
	// history yet) — that's covered by the audit projector marking the very
	// first row as new but the user has zero prior rows so we can't compare.
	deviceID := utils.DeviceFingerprint(req.ClientIP, req.UserAgent, req.AcceptLanguage)
	if s.stepUp != nil && s.requiresStepUp(user.ID, deviceID) {
		// Channel preference: SMS when user has phone, else email.
		ch := ChannelEmail
		if user.Phone != "" {
			ch = ChannelSMS
		}
		token, dispatched, err := s.stepUp.Challenge(ctx,
			user.ID, user.Email, user.FullName, user.Phone,
			req.ClientIP, req.UserAgent, ch)
		if err != nil {
			return nil, err
		}
		s.audit.Failure(user.ID, user.Email, AuditStepUpChallenge, req.ClientIP, req.UserAgent,
			fmt.Sprintf("new device — code sent via %s", dispatched))
		return &LoginResult{RequiresStepUp: true, StepUpToken: token}, nil
	}

	access, refresh, err := s.issueTokenPair(user, req.UserAgent, req.ClientIP)
	if err != nil {
		return nil, err
	}

	if req.ClientIP != "" {
		// Single-column update — never use Save() with partial struct because
		// GORM Save overwrites every column including zero values, blowing away
		// email/fullName/etc.
		_ = s.userRepo.UpdateField(nil, user.ID, "last_login_ip", req.ClientIP)
	}

	// Cache KYC step + lock status for downstream KYCGate middleware.
	s.rdb.Set(ctx, fmt.Sprintf("kyc:%d", user.ID), user.KYCStep, 0)
	s.rdb.Set(ctx, fmt.Sprintf("user_locked:%d", user.ID), boolStr(user.IsLocked), 0)

	// Successful login clears the per-account fail counter.
	s.throttle.RecordSuccess(ctx, req.Email)

	// Device fingerprint reused from step-up gate above.
	newDevice := s.audit.IsNewDeviceForUser(user.ID, deviceID)
	s.audit.SuccessDevice(user.ID, user.Email, model.AuditLoginSuccess,
		req.ClientIP, req.UserAgent, deviceID, "")
	metrics.LoginTotal.WithLabelValues("success").Inc()

	// Anomaly check (velocity + geo). Async-safe: writes audit + email.
	if s.anomaly != nil {
		go s.anomaly.CheckLogin(context.Background(), user.ID, user.Email, req.ClientIP)
	}

	// New-device alert — async mail (NoopMailer logs in dev when SMTP unset).
	if newDevice {
		when := time.Now().Format("2006-01-02 15:04:05 -07:00")
		go func(email, name, ip, ua, when string) {
			_ = s.mail.Send(email,
				"Cảnh báo: Đăng nhập từ thiết bị mới — Micro-Exchange",
				mailer.NewDeviceLoginHTML(name, ip, ua, when))
		}(user.Email, user.FullName, req.ClientIP, req.UserAgent, when)
	}

	return &LoginResult{AccessToken: access, RefreshToken: refresh, User: user}, nil
}

// Login2FA completes 2FA login using a temp token and TOTP code.
func (s *AuthService) Login2FA(tempToken, code, ua, ip string) (string, string, *model.User, error) {
	claims, err := utils.ValidateTempToken(tempToken, s.secret)
	if err != nil {
		return "", "", nil, errors.New("invalid or expired temp token")
	}
	u, err := s.userRepo.FindByID(claims.UserID)
	if err != nil {
		return "", "", nil, errors.New("user not found")
	}
	if !u.Is2FA {
		return "", "", nil, errors.New("2FA not enabled for this user")
	}
	if !s.totp.ValidateCode(u.TwoFASecret, code) {
		return "", "", nil, errors.New("invalid 2FA code")
	}
	access, refresh, err := s.issueTokenPair(u, ua, ip)
	if err != nil {
		return "", "", nil, err
	}
	return access, refresh, u, nil
}

// RefreshToken rotates the presented refresh token and issues a new pair.
// Returns (newAccess, newRefresh, error).
func (s *AuthService) RefreshToken(rawRefresh, ua, ip string) (string, string, error) {
	newRaw, userID, err := s.rtSvc.Rotate(rawRefresh, ua, ip)
	if err != nil {
		return "", "", err
	}
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return "", "", errors.New("user not found")
	}
	if user.IsLocked {
		_ = s.rtSvc.RevokeAllForUser(userID, model.RevokeReasonAdmin)
		return "", "", errors.New("account is locked")
	}
	access, err := utils.GenerateAccessToken(user.ID, user.Email, user.Role, s.secret)
	if err != nil {
		return "", "", err
	}
	return access, newRaw, nil
}

// Logout revokes a single refresh token (idempotent).
func (s *AuthService) Logout(rawRefresh string, userID uint, email, ip, ua string) error {
	if rawRefresh == "" {
		return nil
	}
	err := s.rtSvc.Revoke(rawRefresh, model.RevokeReasonLogout)
	if userID != 0 {
		s.audit.Success(userID, email, model.AuditLogout, ip, ua, "")
	}
	return err
}

// LogoutAll revokes every active session of a user.
func (s *AuthService) LogoutAll(userID uint) error {
	return s.rtSvc.RevokeAllForUser(userID, model.RevokeReasonAdmin)
}

func (s *AuthService) GetProfile(userID uint) (*model.User, error) {
	return s.userRepo.FindByID(userID)
}

func (s *AuthService) UpdateProfile(userID uint, fullName string) (*model.User, error) {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, errors.New("user not found")
	}
	if fullName != "" {
		user.FullName = fullName
	}
	if err := s.userRepo.Update(nil, user); err != nil {
		return nil, err
	}
	return user, nil
}

// ChangePassword verifies the old password, updates to the new one, and
// revokes all active refresh-token families (force re-login on every device).
func (s *AuthService) ChangePassword(userID uint, oldPass, newPass string) error {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return errors.New("user not found")
	}
	if !utils.CheckPassword(oldPass, user.PasswordHash) {
		return errors.New("current password is incorrect")
	}
	// Block known-breached passwords on rotation, same policy as register.
	if err := utils.CheckPasswordPolicy(newPass); err != nil {
		return err
	}
	hash, err := utils.HashPassword(newPass)
	if err != nil {
		return err
	}
	user.PasswordHash = hash
	if err := s.userRepo.UpdateField(nil, user.ID, "password_hash", hash); err != nil {
		return err
	}
	// Security: force logout everywhere.
	_ = s.rtSvc.RevokeAllForUser(userID, model.RevokeReasonPasswordChange)
	s.audit.Success(user.ID, user.Email, model.AuditPasswordChange, "", "", "")
	return nil
}

func (s *AuthService) GenerateWSToken(userID uint) (string, error) {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return "", err
	}
	return utils.GenerateAccessToken(user.ID, user.Email, user.Role, s.secret)
}

func (s *AuthService) GetUserByID(userID uint) (*model.User, error) {
	return s.userRepo.FindByID(userID)
}

func (s *AuthService) Enable2FA(userID uint) (secret, qrURL string, err error) {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return "", "", errors.New("user not found")
	}
	secret, qrURL, err = s.totp.GenerateSecret(user.Email)
	if err != nil {
		return "", "", err
	}
	if err := s.userRepo.UpdateField(nil, userID, "two_fa_secret", secret); err != nil {
		return "", "", err
	}
	return secret, qrURL, nil
}

func (s *AuthService) Verify2FA(userID uint, code string) error {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return errors.New("user not found")
	}
	if user.TwoFASecret == "" {
		return errors.New("2FA setup not initiated")
	}
	if !s.totp.ValidateCode(user.TwoFASecret, code) {
		return errors.New("invalid 2FA code")
	}
	return s.userRepo.UpdateField(nil, userID, "is2_fa", true)
}

func (s *AuthService) Disable2FA(userID uint, code string) error {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return errors.New("user not found")
	}
	if !user.Is2FA {
		return errors.New("2FA is not enabled")
	}
	if !s.totp.ValidateCode(user.TwoFASecret, code) {
		return errors.New("invalid 2FA code")
	}
	return s.userRepo.UpdateFields(nil, userID, map[string]interface{}{
		"is2_fa": false, "two_fa_secret": "",
	})
}

// requiresStepUp returns true when the device fingerprint is unknown for this
// user AND they have at least one prior login.success row. We skip step-up on
// a user's very first login (no history to compare against).
//
// Set STEP_UP_ENABLED=false (the default in dev) to short-circuit the gate
// entirely — useful when the email transport is not configured locally.
func (s *AuthService) requiresStepUp(userID uint, deviceID string) bool {
	if os.Getenv("STEP_UP_ENABLED") != "true" {
		return false
	}
	if s.audit == nil || deviceID == "" {
		return false
	}
	// If this is the user's first login.success ever, allow without step-up.
	rows, total, err := s.audit.ListByUser(userID, 1, 1)
	if err != nil || total == 0 || len(rows) == 0 {
		return false
	}
	// New device + has prior history → require step-up.
	return s.audit.IsNewDeviceForUser(userID, deviceID)
}

// CompleteStepUp finishes the step-up flow with a verified token+code,
// then issues real cookies. Returns the same shape as Login.
func (s *AuthService) CompleteStepUp(token, code, ua, ip string) (*LoginResult, error) {
	if s.stepUp == nil {
		return nil, errors.New("step-up not configured")
	}
	res, err := s.stepUp.ConfirmStepUp(context.Background(), token, code, ip, ua)
	if err != nil {
		return nil, err
	}
	user, err := s.userRepo.FindByID(res.UserID)
	if err != nil {
		return nil, errors.New("user not found")
	}
	access, refresh, err := s.issueTokenPair(user, ua, ip)
	if err != nil {
		return nil, err
	}
	// Record device fingerprint so subsequent logins won't trigger step-up.
	deviceID := utils.DeviceFingerprint(ip, ua, "")
	s.audit.SuccessDevice(user.ID, user.Email, model.AuditLoginSuccess, ip, ua, deviceID,
		"step-up confirmed")
	return &LoginResult{AccessToken: access, RefreshToken: refresh, User: user}, nil
}

// issueTokenPair generates a fresh access JWT + a brand-new refresh family.
func (s *AuthService) issueTokenPair(u *model.User, ua, ip string) (string, string, error) {
	access, err := utils.GenerateAccessToken(u.ID, u.Email, u.Role, s.secret)
	if err != nil {
		return "", "", err
	}
	refresh, _, err := s.rtSvc.IssueRoot(u.ID, ua, ip)
	if err != nil {
		return "", "", err
	}
	return access, refresh, nil
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// ─────────────── Seed helpers ───────────────
//
// Seed flows publish user.registered events. Wallet-service consumes those
// events and creates/funds the wallets in its own DB. Auth-service NEVER
// touches wallets directly.

// SeedAdmin creates demo admin + demo user when SEED_DEMO_DATA=true and
// no users exist yet. Wallet creation is delegated to wallet-service via
// user.registered events with a SeedProfile hint.
func (s *AuthService) SeedAdmin() {
	if os.Getenv("SEED_DEMO_DATA") != "true" {
		return
	}
	if s.userRepo.Count() > 0 {
		return
	}

	adminPass := os.Getenv("ADMIN_PASSWORD")
	if adminPass == "" {
		adminPass = "admin123"
	}
	hash, _ := utils.HashPassword(adminPass)
	admin := &model.User{Email: "admin@exchange.com", PasswordHash: hash, FullName: "Admin", Role: "ADMIN", KYCStatus: "VERIFIED", KYCStep: 4, EmailVerified: true}
	if err := s.userRepo.Create(nil, admin); err == nil {
		s.bus.Publish(context.Background(), eventbus.TopicUserRegistered, eventbus.UserRegisteredEvent{
			UserID: admin.ID, Email: admin.Email, FullName: admin.FullName,
			Role:        "ADMIN",
			SeedProfile: "admin", // wallet-service tops up balances accordingly
		})
	}

	hash2, _ := utils.HashPassword("user123")
	user := &model.User{Email: "user@exchange.com", PasswordHash: hash2, FullName: "Demo User", Role: "USER"}
	if err := s.userRepo.Create(nil, user); err == nil {
		s.bus.Publish(context.Background(), eventbus.TopicUserRegistered, eventbus.UserRegisteredEvent{
			UserID: user.ID, Email: user.Email, FullName: user.FullName,
			Role:        "USER",
			SeedProfile: "demo",
		})
	}
}

// EnsureFeeWallet seeds the platform fee-collection user (role=SYSTEM, locked,
// no password). Returns the user ID — caller caches it for fee crediting.
// The wallet rows themselves are created by wallet-service's user.registered
// consumer (with SeedProfile="system" → no funded balances, just the rows).
//
// Idempotent: if the user already exists, returns its ID without changes.
func (s *AuthService) EnsureFeeWallet() (uint, error) {
	if u, err := s.userRepo.FindByEmail(types.SystemEmailFeeWallet); err == nil && u != nil && u.ID != 0 {
		return u.ID, nil
	}
	user := &model.User{
		Email:         types.SystemEmailFeeWallet,
		PasswordHash:  "", // empty — bcrypt comparisons always fail, login impossible
		FullName:      types.SystemNameFeeWallet,
		Role:          types.RoleSystem,
		KYCStatus:     "VERIFIED",
		EmailVerified: true,
		IsLocked:      true, // belt-and-suspenders: locked even if password somehow set
		LockReason:    "system account — locked by design",
	}
	if err := s.userRepo.Create(nil, user); err != nil {
		return 0, err
	}
	s.bus.Publish(context.Background(), eventbus.TopicUserRegistered, eventbus.UserRegisteredEvent{
		UserID: user.ID, Email: user.Email, FullName: user.FullName,
		Role:        "SYSTEM",
		SeedProfile: "system",
	})
	log.Printf("[auth] seeded fee_wallet system user id=%d", user.ID)
	return user.ID, nil
}

// CleanupExpiredTokens prunes expired/revoked tokens older than 30 days.
// Run periodically (cron / background).
func (s *AuthService) CleanupExpiredTokens() {
	if s.db == nil {
		return
	}
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	res := s.db.Where("expires_at < ? OR (revoked_at IS NOT NULL AND revoked_at < ?)", cutoff, cutoff).
		Delete(&model.RefreshToken{})
	if res.RowsAffected > 0 {
		log.Printf("[auth] pruned %d expired refresh tokens", res.RowsAffected)
	}
}
