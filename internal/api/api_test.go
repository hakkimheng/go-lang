package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/backend-pc4/restgo/internal/api"
	"github.com/backend-pc4/restgo/internal/store"
	"github.com/backend-pc4/restgo/internal/token"
)

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokens := token.NewManager([]byte("test-secret-value-must-be-long-enough"), "restgo-test", time.Minute, time.Hour)
	return api.NewServer(api.Options{
		Users:      store.NewMemoryUserStore(),
		Tokens:     tokens,
		Logger:     logger,
		EnableDocs: true,
	}).Routes()
}

func do(t *testing.T, h http.Handler, method, path, body, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&out); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
	return out
}

type authEnvelope struct {
	User struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	} `json:"user"`
	Tokens token.Pair `json:"tokens"`
}

const validRegister = `{"email":"ada@example.com","name":"Ada","password":"sup3rsecret"}`

func TestHealth(t *testing.T) {
	rec := do(t, newTestHandler(t), http.MethodGet, "/healthz", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
}

func TestRegisterLoginAndAccessProtectedRoute(t *testing.T) {
	h := newTestHandler(t)

	rec := do(t, h, http.MethodPost, "/api/v1/auth/register", validRegister, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: got status %d (%s), want 201", rec.Code, rec.Body.String())
	}
	registered := decode[authEnvelope](t, rec)
	if registered.Tokens.AccessToken == "" || registered.Tokens.RefreshToken == "" {
		t.Fatal("register: expected both tokens to be returned")
	}
	if strings.Contains(rec.Body.String(), "password") {
		t.Errorf("register: response leaked a password field: %s", rec.Body.String())
	}

	// Email lookup must be case-insensitive.
	rec = do(t, h, http.MethodPost, "/api/v1/auth/login",
		`{"email":"ADA@example.com","password":"sup3rsecret"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login: got status %d (%s), want 200", rec.Code, rec.Body.String())
	}
	loggedIn := decode[authEnvelope](t, rec)

	rec = do(t, h, http.MethodGet, "/api/v1/me", "", loggedIn.Tokens.AccessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("me: got status %d (%s), want 200", rec.Code, rec.Body.String())
	}
	me := decode[struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}](t, rec)
	if me.ID != registered.User.ID {
		t.Errorf("me: got id %q, want %q", me.ID, registered.User.ID)
	}
	if me.Email != "ada@example.com" {
		t.Errorf("me: got email %q, want ada@example.com", me.Email)
	}
}

func TestRegisterRejectsDuplicateEmail(t *testing.T) {
	h := newTestHandler(t)

	if rec := do(t, h, http.MethodPost, "/api/v1/auth/register", validRegister, ""); rec.Code != http.StatusCreated {
		t.Fatalf("first register: got status %d, want 201", rec.Code)
	}
	rec := do(t, h, http.MethodPost, "/api/v1/auth/register", validRegister, "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate register: got status %d (%s), want 409", rec.Code, rec.Body.String())
	}
}

func TestRegisterValidation(t *testing.T) {
	tests := map[string]struct {
		body      string
		wantField string
	}{
		"bad email":      {`{"email":"not-an-email","name":"Ada","password":"sup3rsecret"}`, "email"},
		"short password": {`{"email":"ada@example.com","name":"Ada","password":"short"}`, "password"},
		"missing name":   {`{"email":"ada@example.com","name":"","password":"sup3rsecret"}`, "name"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rec := do(t, newTestHandler(t), http.MethodPost, "/api/v1/auth/register", tc.body, "")
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("got status %d (%s), want 422", rec.Code, rec.Body.String())
			}
			body := decode[struct {
				Details map[string]string `json:"details"`
			}](t, rec)
			if _, ok := body.Details[tc.wantField]; !ok {
				t.Errorf("expected a problem for field %q, got %v", tc.wantField, body.Details)
			}
		})
	}
}

func TestRegisterRejectsUnknownFields(t *testing.T) {
	rec := do(t, newTestHandler(t), http.MethodPost, "/api/v1/auth/register",
		`{"email":"ada@example.com","name":"Ada","password":"sup3rsecret","role":"admin"}`, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d (%s), want 400", rec.Code, rec.Body.String())
	}
}

func TestLoginWithWrongCredentials(t *testing.T) {
	h := newTestHandler(t)
	do(t, h, http.MethodPost, "/api/v1/auth/register", validRegister, "")

	tests := map[string]string{
		"wrong password": `{"email":"ada@example.com","password":"wrongpassword"}`,
		"unknown email":  `{"email":"nobody@example.com","password":"sup3rsecret"}`,
	}

	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			rec := do(t, h, http.MethodPost, "/api/v1/auth/login", body, "")
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("got status %d (%s), want 401", rec.Code, rec.Body.String())
			}
			// Both cases must be indistinguishable to avoid leaking which
			// emails are registered.
			if got := decode[struct {
				Error string `json:"error"`
			}](t, rec).Error; got != "invalid email or password" {
				t.Errorf("got error %q, want a generic message", got)
			}
		})
	}
}

func TestProtectedRouteRejectsBadTokens(t *testing.T) {
	h := newTestHandler(t)
	registered := decode[authEnvelope](t, do(t, h, http.MethodPost, "/api/v1/auth/register", validRegister, ""))

	tests := map[string]string{
		"no token":          "",
		"garbage token":     "not-a-jwt",
		"tampered token":    registered.Tokens.AccessToken + "x",
		"refresh as access": registered.Tokens.RefreshToken,
	}

	for name, bearer := range tests {
		t.Run(name, func(t *testing.T) {
			rec := do(t, h, http.MethodGet, "/api/v1/me", "", bearer)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("got status %d (%s), want 401", rec.Code, rec.Body.String())
			}
			if rec.Header().Get("WWW-Authenticate") == "" {
				t.Error("expected a WWW-Authenticate header on a 401")
			}
		})
	}
}

func TestRefreshRotatesTokens(t *testing.T) {
	h := newTestHandler(t)
	registered := decode[authEnvelope](t, do(t, h, http.MethodPost, "/api/v1/auth/register", validRegister, ""))

	rec := do(t, h, http.MethodPost, "/api/v1/auth/refresh",
		`{"refresh_token":"`+registered.Tokens.RefreshToken+`"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh: got status %d (%s), want 200", rec.Code, rec.Body.String())
	}
	refreshed := decode[token.Pair](t, rec)
	if refreshed.AccessToken == "" {
		t.Fatal("refresh: expected a new access token")
	}

	if rec := do(t, h, http.MethodGet, "/api/v1/me", "", refreshed.AccessToken); rec.Code != http.StatusOK {
		t.Fatalf("me with refreshed token: got status %d, want 200", rec.Code)
	}

	// An access token must not be usable to refresh.
	rec = do(t, h, http.MethodPost, "/api/v1/auth/refresh",
		`{"refresh_token":"`+registered.Tokens.AccessToken+`"}`, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("access token used as refresh: got status %d, want 401", rec.Code)
	}
}

func TestListUsersRequiresAuth(t *testing.T) {
	h := newTestHandler(t)
	registered := decode[authEnvelope](t, do(t, h, http.MethodPost, "/api/v1/auth/register", validRegister, ""))

	if rec := do(t, h, http.MethodGet, "/api/v1/users", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated: got status %d, want 401", rec.Code)
	}

	rec := do(t, h, http.MethodGet, "/api/v1/users", "", registered.Tokens.AccessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated: got status %d (%s), want 200", rec.Code, rec.Body.String())
	}
	body := decode[struct {
		Count int `json:"count"`
	}](t, rec)
	if body.Count != 1 {
		t.Errorf("got count %d, want 1", body.Count)
	}
}

func TestDocsEndpoints(t *testing.T) {
	h := newTestHandler(t)

	rec := do(t, h, http.MethodGet, "/openapi.yaml", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("spec: got status %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/yaml") {
		t.Errorf("spec: got Content-Type %q, want application/yaml", ct)
	}
	if !strings.Contains(rec.Body.String(), "openapi: 3.1.0") {
		t.Error("spec: body does not look like an OpenAPI 3.1 document")
	}

	rec = do(t, h, http.MethodGet, "/docs", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("ui: got status %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("ui: got Content-Type %q, want text/html", ct)
	}
	// The UI is useless if it cannot find the document it describes.
	if body := rec.Body.String(); !strings.Contains(body, `url: "/openapi.yaml"`) {
		t.Error("ui: page does not point at /openapi.yaml")
	}
}

func TestDocsCanBeDisabled(t *testing.T) {
	h := api.NewServer(api.Options{
		Users:      store.NewMemoryUserStore(),
		Tokens:     token.NewManager([]byte("test-secret-value-must-be-long-enough"), "restgo-test", time.Minute, time.Hour),
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		EnableDocs: false,
	}).Routes()

	for _, path := range []string{"/docs", "/openapi.yaml"} {
		if rec := do(t, h, http.MethodGet, path, "", ""); rec.Code != http.StatusNotFound {
			t.Errorf("%s: got status %d with docs disabled, want 404", path, rec.Code)
		}
	}

	// Disabling docs must not affect the API itself.
	if rec := do(t, h, http.MethodGet, "/healthz", "", ""); rec.Code != http.StatusOK {
		t.Errorf("healthz: got status %d, want 200", rec.Code)
	}
}

func TestUnknownRouteAndMethod(t *testing.T) {
	h := newTestHandler(t)

	rec := do(t, h, http.MethodGet, "/api/v1/nope", "", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown route: got status %d, want 404", rec.Code)
	}
	assertJSONError(t, rec, "route not found")

	rec = do(t, h, http.MethodGet, "/api/v1/auth/login", "", "")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("wrong method: got status %d, want 405", rec.Code)
	}
	assertJSONError(t, rec, "method not allowed")
	if allow := rec.Header().Get("Allow"); allow != "POST" {
		t.Errorf("got Allow %q, want POST", allow)
	}
}

// assertJSONError checks that a routing error still uses the JSON envelope.
func assertJSONError(t *testing.T, rec *httptest.ResponseRecorder, wantMsg string) {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("got Content-Type %q, want JSON", ct)
	}
	got := decode[struct {
		Error string `json:"error"`
	}](t, rec).Error
	if got != wantMsg {
		t.Errorf("got error %q, want %q", got, wantMsg)
	}
}
