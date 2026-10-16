package auth

import (
	"context"
	"errors"
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
			_, err := svc.Register(ctx, tc.user, tc.pass, "ann@example.com")
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

	user, err := svc.Register(ctx, "Анна_1", "пароль12", "anna@example.com")
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

	_, err = svc.Register(ctx, "Анна_1", "пароль12", "anna@example.com")
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

type box struct {
	n                 int
	to, subject, body string
}

func (b *box) Send(_ context.Context, to, subject, body string) error {
	b.n++
	b.to, b.subject, b.body = to, subject, body
	return nil
}

func TestSetEmailOnExistingAccount(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	sent := &box{}
	svc.UseMail(sent, "http://127.0.0.1:8080")
	ann, err := svc.Register(ctx, "ann", "password", "ann@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Register(ctx, "bob", "password", "bob@example.com"); err != nil {
		t.Fatal(err)
	}
	before := sent.n
	same, err := svc.SetEmail(ctx, ann.ID, ann.Username, " Ann@Example.com ")
	if err != nil || same != "ann@example.com" || sent.n != before {
		t.Fatalf("same address: %q n=%d err=%v", same, sent.n, err)
	}
	got, err := svc.SetEmail(ctx, ann.ID, ann.Username, "ann2@example.com")
	if err != nil || got != "ann2@example.com" {
		t.Fatalf("set: %q %v", got, err)
	}
	if sent.to != "ann2@example.com" || !strings.Contains(sent.body, "указан этот адрес") {
		t.Fatalf("notice %q %q", sent.to, sent.body)
	}
	if _, err := svc.SetEmail(ctx, ann.ID, ann.Username, "bob@example.com"); apperr.From(err).Code != apperr.CodeValidation {
		t.Fatalf("taken: %v", err)
	}
	if _, err := svc.SetEmail(ctx, ann.ID, ann.Username, "nope"); apperr.From(err).Code != apperr.CodeValidation {
		t.Fatalf("invalid: %v", err)
	}
	cleared, err := svc.SetEmail(ctx, ann.ID, ann.Username, "  ")
	if err != nil || cleared != "" {
		t.Fatalf("clear: %q %v", cleared, err)
	}
	stored, err := svc.Email(ctx, ann.ID)
	if err != nil || stored != "" {
		t.Fatalf("stored %q %v", stored, err)
	}
}

func TestResetPasswordByMail(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	sent := &box{}
	svc.UseMail(sent, "http://127.0.0.1:8080")
	if _, err := svc.Register(ctx, "ann", "password", "ann@example.com"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sent.body, "создан") {
		t.Fatalf("welcome %q", sent.body)
	}
	if err := svc.Forgot(ctx, "missing@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Forgot(ctx, "ann@example.com"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sent.body, "reset.html?token=") {
		t.Fatalf("reset mail %q", sent.body)
	}
	token := sent.body[strings.Index(sent.body, "token=")+len("token="):]
	token = strings.TrimSpace(strings.Split(token, "\n")[0])
	if err := svc.Reset(ctx, token, "new-password"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Login(ctx, "ann", "password"); apperr.From(err).Code != apperr.CodeInvalidCredentials {
		t.Fatalf("old password: %v", err)
	}
	if _, _, err := svc.Login(ctx, "ann", "new-password"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reset(ctx, token, "another-password"); apperr.From(err).Code != apperr.CodeValidation {
		t.Fatalf("spent token: %v", err)
	}
}

func newService(t *testing.T) *Service {
	t.Helper()
	return New(storage.OpenTest(t))
}
