package auth

import (
	"context"
	"time"
)

type ServiceConfig struct {
	JWTAccessSecret   []byte
	JWTMFASecret      []byte
	TOTPEncryptionKey []byte
	RecoveryPepper    string
	AccessTokenTTL    time.Duration
	RefreshTokenTTL   time.Duration
	MFAPendingTTL     time.Duration
}

type Service struct {
	users    UserRepository
	roles    RoleRepository
	recovery RecoveryCodeRepository
	refresh  RefreshTokenRepository
	cfg      ServiceConfig
}

func NewService(users UserRepository, roles RoleRepository, recovery RecoveryCodeRepository,
	refresh RefreshTokenRepository, cfg ServiceConfig) *Service {
	return &Service{users: users, roles: roles, recovery: recovery, refresh: refresh, cfg: cfg}
}

type LoginResult struct {
	Stage        string `json:"stage"` // "mfa_required" | "authenticated"
	MFAToken     string `json:"mfa_token,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

type UserProfile struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	RoleID      int16     `json:"role_id"`
	RoleName    string    `json:"role_name"`
	TOTPEnabled bool      `json:"totp_enabled"`
	CreatedAt   time.Time `json:"created_at"`
}

type SessionMeta struct {
	UserAgent string
	IP        string
}

func (s *Service) Register(ctx context.Context, email, password, roleName string) (*User, error) {
	role, err := s.roles.GetByName(ctx, roleName)
	if err != nil {
		return nil, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	u := &User{Email: email, PasswordHash: hash, RoleID: role.ID, IsActive: true}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Service) GetProfile(ctx context.Context, userID string) (*UserProfile, error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	role, err := s.roles.GetByID(ctx, u.RoleID)
	roleName := "operations"
	if err == nil && role != nil {
		roleName = role.Name
	}
	return &UserProfile{
		ID:          u.ID,
		Email:       u.Email,
		RoleID:      u.RoleID,
		RoleName:    roleName,
		TOTPEnabled: u.TOTPEnabled,
		CreatedAt:   u.CreatedAt,
	}, nil
}

func (s *Service) Login(ctx context.Context, email, password string, meta SessionMeta) (*LoginResult, error) {
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		// Same error whether the email doesn't exist or the password is wrong,
		// so a caller can't use this endpoint to enumerate registered emails.
		return nil, ErrInvalidCredentials
	}
	if !u.IsActive {
		return nil, ErrUserInactive
	}
	ok, err := VerifyPassword(u.PasswordHash, password)
	if err != nil || !ok {
		return nil, ErrInvalidCredentials
	}

	if u.TOTPEnabled {
		mfaToken, err := IssueMFAPendingToken(s.cfg.JWTMFASecret, u.ID, s.cfg.MFAPendingTTL)
		if err != nil {
			return nil, err
		}
		return &LoginResult{Stage: "mfa_required", MFAToken: mfaToken}, nil
	}

	return s.issueSession(ctx, u, meta)
}

func (s *Service) VerifyTOTP(ctx context.Context, mfaToken, code string, meta SessionMeta) (*LoginResult, error) {
	claims, err := ParseMFAPendingToken(s.cfg.JWTMFASecret, mfaToken)
	if err != nil {
		return nil, ErrInvalidToken
	}
	u, err := s.users.GetByID(ctx, claims.UserID())
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if !u.TOTPEnabled || u.TOTPSecretEnc == nil {
		return nil, ErrTOTPNotEnrolled
	}

	secret, err := DecryptSecret(s.cfg.TOTPEncryptionKey, *u.TOTPSecretEnc)
	if err != nil {
		return nil, err
	}

	if ValidateTOTPCode(secret, code) {
		return s.issueSession(ctx, u, meta)
	}
	if s.tryConsumeRecoveryCode(ctx, u.ID, code) {
		return s.issueSession(ctx, u, meta)
	}
	return nil, ErrInvalidTOTPCode
}

func (s *Service) tryConsumeRecoveryCode(ctx context.Context, userID, code string) bool {
	codes, err := s.recovery.FindUnusedByUser(ctx, userID)
	if err != nil {
		return false
	}
	for _, c := range codes {
		if VerifyRecoveryCode(s.cfg.RecoveryPepper, code, c.CodeHash) {
			_ = s.recovery.MarkUsed(ctx, c.ID)
			return true
		}
	}
	return false
}

func (s *Service) EnrollTOTP(ctx context.Context, userID string) (secret, otpauthURL string, err error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return "", "", err
	}
	if u.TOTPEnabled {
		return "", "", ErrTOTPAlreadyEnabled
	}

	key, err := GenerateTOTPSecret(u.Email)
	if err != nil {
		return "", "", err
	}
	encSecret, err := EncryptSecret(s.cfg.TOTPEncryptionKey, key.Secret())
	if err != nil {
		return "", "", err
	}
	if err := s.users.UpdateTOTPSecret(ctx, userID, encSecret); err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

func (s *Service) ConfirmTOTP(ctx context.Context, userID, code string) ([]string, error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if u.TOTPSecretEnc == nil {
		return nil, ErrTOTPNotEnrolled
	}
	secret, err := DecryptSecret(s.cfg.TOTPEncryptionKey, *u.TOTPSecretEnc)
	if err != nil {
		return nil, err
	}
	if !ValidateTOTPCode(secret, code) {
		return nil, ErrInvalidTOTPCode
	}
	if err := s.users.ConfirmTOTP(ctx, userID, time.Now()); err != nil {
		return nil, err
	}
	return s.regenerateRecoveryCodes(ctx, userID)
}

func (s *Service) RegenerateRecoveryCodes(ctx context.Context, userID string) ([]string, error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !u.TOTPEnabled {
		return nil, ErrTOTPNotEnrolled
	}
	return s.regenerateRecoveryCodes(ctx, userID)
}

func (s *Service) regenerateRecoveryCodes(ctx context.Context, userID string) ([]string, error) {
	rawCodes := make([]string, 0, recoveryCodeCount)
	hashes := make([]string, 0, recoveryCodeCount)
	for i := 0; i < recoveryCodeCount; i++ {
		code, err := GenerateRecoveryCode()
		if err != nil {
			return nil, err
		}
		rawCodes = append(rawCodes, code)
		hashes = append(hashes, HashRecoveryCode(s.cfg.RecoveryPepper, code))
	}
	if err := s.recovery.ReplaceAll(ctx, userID, hashes); err != nil {
		return nil, err
	}
	return rawCodes, nil
}

func (s *Service) issueSession(ctx context.Context, u *User, meta SessionMeta) (*LoginResult, error) {
	access, err := IssueAccessToken(s.cfg.JWTAccessSecret, u.ID, u.RoleID, s.cfg.AccessTokenTTL)
	if err != nil {
		return nil, err
	}
	raw, hash, err := GenerateOpaqueToken()
	if err != nil {
		return nil, err
	}
	rt := &RefreshToken{
		UserID:    u.ID,
		TokenHash: hash,
		ExpiresAt: time.Now().Add(s.cfg.RefreshTokenTTL),
	}
	if meta.UserAgent != "" {
		rt.UserAgent = &meta.UserAgent
	}
	if meta.IP != "" {
		rt.IP = &meta.IP
	}
	if err := s.refresh.Create(ctx, rt); err != nil {
		return nil, err
	}
	return &LoginResult{Stage: "authenticated", AccessToken: access, RefreshToken: raw}, nil
}

// RefreshSession rotates the refresh token on every use. If a token that was
// already rotated/revoked is presented again, it's treated as theft: every
// session for that user is revoked and re-login is required.
func (s *Service) RefreshSession(ctx context.Context, rawToken string, meta SessionMeta) (*LoginResult, error) {
	hash := HashOpaqueToken(rawToken)
	rt, err := s.refresh.GetByHash(ctx, hash)
	if err != nil {
		return nil, ErrInvalidRefreshToken
	}
	if rt.RevokedAt != nil {
		_ = s.refresh.RevokeAllForUser(ctx, rt.UserID)
		return nil, ErrSessionCompromised
	}
	if time.Now().After(rt.ExpiresAt) {
		return nil, ErrRefreshTokenExpired
	}

	u, err := s.users.GetByID(ctx, rt.UserID)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if !u.IsActive {
		return nil, ErrUserInactive
	}

	access, err := IssueAccessToken(s.cfg.JWTAccessSecret, u.ID, u.RoleID, s.cfg.AccessTokenTTL)
	if err != nil {
		return nil, err
	}
	rawNew, hashNew, err := GenerateOpaqueToken()
	if err != nil {
		return nil, err
	}
	newRT := &RefreshToken{
		UserID:    u.ID,
		TokenHash: hashNew,
		ExpiresAt: time.Now().Add(s.cfg.RefreshTokenTTL),
	}
	if meta.UserAgent != "" {
		newRT.UserAgent = &meta.UserAgent
	}
	if meta.IP != "" {
		newRT.IP = &meta.IP
	}
	if err := s.refresh.Create(ctx, newRT); err != nil {
		return nil, err
	}
	if err := s.refresh.MarkRotated(ctx, rt.ID, newRT.ID); err != nil {
		return nil, err
	}

	return &LoginResult{Stage: "authenticated", AccessToken: access, RefreshToken: rawNew}, nil
}

func (s *Service) Logout(ctx context.Context, rawToken string) error {
	hash := HashOpaqueToken(rawToken)
	rt, err := s.refresh.GetByHash(ctx, hash)
	if err != nil {
		return nil // idempotent: unknown token is already "logged out"
	}
	return s.refresh.Revoke(ctx, rt.ID)
}

func (s *Service) LogoutAll(ctx context.Context, userID string) error {
	return s.refresh.RevokeAllForUser(ctx, userID)
}
