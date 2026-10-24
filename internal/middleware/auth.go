package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/backend-pc4/restgo/internal/httpx"
	"github.com/backend-pc4/restgo/internal/token"
)

// ctxKey is unexported so no other package can collide with our context keys.
type ctxKey struct{ name string }

var claimsKey = ctxKey{"claims"}

// Authenticator verifies access tokens. internal/token.Manager satisfies it;
// the indirection keeps middleware testable with a stub.
type Authenticator interface {
	Verify(raw string, want token.Kind) (*token.Claims, error)
}

// RequireAuth rejects requests without a valid `Authorization: Bearer <jwt>`
// header and stores the verified claims on the request context.
func RequireAuth(auth Authenticator) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				w.Header().Set("WWW-Authenticate", `Bearer realm="restgo"`)
				httpx.Error(w, http.StatusUnauthorized, "missing or malformed Authorization header")
				return
			}

			claims, err := auth.Verify(raw, token.KindAccess)
			if err != nil {
				w.Header().Set("WWW-Authenticate", `Bearer realm="restgo", error="invalid_token"`)
				httpx.Error(w, http.StatusUnauthorized, "invalid or expired access token")
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ClaimsFrom returns the claims attached by RequireAuth.
func ClaimsFrom(ctx context.Context) (*token.Claims, bool) {
	claims, ok := ctx.Value(claimsKey).(*token.Claims)
	return claims, ok
}

func bearerToken(header string) (string, bool) {
	scheme, value, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	value = strings.TrimSpace(value)
	return value, value != ""
}
