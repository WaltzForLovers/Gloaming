package catalog

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestSearchSavesFilmsWhenShowCatalogTimesOut(t *testing.T) {
	db := openDB(t)
	shows := &fakeClient{block: true}
	films := &fakeFilms{films: map[string][]Show{
		"dragon": {{ID: 373096, Name: "How to Train Your Dragon", Year: 2010, Runtime: 98}},
	}}
	svc := New(db, shows).WithFilms(films)
	svc.timeout = 30 * time.Millisecond
	cards, err := svc.Search(context.Background(), "dragon")
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 || cards[0].Name != "How to Train Your Dragon" || cards[0].ID == 0 {
		t.Fatalf("%+v", cards)
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

func TestMergeCardsKeepsTheOwnersFirst(t *testing.T) {
	owned := []Card{{ID: 1, Name: "Mine"}}
	found := make([]Card, 12)
	for i := range found {
		found[i] = Card{ID: int64(i + 2), Name: "Catalog"}
	}
	found[0].ID = 1
	got := MergeCards(owned, found)
	if len(got) != 10 || got[0].Name != "Mine" || got[1].ID != 3 {
		t.Fatalf("%d %+v", len(got), got)
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

func TestSearchReturnsFilmsWhileShowsLag(t *testing.T) {
	db := openDB(t)
	shows := &fakeClient{block: true}
	films := &fakeFilms{films: map[string][]Show{
		"lag": {{ID: 3, Name: "Film", Year: 2001, Runtime: 90}},
	}}
	svc := New(db, shows).WithFilms(films)
	svc.timeout = 5 * time.Second
	svc.grace = 30 * time.Millisecond
	start := time.Now()
	cards, err := svc.Search(context.Background(), "lag")
	if err != nil || len(cards) != 1 || cards[0].Name != "Film" || cards[0].Year != 2001 {
		t.Fatalf("%v %+v", err, cards)
	}
	if time.Since(start) > time.Second {
		t.Fatal(time.Since(start))
	}
}

func TestSearchLeavesASlowSeasonAssembling(t *testing.T) {
	db := openDB(t)
	shows := &fakeClient{
		shows:         map[string][]Show{"slow": {{ID: 9, Name: "Slow", Year: 2011, AverageRuntime: 40}}},
		blockEpisodes: true,
	}
	svc := New(db, shows)
	svc.episodeBudget = 30 * time.Millisecond
	cards, err := svc.Search(context.Background(), "slow")
	if err != nil || len(cards) != 1 || !cards[0].Assembling {
		t.Fatalf("%v %+v", err, cards)
	}
	seasons, err := db.TitleSeasons(context.Background(), cards[0].ID, 0)
	if err != nil || len(seasons) != 0 {
		t.Fatalf("%v %+v", err, seasons)
	}
	shows.blockEpisodes = false
	shows.episodes = map[int][]Episode{
		9: {{Season: 1, Number: 1, Runtime: 40}, {Season: 1, Number: 2, Runtime: 40}},
	}
	if err := svc.Assemble(context.Background(), cards[0].ID, ""); err != nil {
		t.Fatal(err)
	}
	title, err := db.TitleByID(context.Background(), cards[0].ID)
	if err != nil || title.Assembling || title.EpisodeCount != 2 {
		t.Fatalf("%v %+v", err, title)
	}
	seasons, err = db.TitleSeasons(context.Background(), cards[0].ID, 0)
	if err != nil || len(seasons) != 1 || len(seasons[0].Episodes) != 2 {
		t.Fatalf("%v %+v", err, seasons)
	}
}

func TestCatalogGetUsesCache(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		fmt.Fprint(w, `[{"show":{"id":1,"name":"Cached"}}]`)
	}))
	defer srv.Close()
	client := NewHTTPClient(srv.URL).WithCache(newMemoryCache())
	for i := 0; i < 2; i++ {
		shows, err := client.SearchShows(context.Background(), "q")
		if err != nil || len(shows) != 1 || shows[0].Name != "Cached" {
			t.Fatal(err, shows)
		}
	}
	if hits != 1 {
		t.Fatalf("hits %d", hits)
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
	shows         map[string][]Show
	episodes      map[int][]Episode
	err           error
	block         bool
	blockEpisodes bool
	queries       []string
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
	if f.blockEpisodes {
		<-ctx.Done()
		return nil, ctx.Err()
	}
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
	return storage.OpenTest(t)
}
