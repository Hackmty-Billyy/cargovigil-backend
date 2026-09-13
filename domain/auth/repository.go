package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/uptrace/bun"
)

const pgUniqueViolation = "23505"

type RoleRepository interface {
	GetByName(ctx context.Context, name string) (*Role, error)
	GetByID(ctx context.Context, id int16) (*Role, error)
}

type UserRepository interface {
	Create(ctx context.Context, u *User) error
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByID(ctx context.Context, id string) (*User, error)
	UpdateTOTPSecret(ctx context.Context, userID, secretEnc string) error
	ConfirmTOTP(ctx context.Context, userID string, confirmedAt time.Time) error
}

type RecoveryCodeRepository interface {
	ReplaceAll(ctx context.Context, userID string, hashes []string) error
	FindUnusedByUser(ctx context.Context, userID string) ([]RecoveryCode, error)
	MarkUsed(ctx context.Context, id string) error
}

type RefreshTokenRepository interface {
	Create(ctx context.Context, rt *RefreshToken) error
	GetByHash(ctx context.Context, hash string) (*RefreshToken, error)
	MarkRotated(ctx context.Context, id, replacedByID string) error
	Revoke(ctx context.Context, id string) error
	RevokeAllForUser(ctx context.Context, userID string) error
	DeleteExpiredBefore(ctx context.Context, before time.Time) (int64, error)
}

// ---- Bun implementations ----

type BunRoleRepository struct{ db *bun.DB }

func NewBunRoleRepository(db *bun.DB) *BunRoleRepository {
	return &BunRoleRepository{db: db}
}

func (r *BunRoleRepository) GetByName(ctx context.Context, name string) (*Role, error) {
	role := new(Role)
	err := r.db.NewSelect().Model(role).Where("name = ?", name).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return role, nil
}

func (r *BunRoleRepository) GetByID(ctx context.Context, id int16) (*Role, error) {
	role := new(Role)
	err := r.db.NewSelect().Model(role).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return role, nil
}

type BunUserRepository struct{ db *bun.DB }

func NewBunUserRepository(db *bun.DB) *BunUserRepository {
	return &BunUserRepository{db: db}
}

func (r *BunUserRepository) Create(ctx context.Context, u *User) error {
	u.IsActive = true
	_, err := r.db.NewInsert().Model(u).
		Column("company_id", "email", "password_hash", "role_id", "is_active").
		Returning("id, created_at, updated_at").
		Exec(ctx)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return ErrEmailAlreadyExists
		}
		return err
	}
	return nil
}

func (r *BunUserRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
	u := new(User)
	err := r.db.NewSelect().Model(u).Where("email = ?", email).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *BunUserRepository) GetByID(ctx context.Context, id string) (*User, error) {
	u := new(User)
	err := r.db.NewSelect().Model(u).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *BunUserRepository) UpdateTOTPSecret(ctx context.Context, userID, secretEnc string) error {
	_, err := r.db.NewUpdate().Model((*User)(nil)).
		Set("totp_secret_enc = ?", secretEnc).
		Set("updated_at = now()").
		Where("id = ?", userID).
		Exec(ctx)
	return err
}

func (r *BunUserRepository) ConfirmTOTP(ctx context.Context, userID string, confirmedAt time.Time) error {
	_, err := r.db.NewUpdate().Model((*User)(nil)).
		Set("totp_enabled = true").
		Set("totp_confirmed_at = ?", confirmedAt).
		Set("updated_at = now()").
		Where("id = ?", userID).
		Exec(ctx)
	return err
}

type BunRecoveryCodeRepository struct{ db *bun.DB }

func NewBunRecoveryCodeRepository(db *bun.DB) *BunRecoveryCodeRepository {
	return &BunRecoveryCodeRepository{db: db}
}

func (r *BunRecoveryCodeRepository) ReplaceAll(ctx context.Context, userID string, hashes []string) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewDelete().Model((*RecoveryCode)(nil)).Where("user_id = ?", userID).Exec(ctx); err != nil {
			return err
		}
		if len(hashes) == 0 {
			return nil
		}
		codes := make([]RecoveryCode, len(hashes))
		for i, h := range hashes {
			codes[i] = RecoveryCode{UserID: userID, CodeHash: h}
		}
		_, err := tx.NewInsert().Model(&codes).Exec(ctx)
		return err
	})
}

func (r *BunRecoveryCodeRepository) FindUnusedByUser(ctx context.Context, userID string) ([]RecoveryCode, error) {
	var codes []RecoveryCode
	err := r.db.NewSelect().Model(&codes).
		Where("user_id = ?", userID).
		Where("used_at IS NULL").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return codes, nil
}

func (r *BunRecoveryCodeRepository) MarkUsed(ctx context.Context, id string) error {
	_, err := r.db.NewUpdate().Model((*RecoveryCode)(nil)).
		Set("used_at = now()").
		Where("id = ?", id).
		Exec(ctx)
	return err
}

type BunRefreshTokenRepository struct{ db *bun.DB }

func NewBunRefreshTokenRepository(db *bun.DB) *BunRefreshTokenRepository {
	return &BunRefreshTokenRepository{db: db}
}

func (r *BunRefreshTokenRepository) Create(ctx context.Context, rt *RefreshToken) error {
	_, err := r.db.NewInsert().Model(rt).
		Column("user_id", "token_hash", "expires_at", "user_agent", "ip").
		Returning("id, created_at").
		Exec(ctx)
	return err
}

func (r *BunRefreshTokenRepository) GetByHash(ctx context.Context, hash string) (*RefreshToken, error) {
	rt := new(RefreshToken)
	err := r.db.NewSelect().Model(rt).Where("token_hash = ?", hash).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return rt, nil
}

func (r *BunRefreshTokenRepository) MarkRotated(ctx context.Context, id, replacedByID string) error {
	_, err := r.db.NewUpdate().Model((*RefreshToken)(nil)).
		Set("revoked_at = now()").
		Set("replaced_by_id = ?", replacedByID).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (r *BunRefreshTokenRepository) Revoke(ctx context.Context, id string) error {
	_, err := r.db.NewUpdate().Model((*RefreshToken)(nil)).
		Set("revoked_at = now()").
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (r *BunRefreshTokenRepository) RevokeAllForUser(ctx context.Context, userID string) error {
	_, err := r.db.NewUpdate().Model((*RefreshToken)(nil)).
		Set("revoked_at = now()").
		Where("user_id = ?", userID).
		Where("revoked_at IS NULL").
		Exec(ctx)
	return err
}

func (r *BunRefreshTokenRepository) DeleteExpiredBefore(ctx context.Context, before time.Time) (int64, error) {
	res, err := r.db.NewDelete().Model((*RefreshToken)(nil)).
		Where("expires_at < ?", before).
		WhereOr("revoked_at IS NOT NULL").
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
