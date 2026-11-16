package apperr

import (
	"errors"
	"testing"

	"github.com/WaltzForLovers/Gloaming/internal/plan"
)

func TestCatalogMessages(t *testing.T) {
	samples := []*Error{
		Validation("Name: 3 to 32 characters, letters, digits, and underscore only."),
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
	if UserExists().Message != "That name is taken. Choose another." {
		t.Fatal(UserExists().Message)
	}
	if InvalidCredentials().Message != "Wrong name or password." {
		t.Fatal(InvalidCredentials().Message)
	}
	if Unauthorized().Message != "Sign in first." {
		t.Fatal(Unauthorized().Message)
	}
	if TitleNotFound().Message != "No title matched that. Check the spelling." {
		t.Fatal(TitleNotFound().Message)
	}
	if CatalogUnavailable().Message != "The catalog is unavailable right now. Cards you already opened are still here." {
		t.Fatal(CatalogUnavailable().Message)
	}
	if AlreadyInQueue().Message != "That title is already in the queue." {
		t.Fatal(AlreadyInQueue().Message)
	}
	if InvalidFreeTime().Message != "Enter free time within seven days." {
		t.Fatal(InvalidFreeTime().Message)
	}
	if NotFound().Message != "Queue item not found." {
		t.Fatal(NotFound().Message)
	}
	if PortBusy().Message != "Port 8080 is in use. Close the program that is holding it." {
		t.Fatal(PortBusy().Message)
	}
	if Internal(errors.New("x")).Message != "Something broke. Try again." {
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
