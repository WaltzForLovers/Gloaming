package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/WaltzForLovers/Gloaming/internal/apperr"
)

func TestOpenCreatesTablesAndReopens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.db")
	db := openAt(t, path)
	ctx := context.Background()
	if _, err := db.CreateUser(ctx, "ann", "hash"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = openAt(t, path)
	got, err := db.UserByUsername(ctx, "ann")
	if err != nil {
		t.Fatal(err)
	}
	if got.Username != "ann" || got.PasswordHash != "hash" {
		t.Fatalf("%+v", got)
	}
}

func TestOpenMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "queue.db")
	_, err := Open(path)
	if err == nil {
		t.Fatal("expected error")
	}
	if apperr.From(err).Code != apperr.CodeInternal {
		t.Fatal(apperr.From(err))
	}
}

func TestUsersAndSessions(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	user, err := db.CreateUser(ctx, "ann", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateUser(ctx, "ann", "other"); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("duplicate: %v", err)
	}
	if _, err := db.UserByUsername(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing user: %v", err)
	}

	expires := time.Now().Add(time.Hour).UTC()
	if err := db.CreateSession(ctx, user.ID, "tok", expires); err != nil {
		t.Fatal(err)
	}
	got, err := db.UserByToken(ctx, "tok", time.Now().UTC())
	if err != nil || got.ID != user.ID {
		t.Fatalf("user %+v err %v", got, err)
	}
	if _, err := db.UserByToken(ctx, "tok", expires.Add(time.Second)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired: %v", err)
	}
	if err := db.DeleteSession(ctx, "tok"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UserByToken(ctx, "tok", time.Now().UTC()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted: %v", err)
	}
}

func TestTitlesAndQueue(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	ann, err := db.CreateUser(ctx, "ann", "hash")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := db.CreateUser(ctx, "bob", "hash")
	if err != nil {
		t.Fatal(err)
	}

	first, err := db.UpsertTitle(ctx, Title{ExternalID: 139, Name: "Girls", Year: 2012, EpisodeCount: 10, AverageMinutes: 30})
	if err != nil {
		t.Fatal(err)
	}
	again, err := db.UpsertTitle(ctx, Title{ExternalID: 139, Name: "Girls!", Year: 2012, EpisodeCount: 11, AverageMinutes: 28})
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != first.ID || again.Name != "Girls!" || again.EpisodeCount != 11 {
		t.Fatalf("upsert %+v", again)
	}
	if _, err := db.TitleByID(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing title: %v", err)
	}

	if _, err := db.AddQueueItem(ctx, ann.ID, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing title in queue: %v", err)
	}
	item, err := db.AddQueueItem(ctx, ann.ID, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if item.Position != 1 || item.Title.Name != "Girls!" {
		t.Fatalf("%+v", item)
	}
	if _, err := db.AddQueueItem(ctx, ann.ID, first.ID); !errors.Is(err, ErrQueueDuplicate) {
		t.Fatalf("duplicate: %v", err)
	}

	second, err := db.UpsertTitle(ctx, Title{ExternalID: 140, Name: "Film", Year: 1999, EpisodeCount: 1, AverageMinutes: 100})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddQueueItem(ctx, ann.ID, second.ID); err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListQueue(ctx, ann.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Title.Name != "Girls!" || rows[1].Title.Name != "Film" || rows[0].Position != 1 || rows[1].Position != 2 {
		t.Fatalf("%+v", rows)
	}

	if err := db.MoveQueueItem(ctx, ann.ID, rows[1].ID, -1); err != nil {
		t.Fatal(err)
	}
	rows, err = db.ListQueue(ctx, ann.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Title.Name != "Film" || rows[1].Title.Name != "Girls!" || rows[0].Position != 1 || rows[1].Position != 2 {
		t.Fatalf("after move %+v", rows)
	}
	if err := db.MoveQueueItem(ctx, ann.ID, rows[0].ID, -1); err != nil {
		t.Fatal(err)
	}
	rows, err = db.ListQueue(ctx, ann.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Title.Name != "Film" {
		t.Fatalf("top stayed: %+v", rows)
	}

	if err := db.DeleteQueueItem(ctx, ann.ID, rows[0].ID); err != nil {
		t.Fatal(err)
	}
	rows, err = db.ListQueue(ctx, ann.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Title.Name != "Girls!" || rows[0].Position != 1 {
		t.Fatalf("after delete %+v", rows)
	}
	if err := db.DeleteQueueItem(ctx, ann.ID, rows[0].ID+100); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing item: %v", err)
	}
	if err := db.MoveQueueItem(ctx, bob.ID, rows[0].ID, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign item: %v", err)
	}
	bobRows, err := db.ListQueue(ctx, bob.ID)
	if err != nil || len(bobRows) != 0 {
		t.Fatalf("bob %+v err %v", bobRows, err)
	}
}

func TestSameExternalIDFromTwoSources(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	show, err := db.UpsertTitle(ctx, Title{Source: "tvmaze", ExternalID: 10, Name: "Show", Year: 2008, EpisodeCount: 62, AverageMinutes: 60})
	if err != nil {
		t.Fatal(err)
	}
	film, err := db.UpsertTitle(ctx, Title{Source: "wikidata", ExternalID: 10, Name: "Film", Year: 2010, EpisodeCount: 1, AverageMinutes: 98})
	if err != nil {
		t.Fatal(err)
	}
	if show.ID == film.ID || film.Name != "Film" {
		t.Fatalf("show %+v film %+v", show, film)
	}
	again, err := db.UpsertTitle(ctx, Title{Source: "wikidata", ExternalID: 10, Name: "Film 2", Year: 2010, EpisodeCount: 1, AverageMinutes: 99})
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != film.ID || again.Name != "Film 2" || again.AverageMinutes != 99 {
		t.Fatalf("film upsert %+v", again)
	}
	kept, err := db.TitleByID(ctx, show.ID)
	if err != nil || kept.Name != "Show" {
		t.Fatalf("show overwritten %+v %v", kept, err)
	}
}

func open(t *testing.T) *DB {
	t.Helper()
	return openAt(t, filepath.Join(t.TempDir(), "queue.db"))
}

func openAt(t *testing.T, path string) *DB {
	t.Helper()
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
