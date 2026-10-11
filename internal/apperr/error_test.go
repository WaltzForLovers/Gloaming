package apperr

import (
	"errors"
	"testing"

	"github.com/WaltzForLovers/Gloaming/internal/plan"
)

func TestCatalogMessages(t *testing.T) {
	samples := []*Error{
		Validation("Имя: от 3 до 32 символов, только буквы, цифры и подчёркивание."),
		UserExists(),
		InvalidCredentials(),
		Unauthorized(),
		TitleNotFound(),
		CatalogUnavailable(),
		AlreadyInQueue(),
		InvalidFreeTime(),
		NotFound(),
		PortBusy(),
		Internal(errors.New("disk")),
	}
	seen := map[string]string{}
	for _, err := range samples {
		if err.Code == "" || err.Message == "" {
			t.Fatalf("empty code or message: %+v", err)
		}
		if prev, ok := seen[err.Code]; ok && prev != err.Message && err.Code != CodeValidation {
			t.Fatalf("code %s has two messages: %q and %q", err.Code, prev, err.Message)
		}
		seen[err.Code] = err.Message
		if err.Error() != err.Message {
			t.Fatalf("Error() = %q, want message %q", err.Error(), err.Message)
		}
	}
	want := []string{
		CodeValidation,
		CodeUserExists,
		CodeInvalidCredentials,
		CodeUnauthorized,
		CodeTitleNotFound,
		CodeCatalogUnavailable,
		CodeAlreadyInQueue,
		CodeInvalidFreeTime,
		CodeNotFound,
		CodePortBusy,
		CodeInternal,
	}
	for _, code := range want {
		if _, ok := seen[code]; !ok {
			t.Fatalf("missing code %s", code)
		}
	}
}

func TestFixedPhrases(t *testing.T) {
	if UserExists().Message != "Такое имя уже есть. Выберите другое." {
		t.Fatal(UserExists().Message)
	}
	if InvalidCredentials().Message != "Неверное имя или пароль." {
		t.Fatal(InvalidCredentials().Message)
	}
	if Unauthorized().Message != "Сначала войдите." {
		t.Fatal(Unauthorized().Message)
	}
	if TitleNotFound().Message != "Такого названия не нашли. Проверьте написание." {
		t.Fatal(TitleNotFound().Message)
	}
	if CatalogUnavailable().Message != "Каталог сейчас недоступен. Уже открытые карточки на месте." {
		t.Fatal(CatalogUnavailable().Message)
	}
	if AlreadyInQueue().Message != "Этот тайтл уже в очереди." {
		t.Fatal(AlreadyInQueue().Message)
	}
	if InvalidFreeTime().Message != "Укажите свободное время в пределах семи суток." {
		t.Fatal(InvalidFreeTime().Message)
	}
	if NotFound().Message != "Пункт очереди не найден." {
		t.Fatal(NotFound().Message)
	}
	if PortBusy().Message != "Порт 8080 занят. Закройте программу, которая его держит." {
		t.Fatal(PortBusy().Message)
	}
	if Internal(errors.New("x")).Message != "Что-то сломалось. Попробуйте ещё раз." {
		t.Fatal(Internal(nil).Message)
	}
}

func TestFromMapsPlanAndHidesCause(t *testing.T) {
	got := From(plan.ErrInvalidFreeTime)
	if got.Code != CodeInvalidFreeTime {
		t.Fatalf("code %s", got.Code)
	}
	if errors.Is(got, plan.ErrInvalidFreeTime) == false {
		t.Fatal("expected plan error in chain")
	}
	plain := From(errors.New("sql: locked"))
	if plain.Code != CodeInternal {
		t.Fatalf("code %s", plain.Code)
	}
	if plain.Message != Internal(nil).Message {
		t.Fatal(plain.Message)
	}
	if errors.Unwrap(plain).Error() != "sql: locked" {
		t.Fatal(errors.Unwrap(plain))
	}
	same := From(TitleNotFound())
	if same.Code != CodeTitleNotFound {
		t.Fatal(same)
	}
}
