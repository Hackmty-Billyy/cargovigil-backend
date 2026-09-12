package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/hex"
)

const recoveryCodeCount = 10

// GenerateRecoveryCode produces an 80-bit random backup code. Entropy is high
// enough that a fast hash (not argon2) is sufficient for storage, unlike
// user-chosen passwords which need a slow, memory-hard hash to resist offline
// cracking.
func GenerateRecoveryCode() (string, error) {
	buf := make([]byte, 10)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf), nil
}

func HashRecoveryCode(pepper, code string) string {
	sum := sha256.Sum256([]byte(pepper + code))
	return hex.EncodeToString(sum[:])
}

func VerifyRecoveryCode(pepper, code, hash string) bool {
	computed := HashRecoveryCode(pepper, code)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(hash)) == 1
}
