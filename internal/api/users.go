package api

import (
	"net/http"

	"github.com/backend-pc4/restgo/internal/httpx"
	"github.com/backend-pc4/restgo/internal/middleware"
	"github.com/backend-pc4/restgo/internal/model"
)

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFrom(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	user, err := s.users.GetByID(r.Context(), claims.Subject)
	if err != nil {
		// The token is valid but the account is gone.
		httpx.Error(w, http.StatusUnauthorized, "account no longer exists")
		return
	}

	httpx.JSON(w, http.StatusOK, user)
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.users.List(r.Context())
	if err != nil {
		s.logger.Error("list users", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not list users")
		return
	}

	httpx.JSON(w, http.StatusOK, struct {
		Data  []model.User `json:"data"`
		Count int          `json:"count"`
	}{Data: users, Count: len(users)})
}
