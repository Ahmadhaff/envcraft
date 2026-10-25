package usecase

import (
	"errors"
	"testing"

	"github.com/ahmadhaffoudhi/auth-service/internal/domain"
)

type mem struct {
	users map[string]domain.User
}

func (m *mem) Save(user domain.User) error {
	if m.users == nil {
		m.users = map[string]domain.User{}
	}
	m.users[user.ID] = user
	return nil
}

func (m *mem) FindByID(id string) (domain.User, error) {
	user, ok := m.users[id]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return user, nil
}

func TestCreateAndGet(t *testing.T) {
	svc := NewUserService(&mem{})
	created, err := svc.Create("u1", "ada@example.com", "Ada")
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Email != "ada@example.com" {
		t.Fatalf("email %q", got.Email)
	}
	if _, err := svc.Create("u2", " ", "Ada"); err == nil {
		t.Fatal("expected validation error")
	}
	_, err = svc.Get("missing")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err %v", err)
	}
}
