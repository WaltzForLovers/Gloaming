package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/WaltzForLovers/Gloaming/internal/apperr"
)

func TestOpenCreatesTablesAndReopens(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	if _, err := db.CreateUser(ctx, "ann", "hash", ""); err != nil {
		t.Fatal(err)
	}
	dsn := db.DSN()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.Close() })
	got, err := reopened.UserByUsername(ctx, "ann")
	if err != nil {
		t.Fatal(err)
	}
	if got.Username != "ann" || got.PasswordHash != "hash" {
		t.Fatalf("%+v", got)
	}
}

func TestOpenMissingDatabaseHost(t *testing.T) {
	_, err := Open("postgres://127.0.0.1:1/nope?sslmode=disable&connect_timeout=1")
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
	user, err := db.CreateUser(ctx, "ann", "hash", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateUser(ctx, "ann", "other", ""); !errors.Is(err, ErrUsernameTaken) {
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
	ann, err := db.CreateUser(ctx, "ann", "hash", "")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := db.CreateUser(ctx, "bob", "hash", "")
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

func TestWatchedEpisodesLeaveTheRemainder(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	user, err := db.CreateUser(ctx, "ann", "hash", "")
	if err != nil {
		t.Fatal(err)
	}
	title, err := db.UpsertTitle(ctx, Title{ExternalID: 1, Name: "Show", Year: 2010, EpisodeCount: 1, AverageMinutes: 1})
	if err != nil {
		t.Fatal(err)
	}
	err = db.ReplaceEpisodes(ctx, title.ID, []EpisodeSeed{
		{SeasonNumber: 1, Number: 1, Name: "One", Runtime: 40},
		{SeasonNumber: 1, Number: 2, Name: "Two", Runtime: 50},
		{SeasonNumber: 2, Number: 1, Name: "Next", Runtime: 60},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddQueueItem(ctx, user.ID, title.ID); !errors.Is(err, ErrPickSeason) {
		t.Fatalf("whole show: %v", err)
	}
	seasons, err := db.TitleSeasons(ctx, title.ID, user.ID)
	if err != nil || len(seasons) != 2 || len(seasons[0].Episodes) != 2 {
		t.Fatalf("%+v %v", seasons, err)
	}
	if err := db.SetWatched(ctx, user.ID, seasons[0].Episodes[0].ID, true); err != nil {
		t.Fatal(err)
	}
	item, err := db.AddSeason(ctx, user.ID, seasons[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if item.TotalEpisodes != 2 || len(item.Remaining) != 1 || item.Remaining[0] != 50 {
		t.Fatalf("%+v", item)
	}
}

func TestManualTitleStaysWithItsOwner(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	owner, err := db.CreateUser(ctx, "owner", "hash", "")
	if err != nil {
		t.Fatal(err)
	}
	other, err := db.CreateUser(ctx, "other", "hash", "")
	if err != nil {
		t.Fatal(err)
	}
	saved, err := db.CreateManual(ctx, owner.ID, Title{Name: "Домашний вечер", Year: 1999, EpisodeCount: 3, AverageMinutes: 40})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Source != "manual" || saved.OwnerID != owner.ID || saved.ID == 0 {
		t.Fatalf("%+v", saved)
	}
	seasons, err := db.TitleSeasons(ctx, saved.ID, owner.ID)
	if err != nil || len(seasons) != 1 || len(seasons[0].Episodes) != 3 || seasons[0].Episodes[2].Runtime != 40 {
		t.Fatalf("%+v %v", seasons, err)
	}
	found, err := db.ManualTitles(ctx, owner.ID, "дом")
	if err != nil || len(found) != 1 || found[0].ID != saved.ID {
		t.Fatalf("%+v %v", found, err)
	}
	hidden, err := db.ManualTitles(ctx, other.ID, "домашний")
	if err != nil || len(hidden) != 0 {
		t.Fatalf("%+v %v", hidden, err)
	}
	if _, err := db.ManualTitles(ctx, owner.ID, "100%"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddQueueItem(ctx, other.ID, saved.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other user: %v", err)
	}
	item, err := db.AddQueueItem(ctx, owner.ID, saved.ID)
	if err != nil || item.Title.Name != "Домашний вечер" || len(item.Remaining) != 3 {
		t.Fatalf("%+v %v", item, err)
	}
}

func open(t *testing.T) *DB {
	t.Helper()
	return OpenTest(t)
}
