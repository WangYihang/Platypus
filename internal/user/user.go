// Package user models authenticated principals of the Platypus server: who
// they are, what global role they hold, and how their passwords are hashed.
// Persistence lives elsewhere (internal/storage/users.go); this package is
// the shared type + crypto primitives used by both the HTTP layer and the
// repo layer.
package user

import (
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Role names match the CHECK constraint on users.role in the initial
// migration. The zero value is deliberately invalid so a missing role in a
// response body can be detected.
type Role string

const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

// User is the in-memory form of a row in the users table.
type User struct {
	ID           string
	Username     string
	PasswordHash string
	Role         Role
	CreatedAt    time.Time
	LastLoginAt  *time.Time
}

// ParseRole normalises and validates a role string. Unknown values return an
// error so callers can return a 400 instead of deferring the failure to the
// DB CHECK constraint.
func ParseRole(s string) (Role, error) {
	switch Role(strings.ToLower(strings.TrimSpace(s))) {
	case RoleAdmin:
		return RoleAdmin, nil
	case RoleOperator:
		return RoleOperator, nil
	case RoleViewer:
		return RoleViewer, nil
	default:
		return "", errors.New("invalid role")
	}
}

// ErrEmptyPassword is returned by HashPassword when given the empty string.
// bcrypt itself accepts empty passwords, but an empty password is almost
// always a bug upstream — fail loudly at the boundary.
var ErrEmptyPassword = errors.New("password must not be empty")

// HashPassword returns a bcrypt hash usable as users.password_hash. Cost is
// bcrypt.DefaultCost which gives ~60ms per hash on modern hardware — high
// enough to resist offline brute-force, low enough for interactive login.
func HashPassword(plain string) (string, error) {
	if plain == "" {
		return "", ErrEmptyPassword
	}
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// VerifyPassword returns true iff the bcrypt hash was produced from the
// given plaintext. Constant-time under the hood (bcrypt.CompareHashAndPassword).
func VerifyPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
