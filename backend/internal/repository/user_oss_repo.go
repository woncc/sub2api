package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type userOSSRepository struct {
	db *sql.DB
}

// NewUserOSSRepository stores per-user object storage repositories.
func NewUserOSSRepository(db *sql.DB) service.UserOSSRepository {
	return &userOSSRepository{db: db}
}

const userOSSSelectColumns = `
	id, user_id, provider, bucket, domain, region, endpoint, access_key_id,
	secret_encrypted, account_id, force_path_style, bucket_url, created_at, updated_at`

func (r *userOSSRepository) ListByUser(ctx context.Context, userID int64) ([]*service.UserOSSRecord, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("user oss repository is unavailable")
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+userOSSSelectColumns+`
		FROM user_oss_repositories
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY id DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list user oss repositories: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]*service.UserOSSRecord, 0)
	for rows.Next() {
		rec, err := scanUserOSS(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *userOSSRepository) GetByUser(ctx context.Context, userID, id int64) (*service.UserOSSRecord, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("user oss repository is unavailable")
	}
	row := r.db.QueryRowContext(ctx, `
		SELECT `+userOSSSelectColumns+`
		FROM user_oss_repositories
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`, id, userID)
	rec, err := scanUserOSS(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrUserOSSNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user oss repository: %w", err)
	}
	return rec, nil
}

func (r *userOSSRepository) Create(ctx context.Context, rec *service.UserOSSRecord) (*service.UserOSSRecord, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("user oss repository is unavailable")
	}
	now := time.Now().UTC()
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO user_oss_repositories (
			user_id, provider, bucket, domain, region, endpoint, access_key_id,
			secret_encrypted, account_id, force_path_style, bucket_url, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$12)
		RETURNING `+userOSSSelectColumns,
		rec.UserID, rec.Provider, rec.Bucket, rec.Domain, rec.Region, rec.Endpoint, rec.AccessKeyID,
		rec.SecretEncrypted, rec.AccountID, rec.ForcePathStyle, rec.BucketURL, now,
	)
	created, err := scanUserOSS(row)
	if err != nil {
		return nil, fmt.Errorf("create user oss repository: %w", err)
	}
	return created, nil
}

func (r *userOSSRepository) Update(ctx context.Context, rec *service.UserOSSRecord) error {
	if r == nil || r.db == nil {
		return errors.New("user oss repository is unavailable")
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE user_oss_repositories SET
			provider = $3,
			bucket = $4,
			domain = $5,
			region = $6,
			endpoint = $7,
			access_key_id = $8,
			secret_encrypted = $9,
			account_id = $10,
			force_path_style = $11,
			bucket_url = $12,
			updated_at = $13
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		rec.ID, rec.UserID, rec.Provider, rec.Bucket, rec.Domain, rec.Region, rec.Endpoint,
		rec.AccessKeyID, rec.SecretEncrypted, rec.AccountID, rec.ForcePathStyle, rec.BucketURL,
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("update user oss repository: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrUserOSSNotFound
	}
	return nil
}

func (r *userOSSRepository) Delete(ctx context.Context, userID, id int64) error {
	if r == nil || r.db == nil {
		return errors.New("user oss repository is unavailable")
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE user_oss_repositories
		SET deleted_at = $3, updated_at = $3
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		id, userID, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("delete user oss repository: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrUserOSSNotFound
	}
	return nil
}

type userOSSScanner interface {
	Scan(dest ...any) error
}

func scanUserOSS(row userOSSScanner) (*service.UserOSSRecord, error) {
	var rec service.UserOSSRecord
	err := row.Scan(
		&rec.ID, &rec.UserID, &rec.Provider, &rec.Bucket, &rec.Domain, &rec.Region, &rec.Endpoint,
		&rec.AccessKeyID, &rec.SecretEncrypted, &rec.AccountID, &rec.ForcePathStyle, &rec.BucketURL,
		&rec.CreatedAt, &rec.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &rec, nil
}
