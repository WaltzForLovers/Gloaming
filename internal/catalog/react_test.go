package catalog

import (
	"context"
	"testing"
	"time"

	"github.com/WaltzForLovers/Gloaming/internal/queue"
	"github.com/WaltzForLovers/Gloaming/internal/storage"
)

func TestApplyWatchedKeepsThursdayLetterAndSuggestions(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	user, err := db.CreateUser(ctx, "ann", "hash", "")
	if err != nil {
		t.Fatal(err)
	}
	queued, err := db.UpsertTitle(ctx, storage.Title{
		ExternalID: 1, Name: "Queued", Year: 2001, EpisodeCount: 1, AverageMinutes: 40, Genres: []string{"Drama"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertTitle(ctx, storage.Title{
		ExternalID: 2, Name: "Neighbor", Year: 2002, EpisodeCount: 1, AverageMinutes: 90, Genres: []string{"Drama"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertTitle(ctx, storage.Title{
		ExternalID: 3, Name: "Comedy", Year: 2003, EpisodeCount: 1, AverageMinutes: 30, Genres: []string{"Comedy"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetDailyMinutes(ctx, user.ID, 40); err != nil {
		t.Fatal(err)
	}
	if _, err := queue.New(db).Add(ctx, user.ID, queued.ID); err != nil {
		t.Fatal(err)
	}
	svc := New(db, &fakeClient{})
	thursday := time.Date(2026, 10, 15, 12, 0, 0, 0, time.Local)
	if thursday.Weekday() != time.Thursday {
		t.Fatalf("fixture weekday %s", thursday.Weekday())
	}
	if err := svc.ApplyWatched(ctx, user.ID, thursday); err != nil {
		t.Fatal(err)
	}
	letter, err := svc.Letter(ctx, user.ID)
	if err != nil || letter != "«Queued» кончится в четверг." {
		t.Fatalf("%q %v", letter, err)
	}
	mark, offset, err := db.RhythmMark(ctx, user.ID)
	if err != nil || mark != "Queued" || offset != 0 {
		t.Fatalf("%q %d %v", mark, offset, err)
	}
	picks, err := svc.Suggestions(ctx, user.ID)
	if err != nil || len(picks) != 1 || picks[0].Name != "Neighbor" || picks[0].Reason != "Как «Queued»: Drama" {
		t.Fatalf("%v %+v", err, picks)
	}
	if err := svc.ApplyWatched(ctx, user.ID, thursday.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	letter, err = svc.Letter(ctx, user.ID)
	if err != nil || letter != "" {
		t.Fatalf("cleared %q %v", letter, err)
	}
}

func TestRefreshPicksFollowsTheQueue(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	user, err := db.CreateUser(ctx, "ann", "hash", "")
	if err != nil {
		t.Fatal(err)
	}
	crime, err := db.UpsertTitle(ctx, storage.Title{
		ExternalID: 1, Name: "Crime One", Year: 2001, EpisodeCount: 1, AverageMinutes: 40, Genres: []string{"Crime", "Drama"},
	})
	if err != nil {
		t.Fatal(err)
	}
	space, err := db.UpsertTitle(ctx, storage.Title{
		ExternalID: 2, Name: "Space One", Year: 2002, EpisodeCount: 1, AverageMinutes: 40, Genres: []string{"Science-Fiction", "Adventure"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, title := range []storage.Title{
		{ExternalID: 3, Name: "Crime Two", Genres: []string{"Crime", "Drama"}},
		{ExternalID: 4, Name: "Space Two", Genres: []string{"Science-Fiction", "Adventure"}},
	} {
		title.Year, title.EpisodeCount, title.AverageMinutes = 2003+i, 1, 40
		if _, err := db.UpsertTitle(ctx, title); err != nil {
			t.Fatal(err)
		}
	}
	svc := New(db, &fakeClient{})
	q := queue.New(db)
	item, err := q.Add(ctx, user.ID, crime.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RefreshPicks(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
	picks, err := svc.Suggestions(ctx, user.ID)
	if err != nil || len(picks) != 1 || picks[0].Name != "Crime Two" {
		t.Fatalf("%v %+v", err, picks)
	}
	if err := q.Remove(ctx, user.ID, item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Add(ctx, user.ID, space.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.RefreshPicks(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
	picks, err = svc.Suggestions(ctx, user.ID)
	if err != nil || len(picks) != 1 || picks[0].Name != "Space Two" {
		t.Fatalf("after swap %v %+v", err, picks)
	}
}

type memPub struct {
	topics []string
	values [][]byte
}

func (m *memPub) Publish(_ context.Context, topic string, _, value []byte) error {
	m.topics = append(m.topics, topic)
	m.values = append(m.values, append([]byte(nil), value...))
	return nil
}

func TestSetWatchedPublishesWhenKafkaIsUp(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	user, err := db.CreateUser(ctx, "ann", "hash", "")
	if err != nil {
		t.Fatal(err)
	}
	title, err := db.UpsertTitle(ctx, storage.Title{
		ExternalID: 4, Name: "Show", Year: 2010, EpisodeCount: 1, AverageMinutes: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	seasons, err := db.TitleSeasons(ctx, title.ID, user.ID)
	if err != nil || len(seasons) != 1 || len(seasons[0].Episodes) != 1 {
		t.Fatal(err, seasons)
	}
	pub := &memPub{}
	svc := New(db, &fakeClient{}).UsePublisher(pub)
	if err := svc.SetWatched(ctx, user.ID, seasons[0].Episodes[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if len(pub.topics) != 1 || pub.topics[0] != "gloaming.watched" {
		t.Fatalf("%v", pub.topics)
	}
	letter, err := svc.Letter(ctx, user.ID)
	if err != nil || letter != "" {
		t.Fatalf("consumer has not run: %q %v", letter, err)
	}
	if err := svc.HandleWatched(ctx, pub.values[0]); err != nil {
		t.Fatal(err)
	}
	mark, _, err := db.RhythmMark(ctx, user.ID)
	if err != nil || mark != "" {
		t.Fatalf("no pace stored as a date without daily minutes: %q %v", mark, err)
	}
}
