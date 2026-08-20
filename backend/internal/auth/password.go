package auth

import "golang.org/x/crypto/bcrypt"

// minPasswordLength is the only password rule the assignment implicitly
// needs — a floor, not a complexity policy. No uppercase/digit/symbol
// requirements: the assignment specifies none, and inventing them would
// add friction without a stated reason.
const minPasswordLength = 8

// HashPassword bcrypt-hashes a plaintext password with a fresh random
// salt (bcrypt.GenerateFromPassword salts automatically — two calls with
// the same input produce different hashes).
func HashPassword(plaintext string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword returns nil if plaintext matches the given bcrypt hash,
// or a non-nil error otherwise (including bcrypt.ErrMismatchedHashAndPassword
// for a simple wrong password — callers should treat any error the same
// way: reject the login).
func VerifyPassword(hash, plaintext string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext))
}
