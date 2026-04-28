package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cryptox/auth-service/internal/repository"
	"github.com/cryptox/auth-service/internal/model"
	"github.com/cryptox/shared/utils"
)

// APIKeyService manages programmatic access credentials.
//
// Authentication scheme (HMAC-SHA256, similar to Binance):
//   Headers:
//     X-API-Key: <keyId>
//     X-API-Sign: hex( HMAC-SHA256(secret, timestamp + method + path + body) )
//     X-API-Timestamp: <unix-ms>   (must be within ±5s of server time)
type APIKeyService struct {
	repo repository.APIKeyRepo
}

func NewAPIKeyService(repo repository.APIKeyRepo) *APIKeyService {
	return &APIKeyService{repo: repo}
}

type CreateAPIKeyResult struct {
	Key    *model.APIKey `json:"key"`
	Secret string        `json:"secret"` // shown ONCE
}

func (s *APIKeyService) Create(userID uint, label, perms, ipWhitelist string, expiresAt *time.Time) (*CreateAPIKeyResult, error) {
	if label == "" {
		return nil, errors.New("label is required")
	}
	perms = normalizePermissions(perms)
	keyID, err := utils.NewOpaqueToken()
	if err != nil {
		return nil, err
	}
	secret, err := utils.NewOpaqueToken()
	if err != nil {
		return nil, err
	}
	keyID = "mxk_" + keyID[:24] // visually distinct, shorter

	k := &model.APIKey{
		UserID:      userID,
		Label:       label,
		KeyID:       keyID,
		SecretHash:  utils.HashToken(secret),
		Permissions: perms,
		IPWhitelist: ipWhitelist,
		ExpiresAt:   expiresAt,
	}
	if err := s.repo.Create(k); err != nil {
		return nil, err
	}
	return &CreateAPIKeyResult{Key: k, Secret: secret}, nil
}

func (s *APIKeyService) List(userID uint) ([]model.APIKey, error) {
	return s.repo.ListByUser(userID)
}

func (s *APIKeyService) Revoke(userID, id uint) error {
	return s.repo.Revoke(id, userID)
}

// Authenticate validates an API key + signature. Returns the key on success.
// Caller is responsible for permission checks (key.Permissions).
func (s *APIKeyService) Authenticate(keyID, signature, timestamp, method, path string, body []byte, clientIP string) (*model.APIKey, error) {
	k, err := s.repo.FindByKeyID(keyID)
	if err != nil {
		return nil, errors.New("invalid api key")
	}
	if k.ExpiresAt != nil && time.Now().After(*k.ExpiresAt) {
		return nil, errors.New("api key expired")
	}
	if k.IPWhitelist != "" && !ipAllowed(clientIP, k.IPWhitelist) {
		return nil, errors.New("ip not whitelisted")
	}
	// Timestamp window — guards replay.
	tsMs, err := parseInt64(timestamp)
	if err != nil {
		return nil, errors.New("invalid timestamp")
	}
	now := time.Now().UnixMilli()
	if now-tsMs > 5000 || tsMs-now > 5000 {
		return nil, errors.New("timestamp out of window")
	}
	expected := utils.HMACHex(k.SecretHash, fmt.Sprintf("%s%s%s%s", timestamp, method, path, string(body)))
	if !secureEqual(expected, signature) {
		return nil, errors.New("invalid signature")
	}
	_ = s.repo.UpdateLastUsed(k.ID, clientIP)
	return k, nil
}

// HasPermission tests whether `perm` is in the comma-separated permissions list.
func HasPermission(k *model.APIKey, perm string) bool {
	for _, p := range strings.Split(k.Permissions, ",") {
		if strings.TrimSpace(p) == perm {
			return true
		}
	}
	return false
}

func normalizePermissions(p string) string {
	if p == "" {
		return "read"
	}
	parts := strings.Split(p, ",")
	allowed := map[string]bool{
		model.APIPermRead: true, model.APIPermTrade: true, model.APIPermWithdraw: true,
	}
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, x := range parts {
		x = strings.TrimSpace(x)
		if allowed[x] && !seen[x] {
			out = append(out, x)
			seen[x] = true
		}
	}
	if len(out) == 0 {
		return "read"
	}
	return strings.Join(out, ",")
}

func ipAllowed(clientIP, whitelist string) bool {
	for _, allowed := range strings.Split(whitelist, ",") {
		if strings.TrimSpace(allowed) == clientIP {
			return true
		}
	}
	return false
}

func parseInt64(s string) (int64, error) {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errors.New("not a number")
		}
		n = n*10 + int64(c-'0')
	}
	return n, nil
}

// secureEqual is a constant-time string compare.
func secureEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}
