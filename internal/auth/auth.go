package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/WaltzForLovers/Gloaming/internal/apperr"
	"github.com/WaltzForLovers/Gloaming/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionLength = 7 * 24 * time.Hour
	bcryptCost    = bcrypt.DefaultCost
	maxPassword   = 72
)

type User struct {
	ID       int64
	Username string
}

type Service struct {
	db  *storage.DB
	now func() time.Time
}

func New(db *storage.DB) *Service {
	return &Service{db: db, now: time.Now}
}

func (s *Service) Register(ctx context.Context, username, password string) (User, error) {
	if err := validate(username, password); err != nil {
		return User{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return User{}, apperr.Internal(err)
	}
	stored, err := s.db.CreateUser(ctx, username, string(hash))
	if errors.Is(err, storage.ErrUsernameTaken) {
		return User{}, apperr.UserExists()
	}
	if err != nil {
		return User{}, apperr.Internal(err)
	}
	return User{ID: stored.ID, Username: stored.Username}, nil
}

func (s *Service) Login(ctx context.Context, username, password string) (string, User, error) {
	stored, err := s.db.UserByUsername(ctx, username)
	if errors.Is(err, storage.ErrNotFound) {
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
		return "", User{}, apperr.InvalidCredentials()
	}
	if err != nil {
		return "", User{}, apperr.Internal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte(password)); err != nil {
		return "", User{}, apperr.InvalidCredentials()
	}
	token, err := newToken()
	if err != nil {
		return "", User{}, apperr.Internal(err)
	}
	if err := s.db.CreateSession(ctx, stored.ID, token, s.now().Add(sessionLength)); err != nil {
		return "", User{}, apperr.Internal(err)
	}
	return token, User{ID: stored.ID, Username: stored.Username}, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	if err := s.db.DeleteSession(ctx, token); err != nil {
		return apperr.Internal(err)
	}
	return nil
}

func (s *Service) User(ctx context.Context, token string) (User, error) {
	if token == "" {
		return User{}, apperr.Unauthorized()
	}
	stored, err := s.db.UserByToken(ctx, token, s.now())
	if errors.Is(err, storage.ErrNotFound) {
		return User{}, apperr.Unauthorized()
	}
	if err != nil {
		return User{}, apperr.Internal(err)
	}
	return User{ID: stored.ID, Username: stored.Username}, nil
}

func validate(username, password string) error {
	n := utf8.RuneCountInString(username)
	if n < 3 || n > 32 || !onlyNameRunes(username) {
		return apperr.Validation("Имя: от 3 до 32 символов, только буквы, цифры и подчёркивание.")
	}
	if utf8.RuneCountInString(password) < 8 {
		return apperr.Validation("Пароль: не короче 8 символов.")
	}
	if len(password) > maxPassword {
		return apperr.Validation("Пароль слишком длинный.")
	}
	return nil
}

func onlyNameRunes(s string) bool {
	for _, r := range s {
		if r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		return false
	}
	return true
}

func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

var dummyHash = func() string {
	hash, err := bcrypt.GenerateFromPassword([]byte("slate-dummy-password"), bcryptCost)
	if err != nil {
		panic(err)
	}
	return string(hash)
}()
