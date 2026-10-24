// Package api wires the stores and token manager into an http.Handler.
package api

import (
	"log/slog"
	"net/http"

	"github.com/backend-pc4/restgo/internal/docs"
	"github.com/backend-pc4/restgo/internal/httpx"
	"github.com/backend-pc4/restgo/internal/middleware"
	"github.com/backend-pc4/restgo/internal/store"
	"github.com/backend-pc4/restgo/internal/token"
)

// Options are the dependencies and switches NewServer needs.
type Options struct {
	Users  store.UserStore
	Tokens *token.Manager
	Logger *slog.Logger
	// EnableDocs mounts the OpenAPI document and Swagger UI.
	EnableDocs bool
}

// Server holds the handler dependencies.
type Server struct {
	users      store.UserStore
	tokens     *token.Manager
	logger     *slog.Logger
	enableDocs bool
}

// NewServer constructs a Server.
func NewServer(opts Options) *Server {
	return &Server{
		users:      opts.Users,
		tokens:     opts.Tokens,
		logger:     opts.Logger,
		enableDocs: opts.EnableDocs,
	}
}

// endpoint is one route. Routes builds the mux from this table and
// TestOpenAPIMatchesRoutes reads it, so an endpoint cannot be added, or have
// its auth requirement changed, without the OpenAPI document being updated to
// match.
type endpoint struct {
	method      string
	path        string
	requireAuth bool
	// inSpec marks the route as part of the documented API contract. Only the
	// docs routes themselves are excluded.
	inSpec  bool
	handler http.HandlerFunc
}

const basePath = "/api/v1"

func (s *Server) endpoints() []endpoint {
	eps := []endpoint{
		{method: http.MethodGet, path: "/healthz", inSpec: true, handler: s.handleHealth},

		{method: http.MethodPost, path: basePath + "/auth/register", inSpec: true, handler: s.handleRegister},
		{method: http.MethodPost, path: basePath + "/auth/login", inSpec: true, handler: s.handleLogin},
		{method: http.MethodPost, path: basePath + "/auth/refresh", inSpec: true, handler: s.handleRefresh},

		{method: http.MethodGet, path: basePath + "/me", requireAuth: true, inSpec: true, handler: s.handleMe},
		{method: http.MethodGet, path: basePath + "/users", requireAuth: true, inSpec: true, handler: s.handleListUsers},
	}

	if s.enableDocs {
		eps = append(eps,
			endpoint{method: http.MethodGet, path: docs.UIPath, handler: docs.UIHandler()},
			endpoint{method: http.MethodGet, path: docs.SpecPath, handler: docs.SpecHandler()},
		)
	}
	return eps
}

// Routes returns the fully wrapped handler for the whole API.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	requireAuth := middleware.RequireAuth(s.tokens)

	for _, ep := range s.endpoints() {
		var h http.Handler = ep.handler
		if ep.requireAuth {
			// Wrapping per route, rather than by path prefix, means a new
			// protected endpoint cannot be left unguarded by forgetting to add
			// it to a group.
			h = requireAuth(h)
		}
		mux.Handle(ep.method+" "+ep.path, h)
	}

	return middleware.Chain(mux,
		middleware.Recoverer(s.logger),
		middleware.RequestLogger(s.logger),
		middleware.JSONErrors(),
	)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
