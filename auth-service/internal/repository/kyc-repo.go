package repository

import (
	"github.com/cryptox/auth-service/internal/model"
	"gorm.io/gorm"
)

// KYCRepo handles KYC profile and document persistence.
type KYCRepo interface {
	CreateProfile(tx *gorm.DB, profile *model.KYCProfile) error
	FindProfileByUserID(userID uint) (*model.KYCProfile, error)
	UpdateProfile(tx *gorm.DB, profile *model.KYCProfile) error
	CreateDocument(tx *gorm.DB, doc *model.KYCDocument) error
	FindDocumentsByUserID(userID uint) ([]model.KYCDocument, error)
	UpdateDocumentStatus(tx *gorm.DB, docID uint, status, note string) error
	FindDocumentByUserAndType(userID uint, docType string) (*model.KYCDocument, error)
	FindPendingUsers(page, size int) ([]model.User, int64, error)
	UpdateAllDocumentsStatus(tx *gorm.DB, userID uint, status, note string) error
}

type gormKYCRepo struct{ db *gorm.DB }

func NewKYCRepo(db *gorm.DB) KYCRepo { return &gormKYCRepo{db: db} }

func (r *gormKYCRepo) getDB(tx *gorm.DB) *gorm.DB {
	if tx != nil {
		return tx
	}
	return r.db
}

func (r *gormKYCRepo) CreateProfile(tx *gorm.DB, profile *model.KYCProfile) error {
	return r.getDB(tx).Create(profile).Error
}

func (r *gormKYCRepo) FindProfileByUserID(userID uint) (*model.KYCProfile, error) {
	var p model.KYCProfile
	err := r.db.Where("user_id = ?", userID).First(&p).Error
	return &p, err
}

func (r *gormKYCRepo) UpdateProfile(tx *gorm.DB, profile *model.KYCProfile) error {
	return r.getDB(tx).Save(profile).Error
}

func (r *gormKYCRepo) CreateDocument(tx *gorm.DB, doc *model.KYCDocument) error {
	return r.getDB(tx).Create(doc).Error
}

func (r *gormKYCRepo) FindDocumentsByUserID(userID uint) ([]model.KYCDocument, error) {
	var docs []model.KYCDocument
	err := r.db.Where("user_id = ?", userID).Find(&docs).Error
	return docs, err
}

func (r *gormKYCRepo) UpdateDocumentStatus(tx *gorm.DB, docID uint, status, note string) error {
	return r.getDB(tx).Model(&model.KYCDocument{}).Where("id = ?", docID).
		Updates(map[string]interface{}{"status": status, "admin_note": note}).Error
}

func (r *gormKYCRepo) FindDocumentByUserAndType(userID uint, docType string) (*model.KYCDocument, error) {
	var doc model.KYCDocument
	err := r.db.Where("user_id = ? AND doc_type = ?", userID, docType).First(&doc).Error
	return &doc, err
}

func (r *gormKYCRepo) FindPendingUsers(page, size int) ([]model.User, int64, error) {
	var users []model.User
	var total int64
	offset := (page - 1) * size
	q := r.db.Model(&model.User{}).Where("kyc_status = ?", "PENDING")
	q.Count(&total)
	err := q.Order("updated_at DESC").Limit(size).Offset(offset).Find(&users).Error
	return users, total, err
}

func (r *gormKYCRepo) UpdateAllDocumentsStatus(tx *gorm.DB, userID uint, status, note string) error {
	return r.getDB(tx).Model(&model.KYCDocument{}).Where("user_id = ?", userID).
		Updates(map[string]interface{}{"status": status, "admin_note": note}).Error
}
