package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/WaltzForLovers/Gloaming/internal/apperr"
	"github.com/WaltzForLovers/Gloaming/internal/mail"
	"github.com/WaltzForLovers/Gloaming/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionLength = 7 * 24 * time.Hour
	resetLength   = time.Hour
	bcryptCost    = bcrypt.DefaultCost
	maxPassword   = 72
)

type User struct {
	ID       int64
	Username string
}

type Service struct {
	db     *storage.DB
	now    func() time.Time
	mail   mail.Sender
	public string
}

func New(db *storage.DB) *Service {
	return &Service{db: db, now: time.Now}
}

func (s *Service) UseMail(sender mail.Sender, publicURL string) *Service {
	s.mail = sender
	s.public = strings.TrimRight(publicURL, "/")
	return s
}

func (s *Service) Register(ctx context.Context, username, password, email string) (User, error) {
	if err := validate(username, password); err != nil {
		return User{}, err
	}
	email = normalizeEmail(email)
	if !validEmail(email) {
		return User{}, apperr.Validation("Укажите адрес почты.")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return User{}, apperr.Internal(err)
	}
	stored, err := s.db.CreateUser(ctx, username, string(hash), email)
	if errors.Is(err, storage.ErrUsernameTaken) {
		return User{}, apperr.UserExists()
	}
	if errors.Is(err, storage.ErrEmailTaken) {
		return User{}, apperr.Validation("Такой адрес уже есть.")
	}
	if err != nil {
		return User{}, apperr.Internal(err)
	}
	s.send(ctx, email, "Аккаунт Gloaming", "Аккаунт "+stored.Username+" создан.\nЕсли это были не вы, ничего делать не нужно.\n")
	return User{ID: stored.ID, Username: stored.Username}, nil
}

func (s *Service) Forgot(ctx context.Context, email string) error {
	email = normalizeEmail(email)
	if !validEmail(email) {
		return apperr.Validation("Укажите адрес почты.")
	}
	userID, err := s.db.UserByEmail(ctx, email)
	if errors.Is(err, storage.ErrNotFound) {
		return nil
	}
	if err != nil {
		return apperr.Internal(err)
	}
	token, err := newToken()
	if err != nil {
		return apperr.Internal(err)
	}
	sum := sha256.Sum256([]byte(token))
	if err := s.db.SaveReset(ctx, userID, hex.EncodeToString(sum[:]), s.now().Add(resetLength)); err != nil {
		return apperr.Internal(err)
	}
	link := s.public + "/reset.html?token=" + token
	if err := s.send(ctx, email, "Смена пароля", "Чтобы задать новый пароль, откройте ссылку. Она действует один час.\n\n"+link+"\n"); err != nil {
		return apperr.Internal(err)
	}
	return nil
}

func (s *Service) Reset(ctx context.Context, token, password string) error {
	if err := validate("reset_ok", password); err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return apperr.Validation("Ссылка устарела. Запросите новую.")
	}
	sum := sha256.Sum256([]byte(token))
	userID, err := s.db.ConsumeReset(ctx, hex.EncodeToString(sum[:]), s.now())
	if errors.Is(err, storage.ErrNotFound) {
		return apperr.Validation("Ссылка устарела. Запросите новую.")
	}
	if err != nil {
		return apperr.Internal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return apperr.Internal(err)
	}
	if err := s.db.SetPassword(ctx, userID, string(hash)); err != nil {
		return apperr.Internal(err)
	}
	if err := s.db.DeleteUserSessions(ctx, userID); err != nil {
		return apperr.Internal(err)
	}
	return nil
}

func (s *Service) send(ctx context.Context, to, subject, body string) error {
	if s.mail == nil || to == "" {
		return nil
	}
	return s.mail.Send(ctx, to, subject, body)
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

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validEmail(email string) bool {
	if len(email) < 6 || len(email) > 254 || strings.ContainsAny(email, " \t") {
		return false
	}
	at := strings.LastIndex(email, "@")
	if at < 1 || at > len(email)-4 {
		return false
	}
	domain := email[at+1:]
	return strings.Contains(domain, ".") && !strings.HasPrefix(domain, ".") && !strings.HasSuffix(domain, ".")
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
