package repository

import (
	"github.com/cryptox/auth-service/internal/model"
	"github.com/cryptox/shared/types"
	"gorm.io/gorm"
)

type userRepo struct{ db *gorm.DB }

func NewUserRepo(db *gorm.DB) UserRepo { return &userRepo{db: db} }

func (r *userRepo) getDB(tx *gorm.DB) *gorm.DB {
	if tx != nil {
		return tx
	}
	return r.db
}

func (r *userRepo) Create(tx *gorm.DB, user *model.User) error {
	return r.getDB(tx).Create(user).Error
}

func (r *userRepo) FindByEmail(email string) (*model.User, error) {
	var u model.User
	err := r.db.Where("email = ?", email).First(&u).Error
	return &u, err
}

func (r *userRepo) FindByID(id uint) (*model.User, error) {
	var u model.User
	err := r.db.First(&u, id).Error
	return &u, err
}

func (r *userRepo) Update(tx *gorm.DB, user *model.User) error {
	return r.getDB(tx).Save(user).Error
}

func (r *userRepo) UpdateField(tx *gorm.DB, id uint, field string, value interface{}) error {
	return r.getDB(tx).Model(&model.User{}).Where("id = ?", id).Update(field, value).Error
}

func (r *userRepo) UpdateFields(tx *gorm.DB, id uint, updates map[string]interface{}) error {
	return r.getDB(tx).Model(&model.User{}).Where("id = ?", id).Updates(updates).Error
}

// Count returns the number of "real" users (USER + ADMIN). SYSTEM accounts
// are excluded so demo seed conditions don't get tripped by infrastructure rows.
func (r *userRepo) Count() int64 {
	var count int64
	r.db.Model(&model.User{}).Where("role <> ?", types.RoleSystem).Count(&count)
	return count
}

func (r *userRepo) FindPaginated(search string, page, size int) ([]model.User, int64, error) {
	var users []model.User
	var total int64
	offset := (page - 1) * size

	// SYSTEM accounts (fee wallet, future internal users) are hidden from
	// admin lists — they are infrastructure, not customers.
	q := r.db.Model(&model.User{}).Where("role <> ?", types.RoleSystem)
	if search != "" {
		q = q.Where("email ILIKE ? OR full_name ILIKE ?", "%"+search+"%", "%"+search+"%")
	}
	q.Count(&total)
	err := q.Order("created_at DESC").Limit(size).Offset(offset).Find(&users).Error
	return users, total, err
}

func (r *userRepo) UpdateKYC(id uint, status string) error {
	return r.db.Model(&model.User{}).Where("id = ?", id).Update("kyc_status", status).Error
}
