// Package httpx contains small helpers for decoding requests and writing
// consistently shaped JSON responses.
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

const maxBodyBytes = 1 << 20 // 1 MiB

// ErrorBody is the envelope for every non-2xx response.
type ErrorBody struct {
	Error   string            `json:"error"`
	Details map[string]string `json:"details,omitempty"`
}

// JSON writes v as JSON with the given status code.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The status line is already on the wire, so logging is all we can do.
		slog.Error("write json response", "error", err)
	}
}

// Error writes a plain error envelope.
func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, ErrorBody{Error: msg})
}

// ValidationError writes a 422 with per-field messages.
func ValidationError(w http.ResponseWriter, details map[string]string) {
	JSON(w, http.StatusUnprocessableEntity, ErrorBody{
		Error:   "validation failed",
		Details: details,
	})
}

// Decode reads a JSON body into dst. It caps the body size and rejects unknown
// fields so typos in a client payload surface as errors instead of being
// silently ignored.
func Decode(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("malformed JSON body: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("body must contain a single JSON object")
	}
	return nil
}
