package catalog

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/WaltzForLovers/Gloaming/internal/apperr"
	"github.com/WaltzForLovers/Gloaming/internal/storage"
)

func TestBuildCard(t *testing.T) {
	withAverage := BuildCard(Show{ID: 1, Name: "Show", Year: 2008, AverageRuntime: 48}, []Episode{{Runtime: 10}, {Runtime: 20}})
	if withAverage.AverageMinutes != 48 || withAverage.EpisodeCount != 2 || withAverage.Kind != "series" || withAverage.Year != 2008 {
		t.Fatalf("%+v", withAverage)
	}

	fromEpisodes := BuildCard(Show{ID: 2, Name: "Mix", Year: 2010}, []Episode{{Runtime: 30}, {Runtime: 0}, {Runtime: 40}})
	if fromEpisodes.AverageMinutes != 35 || fromEpisodes.EpisodeCount != 3 {
		t.Fatalf("%+v", fromEpisodes)
	}

	fromRuntime := BuildCard(Show{ID: 3, Name: "Old", Year: 1999, Runtime: 44}, []Episode{{Runtime: 0}, {Runtime: 0}})
	if fromRuntime.AverageMinutes != 44 {
		t.Fatalf("%+v", fromRuntime)
	}

	unknown := BuildCard(Show{ID: 4, Name: "Bare", Year: 2020}, []Episode{{Runtime: 0}})
	if unknown.AverageMinutes != 0 || unknown.EpisodeCount != 1 || unknown.Kind != "film" {
		t.Fatalf("%+v", unknown)
	}

	none := BuildCard(Show{ID: 5, Name: "Empty"}, nil)
	if none.EpisodeCount != 0 || none.Kind != "" || none.AverageMinutes != 0 || none.Year != 0 {
		t.Fatalf("%+v", none)
	}
}

func TestSearchValidatesAndSaves(t *testing.T) {
	db := openDB(t)
	fake := &fakeClient{
		shows: map[string][]Show{
			"girls": {{ID: 10, Name: "Second", Year: 2011, AverageRuntime: 20, Poster: "https://img.example/a.jpg"}, {ID: 9, Name: "First", Year: 2012, AverageRuntime: 30}},
		},
		episodes: map[int][]Episode{
			9:  {{Runtime: 30}, {Runtime: 30}, {Runtime: 30}},
			10: {{Runtime: 20}},
		},
	}
	svc := New(db, fake)

	if _, err := svc.Search(context.Background(), " a "); apperr.From(err).Code != apperr.CodeValidation {
		t.Fatal(err)
	}
	fake.queries = nil
	cards, err := svc.Search(context.Background(), "  girls ")
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 2 || cards[0].Name != "Second" || cards[1].Name != "First" {
		t.Fatalf("%+v", cards)
	}
	if cards[0].Kind != "film" || cards[0].ID == 0 || cards[0].Poster != "https://img.example/a.jpg" || cards[1].Kind != "series" || cards[1].EpisodeCount != 3 {
		t.Fatalf("%+v", cards)
	}

	fake.err = errors.New("offline")
	got, err := svc.Get(context.Background(), cards[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "First" || got.AverageMinutes != 30 {
		t.Fatalf("cached %+v", got)
	}
	withPoster, err := svc.Get(context.Background(), cards[0].ID)
	if err != nil || withPoster.Poster != "https://img.example/a.jpg" {
		t.Fatalf("poster %+v %v", withPoster, err)
	}
	if _, err := svc.Get(context.Background(), 999); apperr.From(err).Code != apperr.CodeTitleNotFound {
		t.Fatal(err)
	}
}

func TestSearchEmptyAndUnavailable(t *testing.T) {
	db := openDB(t)
	fake := &fakeClient{}
	svc := New(db, fake)
	if _, err := svc.Search(context.Background(), "zzzz"); apperr.From(err).Code != apperr.CodeTitleNotFound {
		t.Fatal(err)
	}

	fake.err = errors.New("timeout")
	_, err := svc.Search(context.Background(), "girls")
	if apperr.From(err).Code != apperr.CodeCatalogUnavailable {
		t.Fatal(err)
	}
	if errors.Unwrap(err).Error() != "timeout" {
		t.Fatal(errors.Unwrap(err))
	}
}

func TestSearchStopsAtTenAndOnTimeout(t *testing.T) {
	db := openDB(t)
	shows := make([]Show, 12)
	for i := range shows {
		shows[i] = Show{ID: i + 1, Name: "S", Year: 2000, AverageRuntime: 10}
	}
	fake := &fakeClient{shows: map[string][]Show{"many": shows}}
	svc := New(db, fake)
	cards, err := svc.Search(context.Background(), "many")
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 10 {
		t.Fatalf("got %d", len(cards))
	}

	fake.block = true
	svc.timeout = 20 * time.Millisecond
	_, err = svc.Search(context.Background(), "many")
	if apperr.From(err).Code != apperr.CodeCatalogUnavailable {
		t.Fatal(err)
	}
}

func TestSearchIncludesFilms(t *testing.T) {
	db := openDB(t)
	shows := &fakeClient{shows: map[string][]Show{
		"dragon": nil,
		"both":   {{ID: 7, Name: "The Show", Year: 2021, AverageRuntime: 40}},
	}}
	films := &fakeFilms{films: map[string][]Show{
		"dragon": {{ID: 373096, Name: "How to Train Your Dragon", Year: 2010, Runtime: 98, Poster: "https://img.example/d.jpg"}},
		"both":   {{ID: 373096, Name: "How to Train Your Dragon", Year: 2010, Runtime: 98, Poster: "https://img.example/d.jpg"}},
	}}
	svc := New(db, shows).WithFilms(films)

	cards, err := svc.Search(context.Background(), "dragon")
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 || cards[0].Name != "How to Train Your Dragon" || cards[0].Kind != "film" || cards[0].EpisodeCount != 1 || cards[0].AverageMinutes != 98 || cards[0].Year != 2010 || cards[0].Poster != "https://img.example/d.jpg" || cards[0].ID == 0 {
		t.Fatalf("%+v", cards)
	}

	shows.shows["clash"] = []Show{{ID: 373096, Name: "Not The Film", Year: 2001, AverageRuntime: 22}}
	savedShow, err := svc.Search(context.Background(), "clash")
	if err != nil {
		t.Fatal(err)
	}
	if savedShow[0].ID == cards[0].ID || savedShow[0].Name != "Not The Film" {
		t.Fatalf("ids collided film %d show %+v", cards[0].ID, savedShow)
	}

	shows.err = errors.New("timeout")
	onlyFilm, err := svc.Search(context.Background(), "dragon")
	if err != nil || len(onlyFilm) != 1 || onlyFilm[0].Kind != "film" {
		t.Fatalf("%+v %v", onlyFilm, err)
	}
	shows.err = nil

	films.err = errors.New("wiki down")
	onlyShow, err := svc.Search(context.Background(), "both")
	if err != nil || len(onlyShow) != 1 || onlyShow[0].Name != "The Show" {
		t.Fatalf("%+v %v", onlyShow, err)
	}
}

func TestSearchKeepsAFilmWhenShowsFillTheList(t *testing.T) {
	db := openDB(t)
	shows := make([]Show, 10)
	for i := range shows {
		shows[i] = Show{ID: i + 1, Name: "S", Year: 2000, AverageRuntime: 10}
	}
	fake := &fakeClient{shows: map[string][]Show{"mix": shows}}
	films := &fakeFilms{films: map[string][]Show{
		"mix": {{ID: 50, Name: "The Film", Year: 1999, Runtime: 100}},
	}}
	cards, err := New(db, fake).WithFilms(films).Search(context.Background(), "mix")
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 10 || cards[len(cards)-1].Name != "The Film" || cards[len(cards)-1].Kind != "film" {
		t.Fatalf("%d %+v", len(cards), cards[len(cards)-1])
	}
}

type fakeFilms struct {
	films map[string][]Show
	err   error
}

func (f *fakeFilms) SearchFilms(_ context.Context, query string) ([]Show, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.films[query], nil
}

type fakeClient struct {
	shows    map[string][]Show
	episodes map[int][]Episode
	err      error
	block    bool
	queries  []string
}

func (f *fakeClient) SearchShows(ctx context.Context, query string) ([]Show, error) {
	if err := f.wait(ctx); err != nil {
		return nil, err
	}
	f.queries = append(f.queries, query)
	if f.err != nil {
		return nil, f.err
	}
	return f.shows[query], nil
}

func (f *fakeClient) Episodes(ctx context.Context, showID int) ([]Episode, error) {
	if err := f.wait(ctx); err != nil {
		return nil, err
	}
	if f.err != nil {
		return nil, f.err
	}
	if f.episodes == nil {
		return []Episode{{Runtime: 10}}, nil
	}
	return f.episodes[showID], nil
}

func (f *fakeClient) wait(ctx context.Context) error {
	if !f.block {
		return nil
	}
	<-ctx.Done()
	return ctx.Err()
}

func openDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "queue.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
