package api

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/WangYihang/Platypus/internal/user"
)

// TokenIssuer signs and verifies both access and refresh JWTs with a shared
// HS256 scheme. Access tokens are short-lived and carry identity; refresh
// tokens are longer-lived and carry only the jti + user id so that
// refresh_tokens row in storage can be looked up for revocation.
//
// TTLs have two sources: the constructor-baked fallback (used by tests and
// pre-registry callers) and, optionally, per-issue provider funcs set via
// SetTTLProviders. When providers are installed (wired to the settings
// registry in production) admin edits to the TTL take effect on the very
// next Issue call — no restart, no re-signing of existing tokens.
type TokenIssuer struct {
	accessKey    []byte
	refreshKey   []byte
	accessTTL    time.Duration
	refreshTTL   time.Duration
	accessTTLFn  func() time.Duration // optional: consulted per IssueAccess
	refreshTTLFn func() time.Duration // optional: consulted per IssueRefresh
	now          func() time.Time     // swappable for tests later
}

// AccessClaims is the application-level claim set embedded in an access JWT.
// It omits anything the server can look up from the user id.
type AccessClaims struct {
	UserID   string
	Username string
	Role     user.Role
}

// RefreshClaims is the application-level claim set embedded in a refresh
// JWT. The jti + user id let us delete the matching refresh_tokens row on
// logout or password change, invalidating outstanding refresh tokens.
type RefreshClaims struct {
	UserID  string
	TokenID string // matches refresh_tokens.id
}

// typedClaims is the full JWT claims body — standard fields + our own.
// `audience` discriminates access from refresh so a token of one flavour
// cannot be parsed as the other.
type typedClaims struct {
	jwt.RegisteredClaims
	Username string    `json:"username,omitempty"`
	Role     user.Role `json:"role,omitempty"`
	TokenID  string    `json:"jti,omitempty"`
}

const (
	audAccess  = "access"
	audRefresh = "refresh"
)

// NewTokenIssuer validates inputs and returns a ready-to-use issuer.
func NewTokenIssuer(accessKey, refreshKey string, accessTTL, refreshTTL time.Duration) (*TokenIssuer, error) {
	if accessKey == "" {
		return nil, errors.New("access key must not be empty")
	}
	if refreshKey == "" {
		return nil, errors.New("refresh key must not be empty")
	}
	return &TokenIssuer{
		accessKey:  []byte(accessKey),
		refreshKey: []byte(refreshKey),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		now:        time.Now,
	}, nil
}

// SetTTLProviders installs functions consulted on every IssueAccess /
// IssueRefresh. Typically wired to settings.Registry.AccessTokenTTL /
// RefreshTokenTTL so admin edits take effect immediately.
func (i *TokenIssuer) SetTTLProviders(access, refresh func() time.Duration) {
	i.accessTTLFn = access
	i.refreshTTLFn = refresh
}

func (i *TokenIssuer) currentAccessTTL() time.Duration {
	if i.accessTTLFn != nil {
		if d := i.accessTTLFn(); d > 0 {
			return d
		}
	}
	return i.accessTTL
}

func (i *TokenIssuer) currentRefreshTTL() time.Duration {
	if i.refreshTTLFn != nil {
		if d := i.refreshTTLFn(); d > 0 {
			return d
		}
	}
	return i.refreshTTL
}

func (i *TokenIssuer) IssueAccess(c AccessClaims) (string, error) {
	now := i.now().UTC()
	claims := typedClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   c.UserID,
			Audience:  jwt.ClaimStrings{audAccess},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(i.currentAccessTTL())),
			ID:        uuid.NewString(),
		},
		Username: c.Username,
		Role:     c.Role,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(i.accessKey)
}

func (i *TokenIssuer) IssueRefresh(c RefreshClaims) (string, error) {
	now := i.now().UTC()
	claims := typedClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   c.UserID,
			Audience:  jwt.ClaimStrings{audRefresh},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(i.currentRefreshTTL())),
			ID:        c.TokenID,
		},
		TokenID: c.TokenID,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(i.refreshKey)
}

func (i *TokenIssuer) ParseAccess(tok string) (AccessClaims, error) {
	c, err := i.parse(tok, i.accessKey, audAccess)
	if err != nil {
		return AccessClaims{}, err
	}
	return AccessClaims{
		UserID:   c.Subject,
		Username: c.Username,
		Role:     c.Role,
	}, nil
}

func (i *TokenIssuer) ParseRefresh(tok string) (RefreshClaims, error) {
	c, err := i.parse(tok, i.refreshKey, audRefresh)
	if err != nil {
		return RefreshClaims{}, err
	}
	return RefreshClaims{
		UserID:  c.Subject,
		TokenID: c.TokenID,
	}, nil
}

func (i *TokenIssuer) parse(tok string, key []byte, wantAud string) (*typedClaims, error) {
	parsed, err := jwt.ParseWithClaims(tok, &typedClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return key, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*typedClaims)
	if !ok || !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	if len(claims.Audience) == 0 || claims.Audience[0] != wantAud {
		return nil, fmt.Errorf("unexpected audience: %v", claims.Audience)
	}
	return claims, nil
}
