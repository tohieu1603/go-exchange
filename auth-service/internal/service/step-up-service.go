package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/cryptox/auth-service/internal/model"
	"github.com/cryptox/auth-service/internal/repository"
	"github.com/cryptox/shared/mailer"
	"github.com/cryptox/shared/sms"
	"github.com/redis/go-redis/v9"
)

// StepUpService gates a successful password authentication behind a second
// factor when the login originates from an UNRECOGNIZED device.
//
// Flow when 2FA is NOT enabled and new device is detected:
//   1. Generate one-time 6-digit numeric code (cryptographically random).
//   2. Store hash(code) in Redis under stepUpToken with TTL 5 min.
//   3. Email code to the user. Issue stepUpToken to client (NOT a JWT).
//   4. Client POSTs /api/auth/step-up {token, code}.
//   5. ConfirmStepUp matches hash, deletes Redis key, returns user.
//
// The Redis value carries the bound user ID + email + IP/UA so the caller
// can rebuild login context (IP/UA used for refresh-token issuance).
type StepUpService struct {
	rdb   *redis.Client
	mail  mailer.Mailer
	sms   sms.Sender
	users repository.UserRepo
	audit *AuditLogService
}

func NewStepUpService(
	rdb *redis.Client,
	mail mailer.Mailer,
	smsSender sms.Sender,
	users repository.UserRepo,
	audit *AuditLogService,
) *StepUpService {
	if mail == nil {
		mail = mailer.NoopMailer{}
	}
	if smsSender == nil {
		smsSender = sms.NoopSender{}
	}
	return &StepUpService{rdb: rdb, mail: mail, sms: smsSender, users: users, audit: audit}
}

// Channel selects the OTP delivery medium for step-up.
type Channel string

const (
	ChannelEmail Channel = "email"
	ChannelSMS   Channel = "sms"
)

const (
	stepUpTTL    = 5 * time.Minute
	stepUpPrefix = "stepup:"
)

// Challenge generates a fresh code, persists the challenge, dispatches the
// code via the chosen channel, and returns the stepUpToken.
//
// Channel fallback rules:
//   - SMS requested but user has no phone number → fall back to email.
//   - Default (empty channel) → email.
func (s *StepUpService) Challenge(
	ctx context.Context,
	userID uint, email, fullName, phone, ip, userAgent string,
	channel Channel,
) (string, Channel, error) {
	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", err
	}
	token := hex.EncodeToString(tokenBytes)
	code := randomDigits(6)

	resolved := channel
	if resolved == ChannelSMS && phone == "" {
		resolved = ChannelEmail // safe fallback when phone missing
	}
	if resolved == "" {
		resolved = ChannelEmail
	}

	payload := map[string]string{
		"user_id":    fmt.Sprintf("%d", userID),
		"email":      email,
		"code_hash":  sha256Hex(code),
		"ip":         ip,
		"user_agent": userAgent,
		"channel":    string(resolved),
	}
	if err := s.rdb.HSet(ctx, stepUpPrefix+token, payload).Err(); err != nil {
		return "", "", err
	}
	s.rdb.Expire(ctx, stepUpPrefix+token, stepUpTTL)

	// Dispatch out of band so login response isn't slowed by SMTP/Twilio.
	go func(channel Channel) {
		switch channel {
		case ChannelSMS:
			if err := s.sms.Send(phone, sms.VerifyCodeMessage(code)); err != nil {
				log.Printf("[stepup] SMS send failed user=%d: %v", userID, err)
			}
		default:
			if err := s.mail.Send(email,
				"Xác thực thiết bị mới — Micro-Exchange",
				mailer.VerifyEmailHTML(fullName, code)); err != nil {
				log.Printf("[stepup] email send failed user=%d: %v", userID, err)
			}
		}
	}(resolved)

	return token, resolved, nil
}

// StepUpResolve holds the credentials Login needs to issue real tokens after
// successful step-up confirmation.
type StepUpResolve struct {
	UserID    uint
	Email     string
	IP        string
	UserAgent string
}

// ConfirmStepUp verifies (token, code), deletes the challenge on success.
// Records audit rows for both success and failure.
func (s *StepUpService) ConfirmStepUp(ctx context.Context, token, code, ip, ua string) (*StepUpResolve, error) {
	if token == "" || code == "" {
		return nil, errors.New("token and code required")
	}
	key := stepUpPrefix + token
	payload, err := s.rdb.HGetAll(ctx, key).Result()
	if err != nil || len(payload) == 0 {
		s.audit.Failure(0, "", "stepup.failure", ip, ua, "token expired or unknown")
		return nil, errors.New("step-up challenge expired or invalid")
	}
	if sha256Hex(code) != payload["code_hash"] {
		s.audit.Failure(parseUintSafe(payload["user_id"]), payload["email"],
			"stepup.failure", ip, ua, "wrong code")
		return nil, errors.New("invalid code")
	}
	// One-shot: delete on success.
	s.rdb.Del(ctx, key)

	resolve := &StepUpResolve{
		UserID:    parseUintSafe(payload["user_id"]),
		Email:     payload["email"],
		IP:        payload["ip"],
		UserAgent: payload["user_agent"],
	}
	s.audit.Success(resolve.UserID, resolve.Email, "stepup.success", ip, ua, "")
	return resolve, nil
}

func randomDigits(n int) string {
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		v, _ := rand.Int(rand.Reader, big.NewInt(10))
		out[i] = byte('0' + v.Int64())
	}
	return string(out)
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func parseUintSafe(s string) uint {
	var n uint
	for _, c := range s {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + uint(c-'0')
	}
	return n
}

// AuditAction constants for step-up. Hard-coded since they don't go through
// the model.AuditXxx constants (kept service-local for now).
const (
	AuditStepUpChallenge = "stepup.challenge"
	AuditStepUpSuccess   = "stepup.success"
	AuditStepUpFailure   = "stepup.failure"
)

// Ensure model import isn't dead — referenced via parseUintSafe consumers.
var _ = model.AuditLoginSuccess
