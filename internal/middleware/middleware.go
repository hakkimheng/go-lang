// Package middleware holds the cross-cutting HTTP concerns: panic recovery,
// request logging, and JWT authentication.
package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/backend-pc4/restgo/internal/httpx"
)

// Middleware decorates an http.Handler.
type Middleware func(http.Handler) http.Handler

// Chain applies middlewares so that the first argument is the outermost layer.
func Chain(h http.Handler, mw ...Middleware) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

// Recoverer turns a panic in a handler into a 500 instead of killing the
// process and dropping the connection.
func Recoverer(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered",
						"panic", rec,
						"method", r.Method,
						"path", r.URL.Path,
					)
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":"internal server error"}`))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// RequestLogger logs one line per request with status and latency.
func RequestLogger(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rec, r)

			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"bytes", rec.written,
				"duration", time.Since(start).String(),
			)
		})
	}
}

// JSONErrors rewrites the plain-text 404 and 405 responses that http.ServeMux
// produces for unmatched routes into the API's JSON error envelope, so clients
// only ever have to parse one response format. Handler-written JSON errors pass
// through untouched.
func JSONErrors() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := &errorRewriter{ResponseWriter: w}
			next.ServeHTTP(rw, r)
			rw.flush()
		})
	}
}

type errorRewriter struct {
	http.ResponseWriter
	status    int
	intercept bool
	wrote     bool
}

func (w *errorRewriter) WriteHeader(status int) {
	if w.wrote {
		return
	}
	w.wrote = true
	w.status = status

	isRoutingError := status == http.StatusNotFound || status == http.StatusMethodNotAllowed
	if isRoutingError && !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
		// Hold the response back so flush can replace it wholesale.
		w.intercept = true
		return
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *errorRewriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	if w.intercept {
		return len(b), nil // discard the plain-text body
	}
	return w.ResponseWriter.Write(b)
}

func (w *errorRewriter) flush() {
	if !w.intercept {
		return
	}
	msg := "route not found"
	if w.status == http.StatusMethodNotAllowed {
		msg = "method not allowed"
	}
	// Preserves the Allow header ServeMux sets on a 405.
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.ResponseWriter.WriteHeader(w.status)
	_ = json.NewEncoder(w.ResponseWriter).Encode(httpx.ErrorBody{Error: msg})
}

type statusRecorder struct {
	http.ResponseWriter
	status  int
	written int
	wrote   bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.wrote {
		return
	}
	r.wrote = true
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wrote {
		r.WriteHeader(http.StatusOK)
	}
	n, err := r.ResponseWriter.Write(b)
	r.written += n
	return n, err
}
