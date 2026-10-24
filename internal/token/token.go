// Package token issues and verifies the HS256 JWTs used for authentication.
package token

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Kind distinguishes a short-lived access token from a long-lived refresh
// token. It is embedded in the claims so a refresh token can never be replayed
// as an access token (or vice versa).
type Kind string

const (
	KindAccess  Kind = "access"
	KindRefresh Kind = "refresh"
)

// ErrInvalidToken covers every verification failure: bad signature, expiry,
// wrong issuer, or an unexpected token kind.
var ErrInvalidToken = errors.New("invalid or expired token")

// Claims is the JWT payload. Subject carries the user ID.
type Claims struct {
	jwt.RegisteredClaims
	Kind  Kind   `json:"kind"`
	Email string `json:"email,omitempty"`
}

// Pair is what the login and refresh endpoints hand back to a client.
type Pair struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int64  `json:"expires_in"`
	RefreshExpiresIn int64  `json:"refresh_expires_in"`
}

// Manager signs and verifies tokens for a single secret and issuer.
type Manager struct {
	secret     []byte
	issuer     string
	accessTTL  time.Duration
	refreshTTL time.Duration
	now        func() time.Time
}

// NewManager builds a Manager. The secret must be non-empty.
func NewManager(secret []byte, issuer string, accessTTL, refreshTTL time.Duration) *Manager {
	return &Manager{
		secret:     secret,
		issuer:     issuer,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		now:        time.Now,
	}
}

// NewPair issues a fresh access/refresh pair for a user.
func (m *Manager) NewPair(userID, email string) (Pair, error) {
	access, err := m.sign(userID, email, KindAccess, m.accessTTL)
	if err != nil {
		return Pair{}, err
	}
	refresh, err := m.sign(userID, email, KindRefresh, m.refreshTTL)
	if err != nil {
		return Pair{}, err
	}
	return Pair{
		AccessToken:      access,
		RefreshToken:     refresh,
		TokenType:        "Bearer",
		ExpiresIn:        int64(m.accessTTL.Seconds()),
		RefreshExpiresIn: int64(m.refreshTTL.Seconds()),
	}, nil
}

// Verify parses a token and checks the signature, issuer, expiry, and kind.
// Any failure collapses into ErrInvalidToken so callers cannot accidentally
// leak the specific reason to a client.
func (m *Manager) Verify(raw string, want Kind) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(
		raw,
		&Claims{},
		func(*jwt.Token) (any, error) { return m.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(m.issuer),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(m.now),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	claims, ok := parsed.Claims.(*Claims)
	if !ok || claims.Kind != want || claims.Subject == "" {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

func (m *Manager) sign(userID, email string, kind Kind, ttl time.Duration) (string, error) {
	jti, err := randomID()
	if err != nil {
		return "", err
	}
	now := m.now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Subject:   userID,
			Issuer:    m.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
		Kind:  kind,
		Email: email,
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("sign %s token: %w", kind, err)
	}
	return signed, nil
}

func randomID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// NewUserID returns a random identifier suitable for a user primary key.
func NewUserID() (string, error) { return randomID() }
