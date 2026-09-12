package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type AccessClaims struct {
	RoleID    int16  `json:"role"`
	CompanyID string `json:"company_id"`
	jwt.RegisteredClaims
}

func (c AccessClaims) UserID() string { return c.Subject }

type MFAPendingClaims struct {
	jwt.RegisteredClaims
}

func (c MFAPendingClaims) UserID() string { return c.Subject }

// IssueAccessToken signs a short-lived JWT proving a completed login
// (password + TOTP when enabled). Uses its own signing secret, separate
// from the MFA-pending token, so a bug in one verifier can't escalate the
// other token type into a full session.
func IssueAccessToken(secret []byte, userID, companyID string, roleID int16, ttl time.Duration) (string, error) {
	claims := AccessClaims{
		RoleID:    roleID,
		CompanyID: companyID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

func ParseAccessToken(secret []byte, tokenStr string) (*AccessClaims, error) {
	claims := &AccessClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return secret, nil
	})
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// IssueMFAPendingToken signs a very short-lived token proving password-step
// success while a TOTP code is still required. It is stateless by design:
// no DB row per login attempt, no cleanup job. It can't be revoked before
// expiry, but it only allows submitting a TOTP code, never direct access.
func IssueMFAPendingToken(secret []byte, userID string, ttl time.Duration) (string, error) {
	claims := MFAPendingClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

func ParseMFAPendingToken(secret []byte, tokenStr string) (*MFAPendingClaims, error) {
	claims := &MFAPendingClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return secret, nil
	})
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// GenerateOpaqueToken creates a refresh token: a random string handed to the
// client once, with only its SHA-256 hash persisted server-side. Unlike the
// access token, it must be revocable, so it is not a JWT.
func GenerateOpaqueToken() (raw string, hash string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	hash = HashOpaqueToken(raw)
	return raw, hash, nil
}

func HashOpaqueToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
