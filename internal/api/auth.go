package api

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/backend-pc4/restgo/internal/httpx"
	"github.com/backend-pc4/restgo/internal/model"
	"github.com/backend-pc4/restgo/internal/store"
	"github.com/backend-pc4/restgo/internal/token"
)

const (
	minPasswordLen = 8
	maxPasswordLen = 72 // bcrypt silently truncates beyond this
)

type registerRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type authResponse struct {
	User   model.User `json:"user"`
	Tokens token.Pair `json:"tokens"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	req.Name = strings.TrimSpace(req.Name)

	if problems := validateRegister(req); len(problems) > 0 {
		httpx.ValidationError(w, problems)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		s.logger.Error("hash password", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not create account")
		return
	}

	id, err := token.NewUserID()
	if err != nil {
		s.logger.Error("generate user id", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not create account")
		return
	}

	user, err := s.users.Create(r.Context(), model.User{
		ID:           id,
		Email:        req.Email,
		Name:         req.Name,
		PasswordHash: string(hash),
		CreatedAt:    time.Now().UTC(),
	})
	if err != nil {
		if errors.Is(err, store.ErrEmailTaken) {
			httpx.Error(w, http.StatusConflict, "email already registered")
			return
		}
		s.logger.Error("create user", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not create account")
		return
	}

	pair, err := s.tokens.NewPair(user.ID, user.Email)
	if err != nil {
		s.logger.Error("issue tokens", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not issue tokens")
		return
	}

	httpx.JSON(w, http.StatusCreated, authResponse{User: user, Tokens: pair})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	user, err := s.users.GetByEmail(r.Context(), req.Email)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// Compare against a dummy hash so a missing account and a wrong
			// password take roughly the same time, and answer identically
			// either way to avoid leaking which emails are registered.
			bcrypt.CompareHashAndPassword(dummyHash, []byte(req.Password))
			httpx.Error(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		s.logger.Error("lookup user", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not sign in")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		httpx.Error(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	pair, err := s.tokens.NewPair(user.ID, user.Email)
	if err != nil {
		s.logger.Error("issue tokens", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not issue tokens")
		return
	}

	httpx.JSON(w, http.StatusOK, authResponse{User: user, Tokens: pair})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.RefreshToken) == "" {
		httpx.ValidationError(w, map[string]string{"refresh_token": "is required"})
		return
	}

	claims, err := s.tokens.Verify(req.RefreshToken, token.KindRefresh)
	if err != nil {
		httpx.Error(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}

	// Re-read the user so a deleted account cannot keep refreshing.
	user, err := s.users.GetByID(r.Context(), claims.Subject)
	if err != nil {
		httpx.Error(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}

	pair, err := s.tokens.NewPair(user.ID, user.Email)
	if err != nil {
		s.logger.Error("issue tokens", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not issue tokens")
		return
	}

	httpx.JSON(w, http.StatusOK, pair)
}

// dummyHash is a valid bcrypt hash of a random value, used only to equalise
// login timing for unknown emails.
var dummyHash = []byte("$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy")

func validateRegister(req registerRequest) map[string]string {
	problems := make(map[string]string)

	if req.Email == "" {
		problems["email"] = "is required"
	} else if _, err := mail.ParseAddress(req.Email); err != nil {
		problems["email"] = "must be a valid email address"
	}

	if req.Name == "" {
		problems["name"] = "is required"
	} else if len(req.Name) > 100 {
		problems["name"] = "must be at most 100 characters"
	}

	switch {
	case req.Password == "":
		problems["password"] = "is required"
	case len(req.Password) < minPasswordLen:
		problems["password"] = "must be at least 8 characters"
	case len(req.Password) > maxPasswordLen:
		problems["password"] = "must be at most 72 characters"
	}

	return problems
}
