// Package store defines persistence for domain objects. Only an in-memory
// implementation ships today; swapping in Postgres means implementing UserStore
// and changing one line in main.
package store

import (
	"context"
	"errors"

	"github.com/backend-pc4/restgo/internal/model"
)

var (
	// ErrNotFound is returned when no record matches the lookup.
	ErrNotFound = errors.New("record not found")
	// ErrEmailTaken is returned when an email is already registered.
	ErrEmailTaken = errors.New("email already registered")
)

// UserStore is the persistence contract the handlers depend on.
type UserStore interface {
	Create(ctx context.Context, u model.User) (model.User, error)
	GetByEmail(ctx context.Context, email string) (model.User, error)
	GetByID(ctx context.Context, id string) (model.User, error)
	List(ctx context.Context) ([]model.User, error)
}
