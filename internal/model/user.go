// Package model holds the domain types shared across the application.
package model

import "time"

// User is the stored representation of an account. PasswordHash never leaves
// the process: the json tag is "-" so it cannot leak through a response body.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}
