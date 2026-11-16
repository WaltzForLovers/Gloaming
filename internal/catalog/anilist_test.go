package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAniListSearchReadsMedia(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&req) != nil || req.Variables["search"] != "frieren" {
			t.Errorf("%s %+v", r.Method, req)
		}
		w.Write([]byte(`{"data":{"Page":{"media":[
			{"id":154587,"format":"TV","episodes":28,"duration":24,"startDate":{"year":2023},
			 "title":{"romaji":"Sousou no Frieren","english":"Frieren: Beyond Journey's End"},
			 "coverImage":{"large":"https://img.example/f.jpg"},"genres":["Adventure","Sci-Fi"],
			 "averageScore":90,"description":"An elf mage.<br><br>(Source: Crunchyroll)"},
			{"id":2,"format":"TV","episodes":null,"duration":24,"startDate":{"year":2026},
			 "title":{"romaji":"Airing Now","english":null},"genres":[],"nextAiringEpisode":{"episode":5}},
			{"id":3,"format":"MOVIE","episodes":null,"duration":110,"startDate":{"year":2001},
			 "title":{"romaji":"Sen to Chihiro no Kamikakushi","english":"Spirited Away"},"genres":["Fantasy"]},
			{"id":4,"format":"TV","episodes":null,"duration":null,"startDate":{"year":2027},
			 "title":{"romaji":"Not Yet","english":null},"genres":[]}
		]}}}`))
	}))
	defer server.Close()

	client := NewAniList(server.URL).WithCache(newMemoryCache())
	shows, err := client.SearchAnime(context.Background(), "frieren")
	if err != nil {
		t.Fatal(err)
	}
	if len(shows) != 3 {
		t.Fatalf("%+v", shows)
	}
	first := shows[0]
	if first.ID != 154587 || first.Source != sourceAniList || first.Name != "Frieren: Beyond Journey's End" || first.Alias != "Sousou no Frieren" ||
		first.Year != 2023 || first.Episodes != 28 || first.Runtime != 24 || first.Rating != 9 || first.Poster != "https://img.example/f.jpg" {
		t.Fatalf("%+v", first)
	}
	if len(first.Genres) != 3 || first.Genres[0] != "Adventure" || first.Genres[1] != "Science-Fiction" || first.Genres[2] != "Anime" {
		t.Fatalf("%v", first.Genres)
	}
	if shows[1].Name != "Airing Now" || shows[1].Episodes != 4 {
		t.Fatalf("%+v", shows[1])
	}
	if shows[2].Name != "Spirited Away" || shows[2].Episodes != 1 || shows[2].Runtime != 110 {
		t.Fatalf("%+v", shows[2])
	}
	if _, err := client.SearchAnime(context.Background(), "frieren"); err != nil || calls != 1 {
		t.Fatalf("cache missed: %v calls=%d", err, calls)
	}
}

func TestSearchAddsAnimeSeasonsAndSkipsTheSameShow(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	shows := &fakeClient{shows: map[string][]Show{
		"titan": {{ID: 919, Name: "Attack on Titan", Year: 2013, AverageRuntime: 25}},
	}}
	anime := &fakeAnime{shows: map[string][]Show{
		"titan": {
			{ID: 16498, Source: sourceAniList, Name: "Attack on Titan", Alias: "Shingeki no Kyojin", Year: 2013, Episodes: 25, Runtime: 24},
			{ID: 20958, Source: sourceAniList, Name: "Attack on Titan Season 2", Year: 2017, Episodes: 12, Runtime: 24,
				Poster: "https://img.example/s2.jpg", Genres: []string{"Action", "Anime"}, Summary: "Season <i>two</i>."},
		},
	}}
	svc := New(db, shows).WithAnime(anime)

	cards, err := svc.Search(ctx, "titan")
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 2 || cards[0].Name != "Attack on Titan" || cards[1].Name != "Attack on Titan Season 2" {
		t.Fatalf("%+v", cards)
	}
	second := cards[1]
	if second.Kind != "series" || second.EpisodeCount != 12 || second.AverageMinutes != 24 || second.Summary != "Season two." {
		t.Fatalf("%+v", second)
	}
	card, seasons, err := svc.Detail(ctx, 0, second.ID)
	if err != nil || card.Assembling {
		t.Fatalf("%v %+v", err, card)
	}
	if len(seasons) != 1 || len(seasons[0].Episodes) != 12 || seasons[0].Poster != "https://img.example/s2.jpg" || seasons[0].Episodes[11].Runtime != 24 {
		t.Fatalf("%+v", seasons)
	}

	shows.err = errors.New("tvmaze down")
	onlyAnime, err := svc.Search(ctx, "titan")
	if err != nil || len(onlyAnime) != 2 || onlyAnime[0].Name != "Attack on Titan" || onlyAnime[0].EpisodeCount != 25 {
		t.Fatalf("%v %+v", err, onlyAnime)
	}
}

type fakeAnime struct {
	shows map[string][]Show
	err   error
}

func (f *fakeAnime) SearchAnime(ctx context.Context, query string) ([]Show, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.shows[query], nil
}
