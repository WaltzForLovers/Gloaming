package auth

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/WaltzForLovers/Gloaming/internal/apperr"
	"github.com/WaltzForLovers/Gloaming/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

func TestRegisterValidation(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	cases := []struct {
		name string
		user string
		pass string
	}{
		{name: "short name", user: "ab", pass: "password"},
		{name: "long name", user: strings.Repeat("a", 33), pass: "password"},
		{name: "space", user: "ann boy", pass: "password"},
		{name: "short password", user: "ann", pass: "short"},
		{name: "long password", user: "ann", pass: strings.Repeat("a", 73)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Register(ctx, tc.user, tc.pass)
			if apperr.From(err).Code != apperr.CodeValidation {
				t.Fatalf("%v", err)
			}
		})
	}
}

func TestRegisterLoginAndLogout(t *testing.T) {
	svc := newService(t)
	db := svc.db
	ctx := context.Background()

	user, err := svc.Register(ctx, "Анна_1", "пароль12")
	if err != nil {
		t.Fatal(err)
	}
	if user.Username != "Анна_1" || user.ID == 0 {
		t.Fatalf("%+v", user)
	}
	stored, err := db.UserByUsername(ctx, "Анна_1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.PasswordHash == "пароль12" || stored.PasswordHash == "" {
		t.Fatal("password stored in the clear")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte("пароль12")); err != nil {
		t.Fatal(err)
	}

	_, err = svc.Register(ctx, "Анна_1", "пароль12")
	if apperr.From(err).Code != apperr.CodeUserExists {
		t.Fatalf("duplicate: %v", err)
	}

	token, logged, err := svc.Login(ctx, "Анна_1", "пароль12")
	if err != nil || token == "" || logged.ID != user.ID {
		t.Fatalf("token %q user %+v err %v", token, logged, err)
	}
	got, err := svc.User(ctx, token)
	if err != nil || got.Username != "Анна_1" {
		t.Fatalf("%+v %v", got, err)
	}

	_, _, err = svc.Login(ctx, "Анна_1", "wrong-password")
	if apperr.From(err).Code != apperr.CodeInvalidCredentials {
		t.Fatalf("bad password: %v", err)
	}
	_, _, err = svc.Login(ctx, "nobody", "password")
	if apperr.From(err).Code != apperr.CodeInvalidCredentials {
		t.Fatalf("missing user: %v", err)
	}

	if err := svc.Logout(ctx, token); err != nil {
		t.Fatal(err)
	}
	_, err = svc.User(ctx, token)
	if apperr.From(err).Code != apperr.CodeUnauthorized {
		t.Fatalf("after logout: %v", err)
	}
	if err := svc.Logout(ctx, token); err != nil {
		t.Fatal(err)
	}
}

func TestUserWithoutSession(t *testing.T) {
	svc := newService(t)
	_, err := svc.User(context.Background(), "")
	if apperr.From(err).Code != apperr.CodeUnauthorized {
		t.Fatal(err)
	}
	if errors.Is(err, storage.ErrNotFound) {
		t.Fatal("storage error leaked")
	}
}

func newService(t *testing.T) *Service {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "queue.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return New(db)
}
