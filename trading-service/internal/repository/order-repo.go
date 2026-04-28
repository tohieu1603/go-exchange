package repository

import (
	"github.com/cryptox/trading-service/internal/model"
	"gorm.io/gorm"
)

// OrderRepo defines persistence operations for orders.
type OrderRepo interface {
	Create(tx *gorm.DB, order *model.Order) error
	FindByID(id uint) (*model.Order, error)
	FindByUserAndID(userID, orderID uint) (*model.Order, error)
	UpdateStatus(tx *gorm.DB, id uint, status string, filledAmount float64, price float64) error
	FindOpen(userID uint) ([]model.Order, error)
	FindPaginated(userID uint, status string, page, size int) ([]model.Order, int64, error)
	Save(tx *gorm.DB, order *model.Order) error
	FindOpenLimitOrders() ([]model.Order, error)
}

type orderRepo struct{ db *gorm.DB }

func NewOrderRepo(db *gorm.DB) OrderRepo { return &orderRepo{db: db} }

func (r *orderRepo) getDB(tx *gorm.DB) *gorm.DB {
	if tx != nil {
		return tx
	}
	return r.db
}

func (r *orderRepo) Create(tx *gorm.DB, order *model.Order) error {
	return r.getDB(tx).Create(order).Error
}

func (r *orderRepo) FindByID(id uint) (*model.Order, error) {
	var o model.Order
	err := r.db.First(&o, id).Error
	return &o, err
}

func (r *orderRepo) FindByUserAndID(userID, orderID uint) (*model.Order, error) {
	var o model.Order
	err := r.db.Where("id = ? AND user_id = ?", orderID, userID).First(&o).Error
	return &o, err
}

func (r *orderRepo) UpdateStatus(tx *gorm.DB, id uint, status string, filledAmount float64, price float64) error {
	return r.getDB(tx).Exec(`
		UPDATE orders SET status = ?, filled_amount = ?, price = CASE WHEN price = 0 THEN ? ELSE price END, updated_at = NOW()
		WHERE id = ?
	`, status, filledAmount, price, id).Error
}

func (r *orderRepo) FindOpen(userID uint) ([]model.Order, error) {
	var orders []model.Order
	err := r.db.Where("user_id = ? AND status IN ?", userID, []string{"OPEN", "PARTIAL"}).
		Order("created_at DESC").Find(&orders).Error
	return orders, err
}

func (r *orderRepo) FindPaginated(userID uint, status string, page, size int) ([]model.Order, int64, error) {
	var orders []model.Order
	var total int64
	offset := (page - 1) * size

	q := r.db.Model(&model.Order{}).Where("user_id = ?", userID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	q.Count(&total)
	err := q.Order("created_at DESC").Limit(size).Offset(offset).Find(&orders).Error
	return orders, total, err
}

func (r *orderRepo) Save(tx *gorm.DB, order *model.Order) error {
	return r.getDB(tx).Save(order).Error
}

func (r *orderRepo) FindOpenLimitOrders() ([]model.Order, error) {
	var orders []model.Order
	err := r.db.Where("status IN ? AND type = ?", []string{"OPEN", "PARTIAL"}, "LIMIT").Find(&orders).Error
	return orders, err
}
