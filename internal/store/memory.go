package store

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/backend-pc4/restgo/internal/model"
)

// MemoryUserStore is a goroutine-safe in-memory UserStore. Data is lost when
// the process exits, which is fine for local development and tests.
type MemoryUserStore struct {
	mu       sync.RWMutex
	byID     map[string]model.User
	idByMail map[string]string
}

// NewMemoryUserStore returns an empty in-memory store.
func NewMemoryUserStore() *MemoryUserStore {
	return &MemoryUserStore{
		byID:     make(map[string]model.User),
		idByMail: make(map[string]string),
	}
}

// Create inserts a user, rejecting duplicate emails. Emails are indexed in
// lower case so that lookups are case-insensitive.
func (s *MemoryUserStore) Create(_ context.Context, u model.User) (model.User, error) {
	key := normalizeEmail(u.Email)

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.idByMail[key]; exists {
		return model.User{}, ErrEmailTaken
	}
	s.byID[u.ID] = u
	s.idByMail[key] = u.ID
	return u, nil
}

// GetByEmail looks a user up by email, ignoring case.
func (s *MemoryUserStore) GetByEmail(_ context.Context, email string) (model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.idByMail[normalizeEmail(email)]
	if !ok {
		return model.User{}, ErrNotFound
	}
	return s.byID[id], nil
}

// GetByID looks a user up by primary key.
func (s *MemoryUserStore) GetByID(_ context.Context, id string) (model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	u, ok := s.byID[id]
	if !ok {
		return model.User{}, ErrNotFound
	}
	return u, nil
}

// List returns every user, newest first, so responses are deterministic.
func (s *MemoryUserStore) List(_ context.Context) ([]model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]model.User, 0, len(s.byID))
	for _, u := range s.byID {
		users = append(users, u)
	}
	sort.Slice(users, func(i, j int) bool {
		if users[i].CreatedAt.Equal(users[j].CreatedAt) {
			return users[i].ID < users[j].ID
		}
		return users[i].CreatedAt.After(users[j].CreatedAt)
	})
	return users, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
