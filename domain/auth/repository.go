package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
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

// ---- Postgres implementations ----

type PostgresRoleRepository struct{ db *pgxpool.Pool }

func NewPostgresRoleRepository(db *pgxpool.Pool) *PostgresRoleRepository {
	return &PostgresRoleRepository{db: db}
}

func (r *PostgresRoleRepository) GetByName(ctx context.Context, name string) (*Role, error) {
	role := &Role{}
	err := r.db.QueryRow(ctx, `SELECT id, name FROM roles WHERE name = $1`, name).
		Scan(&role.ID, &role.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return role, nil
}

func (r *PostgresRoleRepository) GetByID(ctx context.Context, id int16) (*Role, error) {
	role := &Role{}
	err := r.db.QueryRow(ctx, `SELECT id, name FROM roles WHERE id = $1`, id).
		Scan(&role.ID, &role.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return role, nil
}

type PostgresUserRepository struct{ db *pgxpool.Pool }

func NewPostgresUserRepository(db *pgxpool.Pool) *PostgresUserRepository {
	return &PostgresUserRepository{db: db}
}

const userColumns = `id, company_id, email, password_hash, role_id, is_active, totp_enabled,
	totp_secret_enc, totp_confirmed_at, created_at, updated_at`

func scanUser(row pgx.Row) (*User, error) {
	u := &User{}
	err := row.Scan(&u.ID, &u.CompanyID, &u.Email, &u.PasswordHash, &u.RoleID, &u.IsActive, &u.TOTPEnabled,
		&u.TOTPSecretEnc, &u.TOTPConfirmedAt, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *PostgresUserRepository) Create(ctx context.Context, u *User) error {
	err := r.db.QueryRow(ctx,
		`INSERT INTO users (company_id, email, password_hash, role_id, is_active)
		 VALUES ($1, $2, $3, $4, true)
		 RETURNING id, created_at, updated_at`,
		u.CompanyID, u.Email, u.PasswordHash, u.RoleID,
	).Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return ErrEmailAlreadyExists
		}
		return err
	}
	return nil
}

func (r *PostgresUserRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
	row := r.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE email = $1`, email)
	return scanUser(row)
}

func (r *PostgresUserRepository) GetByID(ctx context.Context, id string) (*User, error) {
	row := r.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id)
	return scanUser(row)
}

func (r *PostgresUserRepository) UpdateTOTPSecret(ctx context.Context, userID, secretEnc string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE users SET totp_secret_enc = $1, updated_at = now() WHERE id = $2`,
		secretEnc, userID)
	return err
}

func (r *PostgresUserRepository) ConfirmTOTP(ctx context.Context, userID string, confirmedAt time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE users SET totp_enabled = true, totp_confirmed_at = $1, updated_at = now() WHERE id = $2`,
		confirmedAt, userID)
	return err
}

type PostgresRecoveryCodeRepository struct{ db *pgxpool.Pool }

func NewPostgresRecoveryCodeRepository(db *pgxpool.Pool) *PostgresRecoveryCodeRepository {
	return &PostgresRecoveryCodeRepository{db: db}
}

func (r *PostgresRecoveryCodeRepository) ReplaceAll(ctx context.Context, userID string, hashes []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM user_recovery_codes WHERE user_id = $1`, userID); err != nil {
		return err
	}
	for _, h := range hashes {
		if _, err := tx.Exec(ctx,
			`INSERT INTO user_recovery_codes (user_id, code_hash) VALUES ($1, $2)`, userID, h); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *PostgresRecoveryCodeRepository) FindUnusedByUser(ctx context.Context, userID string) ([]RecoveryCode, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, user_id, code_hash, used_at, created_at
		 FROM user_recovery_codes WHERE user_id = $1 AND used_at IS NULL`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var codes []RecoveryCode
	for rows.Next() {
		var c RecoveryCode
		if err := rows.Scan(&c.ID, &c.UserID, &c.CodeHash, &c.UsedAt, &c.CreatedAt); err != nil {
			return nil, err
		}
		codes = append(codes, c)
	}
	return codes, rows.Err()
}

func (r *PostgresRecoveryCodeRepository) MarkUsed(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `UPDATE user_recovery_codes SET used_at = now() WHERE id = $1`, id)
	return err
}

type PostgresRefreshTokenRepository struct{ db *pgxpool.Pool }

func NewPostgresRefreshTokenRepository(db *pgxpool.Pool) *PostgresRefreshTokenRepository {
	return &PostgresRefreshTokenRepository{db: db}
}

func (r *PostgresRefreshTokenRepository) Create(ctx context.Context, rt *RefreshToken) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO refresh_tokens (user_id, token_hash, expires_at, user_agent, ip)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, created_at`,
		rt.UserID, rt.TokenHash, rt.ExpiresAt, rt.UserAgent, rt.IP,
	).Scan(&rt.ID, &rt.CreatedAt)
}

func (r *PostgresRefreshTokenRepository) GetByHash(ctx context.Context, hash string) (*RefreshToken, error) {
	rt := &RefreshToken{}
	err := r.db.QueryRow(ctx,
		`SELECT id, user_id, token_hash, replaced_by_id, expires_at, revoked_at, user_agent, ip, created_at
		 FROM refresh_tokens WHERE token_hash = $1`, hash,
	).Scan(&rt.ID, &rt.UserID, &rt.TokenHash, &rt.ReplacedByID, &rt.ExpiresAt, &rt.RevokedAt,
		&rt.UserAgent, &rt.IP, &rt.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return rt, nil
}

func (r *PostgresRefreshTokenRepository) MarkRotated(ctx context.Context, id, replacedByID string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now(), replaced_by_id = $1 WHERE id = $2`,
		replacedByID, id)
	return err
}

func (r *PostgresRefreshTokenRepository) Revoke(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = now() WHERE id = $1`, id)
	return err
}

func (r *PostgresRefreshTokenRepository) RevokeAllForUser(ctx context.Context, userID string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	return err
}

func (r *PostgresRefreshTokenRepository) DeleteExpiredBefore(ctx context.Context, before time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM refresh_tokens WHERE expires_at < $1 OR revoked_at IS NOT NULL`, before)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
