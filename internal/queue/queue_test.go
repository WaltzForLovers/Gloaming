package queue

import (
	"context"
	"testing"

	"github.com/WaltzForLovers/Gloaming/internal/apperr"
	"github.com/WaltzForLovers/Gloaming/internal/storage"
)

func TestQueueOrder(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	user, err := db.CreateUser(ctx, "ann", "hash", "")
	if err != nil {
		t.Fatal(err)
	}
	other, err := db.CreateUser(ctx, "bob", "hash", "")
	if err != nil {
		t.Fatal(err)
	}
	first := mustTitle(t, db, 1, "First")
	second := mustTitle(t, db, 2, "Second")
	svc := New(db)

	if _, err := svc.Add(ctx, user.ID, 999); apperr.From(err).Code != apperr.CodeNotFound {
		t.Fatalf("missing title: %v", err)
	}
	if _, err := svc.Add(ctx, user.ID, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(ctx, user.ID, first.ID); apperr.From(err).Code != apperr.CodeAlreadyInQueue {
		t.Fatalf("duplicate: %v", err)
	}
	added, err := svc.Add(ctx, user.ID, second.ID)
	if err != nil {
		t.Fatal(err)
	}

	rows, err := svc.List(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Name != "First" || rows[1].Name != "Second" || rows[0].Position != 1 || rows[1].Position != 2 {
		t.Fatalf("%+v", rows)
	}

	if err := svc.Move(ctx, user.ID, added.ID, "up"); err != nil {
		t.Fatal(err)
	}
	rows, err = svc.List(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Name != "Second" || rows[1].Name != "First" {
		t.Fatalf("after up %+v", rows)
	}
	if err := svc.Move(ctx, user.ID, rows[0].ID, "up"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Move(ctx, user.ID, rows[1].ID, "sideways"); apperr.From(err).Code != apperr.CodeValidation {
		t.Fatal(err)
	}
	rows, err = svc.List(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Name != "Second" {
		t.Fatalf("top stayed %+v", rows)
	}

	if err := svc.Move(ctx, other.ID, rows[0].ID, "down"); apperr.From(err).Code != apperr.CodeNotFound {
		t.Fatalf("foreign: %v", err)
	}
	if err := svc.Remove(ctx, user.ID, rows[0].ID); err != nil {
		t.Fatal(err)
	}
	rows, err = svc.List(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Name != "First" || rows[0].Position != 1 {
		t.Fatalf("after delete %+v", rows)
	}
	if err := svc.Remove(ctx, user.ID, rows[0].ID+100); apperr.From(err).Code != apperr.CodeNotFound {
		t.Fatal(err)
	}
	empty, err := svc.List(ctx, other.ID)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty %+v %v", empty, err)
	}
}

func TestAddAllSeasonsSkipsQueued(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	user, err := db.CreateUser(ctx, "ann", "hash", "")
	if err != nil {
		t.Fatal(err)
	}
	title, err := db.UpsertTitle(ctx, storage.Title{
		ExternalID: 7, Name: "Long", Year: 2000, EpisodeCount: 3, AverageMinutes: 40,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = db.ReplaceEpisodes(ctx, title.ID, []storage.EpisodeSeed{
		{SeasonNumber: 1, Number: 1, Name: "A", Runtime: 40},
		{SeasonNumber: 2, Number: 1, Name: "B", Runtime: 40},
		{SeasonNumber: 3, Number: 1, Name: "C", Runtime: 40},
	})
	if err != nil {
		t.Fatal(err)
	}
	seasons, err := db.TitleSeasons(ctx, title.ID, user.ID)
	if err != nil || len(seasons) != 3 {
		t.Fatalf("%v %+v", err, seasons)
	}
	svc := New(db)
	if _, err := svc.AddSeason(ctx, user.ID, seasons[1].ID); err != nil {
		t.Fatal(err)
	}
	added, err := svc.AddAll(ctx, user.ID, title.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 2 || added[0].SeasonNumber != 1 || added[1].SeasonNumber != 3 || added[0].Position != 2 || added[1].Position != 3 {
		t.Fatalf("%+v", added)
	}
	if _, err := svc.AddAll(ctx, user.ID, title.ID); apperr.From(err).Code != apperr.CodeAlreadyInQueue {
		t.Fatalf("again: %v", err)
	}
}

func mustTitle(t *testing.T, db *storage.DB, external int, name string) storage.Title {
	t.Helper()
	title, err := db.UpsertTitle(context.Background(), storage.Title{
		ExternalID: external, Name: name, Year: 2000, EpisodeCount: 1, AverageMinutes: 40,
	})
	if err != nil {
		t.Fatal(err)
	}
	return title
}

func openDB(t *testing.T) *storage.DB {
	t.Helper()
	return storage.OpenTest(t)
}
