package repository

import (
	"github.com/cryptox/auth-service/internal/model"
	"gorm.io/gorm"
)

// UserRepo persists the User aggregate. Auth-service owns this table; other
// services receive user data via events (user.registered) or gRPC.
type UserRepo interface {
	Create(tx *gorm.DB, user *model.User) error
	FindByEmail(email string) (*model.User, error)
	FindByID(id uint) (*model.User, error)
	Update(tx *gorm.DB, user *model.User) error
	UpdateField(tx *gorm.DB, id uint, field string, value interface{}) error
	UpdateFields(tx *gorm.DB, id uint, updates map[string]interface{}) error
	Count() int64
	FindPaginated(search string, page, size int) ([]model.User, int64, error)
	UpdateKYC(id uint, status string) error
}
