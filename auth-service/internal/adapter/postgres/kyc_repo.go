package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/cryptox/auth-service/internal/adapter/postgres/sqlc"
	"github.com/cryptox/auth-service/internal/domain"
	"github.com/cryptox/auth-service/internal/usecase"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// KYCRepo is the pgx+sqlc adapter for usecase.KYCRepo.
type KYCRepo struct{ q *sqlc.Queries }

func NewKYCRepo(pool *pgxpool.Pool) *KYCRepo { return &KYCRepo{q: sqlc.New(pool)} }

var _ usecase.KYCRepo = (*KYCRepo)(nil)

func (r *KYCRepo) CreateProfile(ctx context.Context, p *domain.KYCProfile) error {
	row, err := r.q.CreateKYCProfile(ctx, sqlc.CreateKYCProfileParams{
		UserID: int64(p.UserID), FirstName: p.FirstName, LastName: p.LastName, DateOfBirth: p.DateOfBirth,
		Phone: p.Phone, Address: p.Address, Ward: p.Ward, District: p.District, City: p.City,
		PostalCode: p.PostalCode, Country: defaultStr(p.Country, "VN"), Occupation: p.Occupation,
		Income: p.Income, TradingExp: p.TradingExp, Purpose: p.Purpose,
	})
	if err != nil {
		return fmt.Errorf("postgres: create kyc profile: %w", err)
	}
	p.ID = uint(row.ID)
	p.CreatedAt = row.CreatedAt
	p.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *KYCRepo) FindProfileByUserID(ctx context.Context, userID uint) (*domain.KYCProfile, error) {
	row, err := r.q.GetKYCProfileByUser(ctx, int64(userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("postgres: get kyc profile: %w", err)
	}
	p := kycProfileToDomain(row)
	return &p, nil
}

func (r *KYCRepo) UpdateProfile(ctx context.Context, p *domain.KYCProfile) error {
	return wrap("update kyc profile", r.q.UpdateKYCProfile(ctx, sqlc.UpdateKYCProfileParams{
		ID: int64(p.ID), FirstName: p.FirstName, LastName: p.LastName, DateOfBirth: p.DateOfBirth,
		Phone: p.Phone, Address: p.Address, Ward: p.Ward, District: p.District, City: p.City,
		PostalCode: p.PostalCode, Country: p.Country, Occupation: p.Occupation, Income: p.Income,
		TradingExp: p.TradingExp, Purpose: p.Purpose,
	}))
}

func (r *KYCRepo) CreateDocument(ctx context.Context, d *domain.KYCDocument) error {
	row, err := r.q.CreateKYCDocument(ctx, sqlc.CreateKYCDocumentParams{
		UserID: int64(d.UserID), DocType: d.DocType, FilePath: d.FilePath,
		Status: defaultStr(d.Status, "PENDING"), AdminNote: d.AdminNote,
	})
	if err != nil {
		return fmt.Errorf("postgres: create kyc document: %w", err)
	}
	d.ID = uint(row.ID)
	d.CreatedAt = row.CreatedAt
	return nil
}

func (r *KYCRepo) FindDocumentsByUserID(ctx context.Context, userID uint) ([]domain.KYCDocument, error) {
	rows, err := r.q.ListKYCDocumentsByUser(ctx, int64(userID))
	if err != nil {
		return nil, fmt.Errorf("postgres: list kyc documents: %w", err)
	}
	out := make([]domain.KYCDocument, len(rows))
	for i, m := range rows {
		out[i] = kycDocToDomain(m)
	}
	return out, nil
}

func (r *KYCRepo) UpdateDocumentStatus(ctx context.Context, docID uint, status, note string) error {
	return wrap("update kyc document status", r.q.UpdateKYCDocumentStatus(ctx, sqlc.UpdateKYCDocumentStatusParams{ID: int64(docID), Status: status, AdminNote: note}))
}

func (r *KYCRepo) FindDocumentByUserAndType(ctx context.Context, userID uint, docType string) (*domain.KYCDocument, error) {
	row, err := r.q.GetKYCDocumentByUserAndType(ctx, sqlc.GetKYCDocumentByUserAndTypeParams{UserID: int64(userID), DocType: docType})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("postgres: get kyc document: %w", err)
	}
	d := kycDocToDomain(row)
	return &d, nil
}

func (r *KYCRepo) FindPendingUsers(ctx context.Context, page, size int) ([]domain.User, int64, error) {
	rows, err := r.q.ListPendingKYCUsers(ctx, sqlc.ListPendingKYCUsersParams{Lim: int32(size), Off: int32((page - 1) * size)})
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: list pending kyc users: %w", err)
	}
	total, err := r.q.CountPendingKYCUsers(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: count pending kyc users: %w", err)
	}
	return usersToDomain(rows), total, nil
}

func (r *KYCRepo) UpdateAllDocumentsStatus(ctx context.Context, userID uint, status, note string) error {
	return wrap("update all kyc documents", r.q.UpdateAllKYCDocumentsStatus(ctx, sqlc.UpdateAllKYCDocumentsStatusParams{UserID: int64(userID), Status: status, AdminNote: note}))
}

func kycProfileToDomain(m sqlc.KycProfile) domain.KYCProfile {
	return domain.KYCProfile{
		ID: uint(m.ID), UserID: uint(m.UserID), FirstName: m.FirstName, LastName: m.LastName,
		DateOfBirth: m.DateOfBirth, Phone: m.Phone, Address: m.Address, Ward: m.Ward, District: m.District,
		City: m.City, PostalCode: m.PostalCode, Country: m.Country, Occupation: m.Occupation,
		Income: m.Income, TradingExp: m.TradingExp, Purpose: m.Purpose, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func kycDocToDomain(m sqlc.KycDocument) domain.KYCDocument {
	return domain.KYCDocument{
		ID: uint(m.ID), UserID: uint(m.UserID), DocType: m.DocType, FilePath: m.FilePath,
		Status: m.Status, AdminNote: m.AdminNote, CreatedAt: m.CreatedAt,
	}
}
