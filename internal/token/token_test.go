package token_test

import (
	"errors"
	"testing"
	"time"

	"github.com/backend-pc4/restgo/internal/token"
)

func newManager(accessTTL time.Duration) *token.Manager {
	return token.NewManager([]byte("secret-value-long-enough-for-tests"), "restgo-test", accessTTL, time.Hour)
}

func TestVerifyRoundTrip(t *testing.T) {
	m := newManager(time.Minute)

	pair, err := m.NewPair("user-1", "ada@example.com")
	if err != nil {
		t.Fatalf("NewPair: %v", err)
	}

	claims, err := m.Verify(pair.AccessToken, token.KindAccess)
	if err != nil {
		t.Fatalf("Verify access: %v", err)
	}
	if claims.Subject != "user-1" {
		t.Errorf("got subject %q, want user-1", claims.Subject)
	}
	if claims.Email != "ada@example.com" {
		t.Errorf("got email %q, want ada@example.com", claims.Email)
	}

	if _, err := m.Verify(pair.RefreshToken, token.KindRefresh); err != nil {
		t.Errorf("Verify refresh: %v", err)
	}
}

func TestVerifyRejectsWrongKind(t *testing.T) {
	m := newManager(time.Minute)
	pair, err := m.NewPair("user-1", "ada@example.com")
	if err != nil {
		t.Fatalf("NewPair: %v", err)
	}

	if _, err := m.Verify(pair.RefreshToken, token.KindAccess); !errors.Is(err, token.ErrInvalidToken) {
		t.Errorf("got %v, want ErrInvalidToken", err)
	}
}

func TestVerifyRejectsExpiredToken(t *testing.T) {
	m := newManager(-time.Minute) // already expired at issue time

	pair, err := m.NewPair("user-1", "ada@example.com")
	if err != nil {
		t.Fatalf("NewPair: %v", err)
	}
	if _, err := m.Verify(pair.AccessToken, token.KindAccess); !errors.Is(err, token.ErrInvalidToken) {
		t.Errorf("got %v, want ErrInvalidToken", err)
	}
}

func TestVerifyRejectsForeignSecret(t *testing.T) {
	issuer := newManager(time.Minute)
	pair, err := issuer.NewPair("user-1", "ada@example.com")
	if err != nil {
		t.Fatalf("NewPair: %v", err)
	}

	attacker := token.NewManager([]byte("a-completely-different-secret-value"), "restgo-test", time.Minute, time.Hour)
	if _, err := attacker.Verify(pair.AccessToken, token.KindAccess); !errors.Is(err, token.ErrInvalidToken) {
		t.Errorf("got %v, want ErrInvalidToken", err)
	}
}

func TestVerifyRejectsUnsignedToken(t *testing.T) {
	m := newManager(time.Minute)

	// alg=none with the same claims shape; must never be accepted.
	const noneToken = "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0." +
		"eyJzdWIiOiJ1c2VyLTEiLCJpc3MiOiJyZXN0Z28tdGVzdCIsImtpbmQiOiJhY2Nlc3MiLCJleHAiOjk5OTk5OTk5OTl9."
	if _, err := m.Verify(noneToken, token.KindAccess); !errors.Is(err, token.ErrInvalidToken) {
		t.Errorf("got %v, want ErrInvalidToken", err)
	}
}
