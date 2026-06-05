-- name: CreateKYCProfile :one
INSERT INTO kyc_profiles (user_id, first_name, last_name, date_of_birth, phone, address, ward, district, city, postal_code, country, occupation, income, trading_exp, purpose)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING id, created_at, updated_at;

-- name: GetKYCProfileByUser :one
SELECT id, user_id, first_name, last_name, date_of_birth, phone, address, ward, district, city, postal_code, country, occupation, income, trading_exp, purpose, created_at, updated_at
FROM kyc_profiles WHERE user_id = $1;

-- name: UpdateKYCProfile :exec
UPDATE kyc_profiles SET
    first_name = $2, last_name = $3, date_of_birth = $4, phone = $5, address = $6, ward = $7,
    district = $8, city = $9, postal_code = $10, country = $11, occupation = $12, income = $13,
    trading_exp = $14, purpose = $15, updated_at = now()
WHERE id = $1;

-- name: CreateKYCDocument :one
INSERT INTO kyc_documents (user_id, doc_type, file_path, status, admin_note)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, created_at;

-- name: ListKYCDocumentsByUser :many
SELECT id, user_id, doc_type, file_path, status, admin_note, created_at
FROM kyc_documents WHERE user_id = $1;

-- name: GetKYCDocumentByUserAndType :one
SELECT id, user_id, doc_type, file_path, status, admin_note, created_at
FROM kyc_documents WHERE user_id = $1 AND doc_type = $2;

-- name: UpdateKYCDocumentStatus :exec
UPDATE kyc_documents SET status = $2, admin_note = $3 WHERE id = $1;

-- name: UpdateAllKYCDocumentsStatus :exec
UPDATE kyc_documents SET status = $2, admin_note = $3 WHERE user_id = $1;
