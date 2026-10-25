package domain

import "errors"

// ErrNotFound is returned when a user id is unknown.
var ErrNotFound = errors.New("user not found")

// User is the domain entity for an account.
type User struct {
	ID    string
	Email string
	Name  string
}

// UserRepository persists users. Adapters implement this port.
type UserRepository interface {
	Save(user User) error
	FindByID(id string) (User, error)
}
