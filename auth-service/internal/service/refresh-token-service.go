package service

import (
	"errors"
	"time"

	"github.com/cryptox/auth-service/internal/model"
	"github.com/cryptox/shared/utils"
	"github.com/cryptox/auth-service/internal/repository"
)

// RefreshTokenService implements the rotating refresh-token pattern with
// token-family theft detection.
//
// Flow on rotate:
//  1. Hash the presented raw token, look up by hash.
//  2. If revoked OR expired → reject.
//  3. If used_at IS NOT NULL → REPLAY: token was already rotated. Revoke
//     entire family (reason=replay_detected). User must re-login.
//  4. Otherwise: mark current token used_at=now, issue a new token in the
//     SAME family with parent_id = current.id.
type RefreshTokenService struct {
	repo repository.RefreshTokenRepo
}

func NewRefreshTokenService(repo repository.RefreshTokenRepo) *RefreshTokenService {
	return &RefreshTokenService{repo: repo}
}

// IssueRoot creates a new refresh token starting a fresh family (login / 2FA login).
// Returns the raw token (sent to client) — only the hash is stored.
func (s *RefreshTokenService) IssueRoot(userID uint, ua, ip string) (string, *model.RefreshToken, error) {
	raw, err := utils.NewOpaqueToken()
	if err != nil {
		return "", nil, err
	}
	rt := &model.RefreshToken{
		UserID:    userID,
		TokenHash: utils.HashToken(raw),
		FamilyID:  utils.NewFamilyID(),
		ParentID:  nil,
		UserAgent: ua,
		IP:        ip,
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(utils.RefreshTokenTTL),
	}
	if err := s.repo.Create(rt); err != nil {
		return "", nil, err
	}
	return raw, rt, nil
}

// Rotate validates the presented raw token and issues a successor in the same
// family. On replay (already-used token presented again), revokes the entire
// family and returns an error.
func (s *RefreshTokenService) Rotate(raw, ua, ip string) (newRaw string, userID uint, err error) {
	current, err := s.repo.FindByHash(utils.HashToken(raw))
	if err != nil {
		return "", 0, errors.New("refresh token not found")
	}
	if current.RevokedAt != nil {
		return "", 0, errors.New("refresh token revoked: " + current.RevokedReason)
	}
	if time.Now().After(current.ExpiresAt) {
		_ = s.repo.RevokeByID(current.ID, model.RevokeReasonExpired)
		return "", 0, errors.New("refresh token expired")
	}
	// Replay detection — used_at already set means this token was rotated
	// previously. Anyone presenting it now is probably an attacker holding a
	// stolen copy. Burn the family.
	if current.UsedAt != nil {
		_ = s.repo.RevokeFamily(current.FamilyID, model.RevokeReasonReplayDetected)
		return "", 0, errors.New("refresh token replay detected — family revoked")
	}

	if err := s.repo.MarkUsed(current.ID); err != nil {
		return "", 0, err
	}

	rawNew, err := utils.NewOpaqueToken()
	if err != nil {
		return "", 0, err
	}
	successor := &model.RefreshToken{
		UserID:    current.UserID,
		TokenHash: utils.HashToken(rawNew),
		FamilyID:  current.FamilyID,
		ParentID:  &current.ID,
		UserAgent: ua,
		IP:        ip,
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(utils.RefreshTokenTTL),
	}
	if err := s.repo.Create(successor); err != nil {
		return "", 0, err
	}
	return rawNew, current.UserID, nil
}

// Revoke a single token by raw value (logout).
func (s *RefreshTokenService) Revoke(raw, reason string) error {
	rt, err := s.repo.FindByHash(utils.HashToken(raw))
	if err != nil {
		return nil // already gone — idempotent
	}
	if rt.RevokedAt != nil {
		return nil
	}
	return s.repo.RevokeByID(rt.ID, reason)
}

// RevokeAllForUser invalidates every active refresh token for a user
// (e.g. after password change, security event).
func (s *RefreshTokenService) RevokeAllForUser(userID uint, reason string) error {
	return s.repo.RevokeByUser(userID, reason)
}
