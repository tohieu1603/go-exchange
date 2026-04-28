package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/cryptox/auth-service/internal/repository"
	"github.com/cryptox/shared/mailer"
	"github.com/cryptox/auth-service/internal/model"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const (
	emailVerifyKeyPrefix = "email_verify:"
	emailVerifyTTL       = 10 * time.Minute
	requiredDocTypes     = 3
)

var validDocTypes = map[string]bool{
	"CCCD_FRONT": true,
	"CCCD_BACK":  true,
	"SELFIE":     true,
}

// KYCService handles step-by-step KYC verification flow.
type KYCService struct {
	kycRepo  repository.KYCRepo
	userRepo repository.UserRepo
	rdb      *redis.Client
	mail     mailer.Mailer
}

func NewKYCService(kycRepo repository.KYCRepo, userRepo repository.UserRepo, rdb *redis.Client, mail mailer.Mailer) *KYCService {
	if mail == nil {
		mail = mailer.NoopMailer{}
	}
	return &KYCService{kycRepo: kycRepo, userRepo: userRepo, rdb: rdb, mail: mail}
}

// SubmitProfileReq holds profile submission data for step 2.
type SubmitProfileReq struct {
	FirstName   string `json:"firstName" binding:"required"`
	LastName    string `json:"lastName" binding:"required"`
	DateOfBirth string `json:"dateOfBirth"`
	Phone       string `json:"phone"`
	Address     string `json:"address"`
	Ward        string `json:"ward"`
	District    string `json:"district"`
	City        string `json:"city" binding:"required"`
	PostalCode  string `json:"postalCode"`
	Country     string `json:"country"`
	Occupation  string `json:"occupation" binding:"required"`
	Income      string `json:"income" binding:"required"`
	TradingExp  string `json:"tradingExp"`
	Purpose     string `json:"purpose"`
}

// KYCStatusResponse holds the full KYC state for a user.
type KYCStatusResponse struct {
	KYCStep       int                  `json:"kycStep"`
	EmailVerified bool                 `json:"emailVerified"`
	KYCStatus     string               `json:"kycStatus"`
	Profile       *model.KYCProfile    `json:"profile,omitempty"`
	Documents     []model.KYCDocument  `json:"documents"`
}

// SendVerifyEmail generates a 6-digit code, stores it in Redis (TTL 10min),
// and dispatches it via the configured mailer (SMTP in prod, log in dev).
// The code is NEVER returned to the API caller in production.
func (s *KYCService) SendVerifyEmail(userID uint) error {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return errors.New("user not found")
	}
	code := fmt.Sprintf("%06d", rand.Intn(1000000))
	key := fmt.Sprintf("%s%d", emailVerifyKeyPrefix, userID)
	if err := s.rdb.Set(context.Background(), key, code, emailVerifyTTL).Err(); err != nil {
		return fmt.Errorf("store verify code: %w", err)
	}
	if err := s.mail.Send(user.Email, "Mã xác thực email — Micro-Exchange", mailer.VerifyEmailHTML(user.FullName, code)); err != nil {
		log.Printf("[kyc] mail send failed user=%d: %v", userID, err)
		// Do not fail the request — user can retry SendVerifyEmail.
	}
	return nil
}

// VerifyEmail checks the code and marks email as verified, advancing KYCStep to 1.
func (s *KYCService) VerifyEmail(userID uint, code string) error {
	key := fmt.Sprintf("%s%d", emailVerifyKeyPrefix, userID)
	stored, err := s.rdb.Get(context.Background(), key).Result()
	if err != nil {
		return errors.New("verification code expired or not found")
	}
	if stored != code {
		return errors.New("invalid verification code")
	}
	s.rdb.Del(context.Background(), key)
	if err := s.userRepo.UpdateFields(nil, userID, map[string]interface{}{
		"email_verified": true,
		"kyc_step":       1,
	}); err != nil {
		return err
	}
	s.rdb.Set(context.Background(), fmt.Sprintf("kyc:%d", userID), 1, 0)
	return nil
}

// SubmitProfile creates or updates the KYC profile, advancing KYCStep to 2.
func (s *KYCService) SubmitProfile(userID uint, req SubmitProfileReq) error {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return errors.New("user not found")
	}
	if !user.EmailVerified {
		return errors.New("email must be verified before submitting profile")
	}

	existing, err := s.kycRepo.FindProfileByUserID(userID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("fetch profile: %w", err)
	}

	profile := &model.KYCProfile{
		UserID: userID, FirstName: req.FirstName, LastName: req.LastName,
		DateOfBirth: req.DateOfBirth, Phone: req.Phone, Address: req.Address,
		Ward: req.Ward, District: req.District, City: req.City,
		PostalCode: req.PostalCode, Occupation: req.Occupation, Income: req.Income,
		TradingExp: req.TradingExp, Purpose: req.Purpose,
	}
	if req.Country != "" {
		profile.Country = req.Country
	} else {
		profile.Country = "VN"
	}

	if existing != nil && existing.ID != 0 {
		profile.ID = existing.ID
		if err := s.kycRepo.UpdateProfile(nil, profile); err != nil {
			return fmt.Errorf("update profile: %w", err)
		}
	} else {
		if err := s.kycRepo.CreateProfile(nil, profile); err != nil {
			return fmt.Errorf("create profile: %w", err)
		}
	}

	if err := s.userRepo.UpdateField(nil, userID, "kyc_step", 2); err != nil {
		return err
	}
	s.rdb.Set(context.Background(), fmt.Sprintf("kyc:%d", userID), 2, 0)
	return nil
}

// UploadDocument records a KYC document. If all 3 docs uploaded, advances to step 3 + sets PENDING.
func (s *KYCService) UploadDocument(userID uint, docType, filePath string) error {
	if !validDocTypes[docType] {
		return errors.New("invalid document type: must be CCCD_FRONT, CCCD_BACK, or SELFIE")
	}
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return errors.New("user not found")
	}
	if user.KYCStep < 2 {
		return errors.New("profile must be submitted before uploading documents")
	}

	// Upsert: replace existing doc of same type
	existing, err := s.kycRepo.FindDocumentByUserAndType(userID, docType)
	if err == nil && existing.ID != 0 {
		if err := s.kycRepo.UpdateDocumentStatus(nil, existing.ID, "PENDING", ""); err != nil {
			return fmt.Errorf("reset doc status: %w", err)
		}
		// Update file path via direct model update
		doc := &model.KYCDocument{ID: existing.ID, UserID: userID, DocType: docType, FilePath: filePath, Status: "PENDING"}
		_ = doc // handled below
	}

	doc := &model.KYCDocument{UserID: userID, DocType: docType, FilePath: filePath, Status: "PENDING"}
	if existing != nil && existing.ID != 0 {
		doc.ID = existing.ID
	}
	if err := s.kycRepo.CreateDocument(nil, doc); err != nil {
		return fmt.Errorf("save document: %w", err)
	}

	// Check if all 3 required docs are present
	docs, err := s.kycRepo.FindDocumentsByUserID(userID)
	if err != nil {
		return nil // non-fatal
	}
	typesSeen := map[string]bool{}
	for _, d := range docs {
		typesSeen[d.DocType] = true
	}
	if typesSeen["CCCD_FRONT"] && typesSeen["CCCD_BACK"] && typesSeen["SELFIE"] {
		if err := s.userRepo.UpdateFields(nil, userID, map[string]interface{}{
			"kyc_step":   3,
			"kyc_status": "PENDING",
		}); err != nil {
			return err
		}
		s.rdb.Set(context.Background(), fmt.Sprintf("kyc:%d", userID), 3, 0)
	}
	return nil
}

// ApproveKYC sets all docs to APPROVED and marks user as VERIFIED.
func (s *KYCService) ApproveKYC(userID uint, adminNote string) error {
	if _, err := s.userRepo.FindByID(userID); err != nil {
		return errors.New("user not found")
	}
	if err := s.kycRepo.UpdateAllDocumentsStatus(nil, userID, "APPROVED", adminNote); err != nil {
		return fmt.Errorf("approve docs: %w", err)
	}
	if err := s.userRepo.UpdateFields(nil, userID, map[string]interface{}{
		"kyc_step":   4,
		"kyc_status": "VERIFIED",
	}); err != nil {
		return err
	}
	s.rdb.Set(context.Background(), fmt.Sprintf("kyc:%d", userID), 4, 0)
	return nil
}

// RejectKYC sets all docs to REJECTED and resets user to step 2 so they can re-upload.
func (s *KYCService) RejectKYC(userID uint, adminNote string) error {
	if _, err := s.userRepo.FindByID(userID); err != nil {
		return errors.New("user not found")
	}
	if err := s.kycRepo.UpdateAllDocumentsStatus(nil, userID, "REJECTED", adminNote); err != nil {
		return fmt.Errorf("reject docs: %w", err)
	}
	return s.userRepo.UpdateFields(nil, userID, map[string]interface{}{
		"kyc_step":   2,
		"kyc_status": "REJECTED",
	})
}

// GetKYCStatus returns the full KYC state for a user.
func (s *KYCService) GetKYCStatus(userID uint) (*KYCStatusResponse, error) {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	resp := &KYCStatusResponse{
		KYCStep:       user.KYCStep,
		EmailVerified: user.EmailVerified,
		KYCStatus:     user.KYCStatus,
		Documents:     []model.KYCDocument{},
	}

	profile, err := s.kycRepo.FindProfileByUserID(userID)
	if err == nil {
		resp.Profile = profile
	}

	docs, err := s.kycRepo.FindDocumentsByUserID(userID)
	if err == nil {
		resp.Documents = docs
	}

	return resp, nil
}

// GetPendingUsers returns paginated users with KYC_STATUS=PENDING.
func (s *KYCService) GetPendingUsers(page, size int) ([]model.User, int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return s.kycRepo.FindPendingUsers(page, size)
}
